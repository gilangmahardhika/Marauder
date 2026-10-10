# Backend implementation

Status: Accepted for implementation. Date: 2026-10-10. Revised after review.

Build this in the order at the end. Tables, routes, and settings fields are fixed by [0002-data-and-api.md](0002-data-and-api.md). Security controls are fixed by [0006-security-and-harness.md](0006-security-and-harness.md). Where this file is looser, 0006 wins. Do not add a second status writer.

Module path: `orchestrator/backend`.

## Packages

| Package | Responsibility | Must not |
| --- | --- | --- |
| `internal/machine` | Legal edges, evidence checks, compare-and-swap `Transition` | Call git, models, or HTTP |
| `internal/trace` | Append events, redact, list by card | Update a row in place |
| `internal/board` | PostgreSQL reads and writes for cards, attempts, locks, runs | Choose the next status |
| `internal/intake` | goldmark parse, hashes, accept, drafts | Create a worktree |
| `internal/settings` | `config.yaml`, `secrets.json`, validation, snapshots | Return a secret to HTTP |
| `internal/router` | Task class and model choice | Call a provider |
| `internal/contextpack` | Build the four packs | Include the author transcript in the reviewer pack |
| `internal/providers` | Anthropic and OpenAI clients | Pick a fallback |
| `internal/execx` | `execChild`: argv only, built env, timeout, output cap | Start a shell |
| `internal/sandbox` | Exports and container argv | Mount the worktree |
| `internal/runtime` | Worktree, file tools, commit | Deploy |
| `internal/vcs` | `git fetch` and `push`, `gh` pull request calls | Merge |
| `internal/deploy` | Deploy lock, command, probe, rollback | Call a model |
| `internal/verify` | Dev check and log capture | Mark pass from prose |
| `internal/worker` | Worker lock, wake loop, jobs | Serve HTTP |
| `internal/app` | Use cases shared by CLI and HTTP | Encode HTML |
| `internal/cli` | Cobra commands | SQL |
| `internal/httpapi` | JSON, bearer, CORS, `Host` check | SQL |

`cmd/orchestrator/main.go` wires the database, settings, and logger. Tests construct `app` with fakes for the provider, git, `gh`, the sandbox engine, and the clock.

## Startup

| Flag | Default |
| --- | --- |
| `--repo` | Current directory |
| `--data` | `~/.orchestrator/<repo-name>-<hash8>` (0002) |
| `--addr` | `127.0.0.1:8787` |
| `--ui-origin` | `http://127.0.0.1:4173` |

Every command runs these steps before its own work:

1. Resolve `--repo` and `--data` to absolute paths with symlinks evaluated. Exit `1` when either contains the other (RFC-036).
2. Create the data directory with mode `0700`, plus `empty-hooks/`, `empty-gh/`, `askpass/`, `worktrees/`, `exports/`, and `logs/`. Exit if an existing data directory is group or world accessible.
3. Fixture mode is on only when the resolved repo path ends with `/testdata/fixture-repo`.
4. Resolve absolute paths for `git`, `gh`, and the sandbox engine once. A later `PATH` change does not change them.
5. Load `secrets.json`. On the first start, when the file is missing, seed it from `ORCHESTRATOR_DATABASE_URL`, `GH_TOKEN`, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, and `DEPLOY_TOKEN` if they are set. After that the file wins over the environment. Exit before opening the database when `databaseUrl` is empty.
6. Open PostgreSQL with that URL and run goose migrations using the postgres dialect.
7. Load and validate `config.yaml` with the same rules as the PUT routes.

`serve` then refuses a non-loopback `--addr`, takes the worker lock (exits if held), runs `Reconcile`, starts the worker, generates a 32-byte UI token, prints `UI token: <hex>` once, and listens.

## Worker lock

`internal/worker` takes an exclusive, non-blocking `flock` on `<data>/worker.lock` (RFC-043). The lock is released when the process exits, including on a crash.

| Command | Lock behavior |
| --- | --- |
| `serve` | Takes it. Exits `1` with `another worker is running` when held |
| `run-next` | Inserts a `runs` row. If the lock is free, takes it, runs `Reconcile`, processes until idle, exits. If held, prints `queued for the running server` and exits `0` |
| Human gates, `ingest`, `accept`, `status`, `trace` | Do not take it |

## Transition

`machine.Transition(tx, card, to, evidence) (Card, error)` is the only status write.

```sql
UPDATE cards
SET status = $1, updated_at = $2, <edge-specific columns>
WHERE id = $3 AND status = $4;
```

The status in the WHERE clause is the status the caller read. `RowsAffected` must be `1`, otherwise `board.ErrConflict`, and the caller does nothing else (RFC-040). The same PostgreSQL transaction appends the `transition` event and inserts the gate row for gated edges:

| Edge into | Gate name |
| --- | --- |
| `pr_open` | `local_checks` |
| `human_review` from `agent_review` | `agent_review` |
| `approved` | `human_approval` |
| `verifying` | `dev_health` |
| `done` | `dev_tests` |

Evidence is a struct with the fields the edge needs: `ExitCode`, `PRURL`, `SHA`, `Approver`, `LogRef`, `Reason`, `ResetAttempts`. Missing evidence returns `ErrEvidence`. An unknown pair returns `ErrIllegal`. Both append `transition_rejected` in a separate write and change nothing else.

The edge map in code is the table in PRD REQ-006. A test fails if a pair from that table is missing from the map, or if the map has a pair the table lacks.

Edges that clear `approved_sha`, and mark the live approval row `cleared_at`: into `in_progress`, into `changes_requested`, `approved -> human_review`, `approved -> blocked`, and `blocked -> human_review`.

## Reconcile

Runs only in the process that holds the worker lock, before any job. No model calls.

1. Delete every `locks` row and reset `deploy_lock`. No job is alive.
2. For each card whose latest `deploy` event has no later `health` event: probe health. Pass writes `health` and moves `deploying_dev -> verifying`. Fail runs rollback and moves to `blocked` with `health`.
3. For each attempt with `ended_at` null: if `commit_sha` is set and the card is `in_progress`, transition to `self_check`, and the next pass runs the gating check without a model call. Otherwise end the attempt with outcome `cancelled`, which does not count.
4. Delete leftover directories under `exports/`.
5. Leave `done` cards untouched.

## Worker

One loop goroutine ticks every 2 seconds. While an unprocessed `runs` row exists, each tick runs a pass. When a pass starts nothing, no job is running, and no card waits on CI, the loop sets `processed_at` on every open `runs` row. The board is then idle until the next row.

Jobs run in goroutines beside the loop. Implement and plan jobs are capped by `scheduler.implementationCap`. At most one deploy job runs. Review and verify jobs are capped at 2 each. Every job takes its card lock with a fresh random holder and releases it with that holder.

Each pass, in order:

1. Refresh head and CI. For cards in `agent_review` with a `recommend_approve` verdict, `human_review`, and `approved`, call `gh pr view` at most once every 30 seconds per card and store `head_sha`.
2. A card in `approved` whose head moved goes to `human_review`, approval cleared.
3. Promote `backlog -> ready` when requirements are accepted, dependencies are `done`, and the card has acceptance lines or is manual.
4. A card in `changes_requested`, or in `in_progress` with no open attempt, whose `attempts_used >= max_attempts` goes to `blocked` with `attempt_cap`.
5. Start a plan job when `draftPending` is set, or when an accepted requirement has no linked card. One plan job at a time.
6. Start a review job for each `pr_open` card.
7. For `agent_review` cards waiting on CI: all required checks pass, or none required, then `human_review`. A required check fails, then `changes_requested` with a failure note of source `ci` naming the failed checks.
8. Start a deploy job for the oldest `approved` card when the deploy lock is free.
9. Start a verify job for each `verifying` card.
10. Start implement jobs while running implement jobs are under the cap: oldest first among `ready`, `changes_requested`, and `in_progress` with no open attempt, whose requirements are accepted, dependencies are `done`, and `attempts_used < max_attempts`.

Every job snapshots settings at start and uses that snapshot to the end.

### Plan job

1. Route `plan`. No model configured: set `planError` to `router_config` and stop.
2. When `draftPending`, the planner pack holds the PRD outline. `propose_requirements` writes rows with `source_kind = 'draft'` and `accepted_at` null, then clears `draftPending`.
3. Otherwise the pack holds accepted requirements without cards, decisions, and existing card titles and dependencies. `create_card` takes `ref`, `title`, `requirementIds`, `decisionIds`, `dependsOn` (card ids or refs from this job), `risk`, and `manual`.
4. The harness rejects a call that names an unknown or unaccepted requirement id, an unknown decision, an unknown dependency, or a dependency cycle. It assigns the card id, copies `checks.local` and `checks.dev` from settings, sets `max_attempts` from the router budget, and inserts the card in `backlog`. A command field in the arguments is ignored and logged as rejected.
5. Planner tokens are logged as `model_call` with no card id.

### Implement job

1. Take the card lock. For a real repo, require `github.owner`, `github.repo`, `github.botLogin`, and a GitHub token, else block with `github_config`. Require a sandbox image, else `sandbox_config`. Route the task class, and block with `router_config` when the route has no model.
2. When the worktree is missing, create it (git commands below) and store `branch`, `worktree_path`, and `worktree_gitdir`. On a later attempt, reuse it.
3. Record the local `refs/heads/<baseBranch>` SHA for the `base_moved` assertion.
4. Insert an attempt row: `n` is the card's highest `n` plus one, `parent_attempt_id` is the previous attempt, `base_sha` is the worktree HEAD.
5. If the card is `ready` or `changes_requested`, transition to `in_progress`. A card already `in_progress` with no open attempt stays there.
6. Build the implementer pack with the latest failure note and no prior model messages. Run the tool loop.
7. Budget hit: end the attempt with outcome `budget`, add one to `attempts_used`, append `budget_stop`, and block with `turn_cap`, `token_cap`, or `wall_cap`.
8. No new commit since `base_sha`: end with `no_commit`, add one to `attempts_used`, and write a failure note with source `no_commit`. The card stays `in_progress`, and pass step 4 or step 10 picks it up.
9. New commit: store `commit_sha`, transition `in_progress -> self_check`, and run the gating check on an export of that SHA (0006).
10. Exit `0`: push the branch, ensure one pull request, rewrite its metadata, end with `checks_passed`, add one to `attempts_used`, transition `self_check -> pr_open`.
11. Exit non-zero: store the log, run triage for a failure note, end with `checks_failed`, and add one to `attempts_used`. If attempts remain, transition `self_check -> in_progress`. If none remain, block with `attempt_cap`.
12. If the local base ref changed during the job, append `assertion_failed` and block with `base_moved`.
13. Release the card lock.

The implement job never calls `deploy`.

### Tool loop

The provider receives the tool list for the profile. The harness executes each tool and returns the result as the next input of this attempt only.

| Profile | Tools |
| --- | --- |
| `planner` | `read_doc`, `propose_requirements`, `create_card` |
| `implementer` | `read_file`, `edit_file`, `run_local_check`, `commit` |
| `reviewer` | `read_diff`, `comment_pr`, `verdict` |
| `triage` | `read_log`, `write_failure_note` |

`read_file` and `edit_file` go through `safePath` (0006). `run_local_check` takes no arguments. It runs the card's `localCheck` on a working-tree export with network none, and returns the exit code and the last 200 lines. It never moves the card. `commit` stages the worktree and commits with the model's message plus fixed trailers:

```text
Orchestrator-Card: <card id>
Orchestrator-Attempt: <attempt id>
Model: <model id>
```

`verdict` accepts `recommend_approve` or `request_changes` only. Anything else is a parse failure. The second parse failure goes to the fallback model once. If that also fails to parse, the card blocks with `review_parse`.

The loop ends when the model stops calling tools, or at `maxModelTurnsPerAttempt`, `maxTokensPerCard` summed across the card, or `maxWallMinutesPerAttempt`.

### Review job

1. Take the card lock. Route `review` with the author model id of the latest attempt. Transition `pr_open -> agent_review`.
2. Build the reviewer pack from linked requirements, decisions, the diff from base to head, the gating check output, the pull request body, and human comments. Omit author messages and tool traces. Write the pack JSON to `logs/<attempt-id>-review.json`.
3. Call the model. Store `review_model_id`, `review_prompt_hash`, `review_independent`, `review_verdict`, and `reviewed_sha` on the reviewed attempt.
4. `request_changes`: write a failure note with source `review` from the review body, then `agent_review -> changes_requested`.
5. `recommend_approve`: read required checks with `gh pr view --json statusCheckRollup`. No required checks: `human_review` with `ciRequired: false` in the gate. All required pass: `human_review` with `ciRequired: true`. Any required failure: `changes_requested` with a `ci` note. Pending: stay in `agent_review` and let pass step 7 finish it.
6. Provider failure after retry and fallback: block with `provider_error`.

A review never changes `attempts_used`.

### Deploy job

1. Take the deploy lock with a fresh holder. Take the card lock.
2. Refresh the head. If it differs from `approved_sha`, transition `approved -> human_review` and release both locks.
3. Validate the snapshot: `devCommand`, `rollbackCommand`, `healthUrl`, and `devHosts` are present and pass the 0002 rules, and `devUrl` is present when `devCheck` is non-empty. Otherwise `approved -> blocked` with `deploy_config` and no exec.
4. `previousSha` is the `requestedSha` of the latest `deploy` event whose following `health` event passed, or empty when none exists.
5. Append the `deploy` event with `requestedSha`, `previousSha`, and `devUrl`. Then transition `approved -> deploying_dev`. Then exec `[devCommand, approvedSha]` with `DEPLOY_TOKEN` and `DEV_URL` in the environment.
6. Non-zero deploy exit: run rollback, append `health` with `passed: false` and the rollback exit, and block with `deploy_failed`.
7. Probe `healthUrl` (0006). Failure: run rollback, append `health` with `passed: false`, block with `health`.
8. Success: append `health` with `passed: true`, set `deployed_sha`, transition `deploying_dev -> verifying`.
9. Release both locks with their holders.

Rollback argv is `[rollbackCommand, previousSha]`, or `[rollbackCommand]` when `previousSha` is empty. The script decides what an empty argument means.

### Verify job

1. Take the card lock.
2. Empty `devCheck`: append `verify` with `mode: health_only`, then `verifying -> done`.
3. Otherwise export `deployed_sha` to `exports/<card-id>-<sha>`, run `devCheck` in the sandbox with network and `DEV_URL` (0006), then delete the export.
4. Exit `0`: append `verify` with `mode: dev_check`, transition to `done`.
5. Non-zero: append `verify`, run triage for a note with source `dev_check`, clear approval. The attempt that shipped already counted. If `attempts_used < max_attempts`, transition to `changes_requested`, otherwise block with `attempt_cap`.

## Human gates

`internal/app` owns these. The CLI and HTTP call the same functions. None of them take the worker lock. All of them use the compare-and-swap transition.

| Function | From | Checks | Result |
| --- | --- | --- | --- |
| `Approve(card, sha)` | `human_review` | `operatorLogin` set, on `approvers`, not `botLogin`. `sha` equals the refreshed head | `approved`, one approval row, `runs` row |
| `RequestChanges(card)` | `human_review`, `approved` | `operatorLogin` on `approvers` | `changes_requested`, approval cleared, note with source `approver`, `runs` row |
| `Reject(card)` | `human_review`, `approved` | Same | `blocked` with `rejected` |
| `Unblock(card, reset)` | `blocked` | `unblockTo` is not null | `ready` or `human_review` per RFC-041, `runs` row |

Approving the same SHA twice finds the live approval row by the unique index and returns the card unchanged.

## Router

```text
func Decide(in RouteInput) (RouteDecision, error)
```

`app` sets the task class from card facts. A model never does.

| Condition | Class |
| --- | --- |
| Draft ids needed, or accepted requirements without cards | `plan` |
| Risk `low` and `attempts_used == 0` | `implement` |
| Risk `high`, or `attempts_used >= 1` | `implement_hard` |
| Pull request open and the gating check passed | `review` |
| A check exited non-zero and a note is required | `triage` |

There is no `deploy` class.

Review with the sentinel `different-family-from-author` picks the first model, in family order, whose family differs from the author's. With one usable family, `review_independent` is false and the reviewer is that family's first model.

Provider call:

1. Call `modelId`.
2. On a transport error, HTTP 429, or HTTP 5xx, call the same model once more.
3. Call `fallbackModelId` once when it is set.
4. If it still fails, end the attempt with outcome `error` and block with `provider_error`. Both error strings, redacted, go in the event.

Append a `route` event before the call and a `model_call` event after it.

## Context packs

Each builder returns the fixed system string for the profile and a user string (0006). Hash the canonical JSON of the user sections with SHA-256 for `prompt_hash`, and the system string for `system_hash`.

| Pack | Sections |
| --- | --- |
| Planner | PRD outline, requirement candidates, decision statements, card titles, dependencies |
| Implementer | This card, linked statements, linked decisions, path list, `localCheck`, last failure note |
| Reviewer | Linked statements, linked decisions, diff, gating check output, pull request body, human comments |
| Triage | Command, exit code, last 200 log lines, acceptance lines |

Snapshot fixtures live in `backend/internal/contextpack/testdata/`. The reviewer fixture contains the diff and does not contain the string `author-transcript`.

## Providers

```text
type Provider interface {
    Complete(ctx context.Context, req Request) (Response, error)
}
```

`Request` has `Model`, `System`, `User`, `Tools`, and `MaxTokens`. `Response` has text, tool calls, token counts, and a stop reason.

The `anthropic` protocol uses `anthropic-sdk-go`. The `openai` protocol uses `openai-go/v3` with the family's `baseUrl`, which also covers local OpenAI-compatible servers on loopback without a key. Keys come from the job's secrets snapshot. Logs receive the redacted request.

A fake implements `Provider` and replays a script file (0005). The REQ-043 fixture uses the fake. Live keys are not required for that suite.

## Git and GitHub

Every git call uses the prefix and environment in 0006, including the explicit `--git-dir` for worktree calls and `GIT_ASKPASS` for network calls.

Worktree creation for a real repo:

```text
git -C <repo> fetch <url> +refs/heads/<base>:refs/orchestrator/base
git -C <repo> worktree add -b orch/<card-id> <data>/worktrees/<card-id> refs/orchestrator/base
```

The fixture skips the fetch and uses the local base branch. `<url>` is `https://<host>/<owner>/<repo>.git`, built from settings. Fetching into `refs/orchestrator/base` leaves `refs/heads/<base>` alone.

Commit identity is `-c user.name=<botLogin> -c user.email=<botLogin>@users.noreply.github.com`. Signing is off.

Push:

```text
git push <url> refs/heads/orch/<card-id>:refs/heads/orch/<card-id>
```

The pull request body has two regions. The harness rewrites only the metadata region on each push.

```text
<!-- orchestrator:metadata:start -->
requirements: REQ-001
decisions: RFC-018
localCheck: go test ./...
attempt: 1
model: example-model
<!-- orchestrator:metadata:end -->

<summary written by the implementer>
```

`gh` commands, with `GH_TOKEN`, `GH_HOST`, and `GH_CONFIG_DIR` set:

```text
gh pr create --repo OWNER/REPO --base BASE --head orch/<card-id> --title TITLE --body-file BODY
gh pr edit URL --body-file BODY
gh pr comment URL --body-file COMMENT
gh pr view URL --json url,headRefOid,statusCheckRollup,reviews,comments
gh api user
```

If `cards.pr_url` is already set, skip `pr create`. The unique index is the second guard. Text from the model goes through `--body-file`, never through argv.

## Settings

`settings.SaveGitHub`, `SaveLLM`, `SaveRouter`, and `SaveDeploy` validate against the 0002 rules and the current state of the other sections. Then they write `config.yaml` and, only for secret fields that were sent, `secrets.json`. Both files are written by temp file, `fsync`, and rename, with mode `0600`. Then they append `config_changed` with field names.

`settings.View*` copies the config into response structs that have no secret fields, so `json.Marshal` cannot leak one.

`settings.Snapshot()` returns an immutable copy of config and secrets for one job.

Saving `checks.local` or `checks.dev` updates cards in `backlog` and blocks cards past `backlog` (except `done`) with `source_changed`.

## HTTP

`internal/httpapi` maps sentinel errors:

| Sentinel | HTTP |
| --- | --- |
| `machine.ErrIllegal`, `machine.ErrEvidence`, `app.ErrActionNotListed` | 422 `transition_rejected` |
| `settings.ErrValidation` | 422 `validation` |
| `board.ErrNotFound` | 404 `not_found` |
| `board.ErrLocked`, `board.ErrConflict` | 409 `conflict` |
| `httpapi.errHost`, `httpapi.errOrigin` | 403 `forbidden` |
| `httpapi.errBody` | 413 `too_large` |
| `httpapi.errPaused` | 429 `rate_limited` |

Middleware order: `Host` check, CORS and `Origin` check, body cap, bearer, handler. CORS sends `Access-Control-Allow-Origin` with the exact `--ui-origin`, allows `Authorization` and `Content-Type`, and allows `GET`, `POST`, `PUT`, and `OPTIONS`. A request with an `Origin` that does not match is `403`.

The worker runs on a context that is not the request context.

## CLI

Cobra commands are thin. Example flow for approve:

1. Require `--sha`.
2. Call `app.Approve(ctx, cardID, sha)`.
3. Print the returned status and the SHA.
4. On error, print the message and exit `1`.

`status` with zero cards prints:

```text
board is empty
next: orchestrator ingest
```

## Build order

Stop at the end of a step until its tests in [0005-verification.md](0005-verification.md) pass.

| Step | Packages | Leave unbuilt |
| --- | --- | --- |
| 1 | `machine`, `trace`, `board`, migration, startup checks | Providers |
| 2 | `intake`, accept in `app` | Worktrees |
| 3 | `execx`, `sandbox`, `runtime` against the fixture repo | Models |
| 4 | Fake provider, implementer pack, tool loop | `gh` |
| 5 | `vcs` | Reviewer |
| 6 | Reviewer pack, second family in the router | Deploy |
| 7 | Human gates in `app` and the CLI | Real health |
| 8 | `deploy`, `verify`, stub health server | Frontend |
| 9 | `settings` and router snapshots | |
| 10 | `worker`: lock, passes, plan job, dependencies, deploy lock | |
| 11 | REQ-043 command | |
| 12 | `httpapi` for every route in 0002 | |

The frontend starts after step 12.
