# Backend implementation

Status: Accepted for implementation. Date: 2026-10-10. Revised after review.

Build this in the order at the end. Tables, routes, and settings fields are fixed by [0002-data-and-api.md](0002-data-and-api.md). Security controls are fixed by [0006-security-and-harness.md](0006-security-and-harness.md). Where this file is looser, 0006 wins. Do not add a second status writer.

Module path: `orchestrator/backend`.

## Packages

| Package | Responsibility | Must not |
| --- | --- | --- |
| `internal/machine` | Legal edges, evidence checks, compare-and-swap `Transition` | Call git, models, or HTTP |
| `internal/trace` | Append events, redact, list by card, redact streamed text | Update a row in place |
| `internal/live` | Fan out the model's reply text to trace streams in memory (RFC-088) | Store or log reply text |
| `internal/board` | PostgreSQL reads and writes for cards, attempts, locks, runs | Choose the next status |
| `internal/intake` | goldmark parse, hashes, accept, drafts, manual rows | Create a worktree |
| `internal/settings` | `config.yaml`, `secrets.json`, validation, snapshots | Return a secret to HTTP |
| `internal/conventions` | Rule, stack, and skill catalog, revisions, stack detection, a card's rules and skills | Read instructions from the target repo |
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
| `--repo` | On a local run and the fixture, the repo to attach. `serve` uses each project's clone at `<data>/projects/<id>/repo` (RFC-050) |
| `--data` | `~/.orchestrator/service` (0002) |
| `--addr` | `127.0.0.1:8787` |
| `--ui-origin` | `http://127.0.0.1:4173`, or the configured origin when `ORCHESTRATOR_CLOUD=1` |

Every command runs these steps before its own work:

1. Resolve `--data` to an absolute path with symlinks evaluated. Exit `1` when the service root contains a project repo or a project repo contains its project directory (RFC-036).
2. Create the service directory with mode `0700`. Exit if it is group or world accessible. Each project directory is created when the project is created, with `empty-hooks/`, `empty-gh/`, `askpass/`, `worktrees/`, `exports/`, and `logs/`.
3. Fixture mode is on only when a project's resolved repo path ends with `/testdata/fixture-repo`.
4. Resolve absolute paths for `git`, `gh`, and the sandbox engine once. A later `PATH` change does not change them.
5. Load the service `secrets.json`. On the first start, when the file is missing, seed `databaseUrl` from `ORCHESTRATOR_DATABASE_URL` and generate `tokenKey`. After that the file wins over the environment. Exit before opening the database when `databaseUrl` or `tokenKey` is empty.
6. Open PostgreSQL with that URL and run goose migrations using the postgres dialect.
7. Load and validate each project's `config.yaml` with the same rules as the PUT routes. A project whose file is invalid does not start its worker. The others still start.

`serve` refuses a non-loopback `--addr` unless `ORCHESTRATOR_CLOUD=1` (RFC-045). It does not print a UI token. It starts one worker per project.

## Worker lock

`internal/worker` takes an exclusive, non-blocking `flock` on `<data>/projects/<project-id>/worker.lock` (RFC-048). The lock is released when that worker stops, including on a crash. Two projects lock different files.

| Command | Lock behavior |
| --- | --- |
| `serve` | Takes each project's lock and runs that worker. A project whose lock is already held is logged and skipped. The process still serves the others |
| `run-next --project` | Inserts a `runs` row for that project. If that project's lock is free, takes it, runs `Reconcile` for that project, processes until idle, exits. If held, prints `queued for the running server` and exits `0`. Today `Reconcile` runs only step 1 and the cancel part of step 3, then the passes, then the implement jobs of queued cards, waiting for them, and prints `started N` (RFC-062, RFC-063) |
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

Runs only in the worker that holds that project's lock, before any job for that project. No model calls. Other projects are left alone.

1. Delete `locks` rows for that project's cards and reset that project's `deploy_lock`. No job of that project is alive.
2. For each card whose latest `deploy` event has no later `health` event: probe health. Pass writes `health` and moves `deploying_dev -> verifying`. Fail runs rollback and moves to `blocked` with `health`.
3. For each attempt with `ended_at` null: if `commit_sha` is set and the card is `in_progress`, transition to `self_check`, and the next pass runs the gating check without a model call. Otherwise end the attempt with outcome `cancelled`, which does not count.
4. Delete leftover directories under `exports/`.
5. Leave `done` cards untouched.

## Worker

One loop goroutine per project ticks every 2 seconds. Projects run those loops at the same time (RFC-048). While an unprocessed `runs` row exists for that project, each tick runs a pass. When a pass starts nothing, no job is running for that project, and no card of that project waits on CI, the loop sets `processed_at` on that project's open `runs` rows. The project is then idle until the next row.

A row with `card_id` set asks for that one card (RFC-061). The pass starts that card's implement job when the card is still `ready`, its dependencies are `done`, and the cap has room, and sets `processed_at` on that row when the job starts. A row without `card_id` wakes the whole project as above. The API never starts a job itself.

What runs today (RFC-062): `worker.Run` in `serve` starts one loop per project and retries a busy project lock on the next tick. A pass runs step 3 (promote), then a model-free step 5 that makes one `backlog` card per accepted requirement without a card through `loop.CreateCard`, then closes the project's rows that have no `card_id` when nothing changed. `worker.Drain` repeats passes until one changes nothing. Since RFC-063, each tick with any open row also runs `worker.StartQueued`: step 10 for card-scoped rows only, oldest first, under the cap, setting `processed_at` when a job starts. A `ready` card nobody queued is not started. Since RFC-071, the implement and review jobs write a card-scoped `retry` row after a failed attempt or an AI request for changes on a `continue` card, so a card a person ran keeps going until it opens a pull request that passes review or runs out of attempts. When a project loop takes its lock, it runs Reconcile step 1 and ends open attempts as `cancelled`. Since RFC-065, every tick, with or without an open row, runs `worker.Advancer`: step 6 starts a review job for each `pr_open` card, two at a time, and retries a review that failed before it moved the card after one minute. Steps 1, 2, and 7 run as one refresh per card through `app.RefreshCard`, at most every 30 seconds per card and two at a time. Steps 8 and 9 are not built. Step 4 runs inside the implement and review jobs as they end, and `app.ImplementCard` blocks a queued card at the cap instead of starting it (RFC-071).

Jobs run in goroutines beside that project's loop. Implement and plan jobs are capped by that project's `scheduler.implementationCap`. At most one deploy job runs per project. Review and verify jobs are capped at 2 each, per project. Every job takes its card lock with a fresh random holder and releases it with that holder. The job uses that project's repo and that project's data directory.

Each pass, in order:

1. Refresh head and CI. For cards in `agent_review` with a `recommend_approve` or `failed` verdict, or a `request_changes` verdict on a `manual` card (RFC-072), `human_review`, and `approved`, call `gh pr view` at most once every 30 seconds per card and store `head_sha`. An `agent_review` card with no verdict whose card lock is free is recorded as a failed review first (RFC-069). An `agent_review` card with `request_changes` that is no longer `manual`, and whose card lock is free, is sent back as in review step 4. A `changes_requested` card is visited on the same 30-second rhythm and `app.AtCap` blocks it with `attempt_cap` when no attempts remain (RFC-077).
2. A card in `approved` whose head moved goes to `human_review`, approval cleared.
3. Promote `backlog -> ready` when requirements are accepted, dependencies are `done`, and the card has acceptance lines or is manual.
4. A card in `changes_requested`, or in `in_progress` with no open attempt, whose `attempts_used >= max_attempts` goes to `blocked` with `attempt_cap`. The implement and review jobs apply the same rule as they end, and a card at the cap does not list `run` and is not started (RFC-071).
5. Start a plan job when `draftPending` is set, or when an accepted requirement has no linked card. One plan job at a time.
6. Start a review job for each `pr_open` card.
7. For `agent_review` cards waiting on CI: all required checks pass, or none required, then `human_review`. Every check on the pull request head counts as required (RFC-065). An empty list counts as none only `review.NoChecksWait` (2 minutes) after the card's latest `local_checks` gate (RFC-069). A required check fails, then `changes_requested` with a failure note of source `ci` naming the failed checks.
8. Start a deploy job for a card in `approved` only when a Lead Developer member has requested deploy and that project's deploy lock is free. Reaching `approved` does not start deploy.
9. Start a verify job for each `verifying` card.
10. Start implement jobs while running implement jobs are under the cap: oldest first among `ready`, `changes_requested`, and `in_progress` with no open attempt, whose requirements are accepted, dependencies are `done`, and `attempts_used < max_attempts`.

Every job snapshots that project's settings at start and uses that snapshot to the end. Clone and fetch use the company GitHub token (RFC-049). The push and the pull request use the card owner's verified token, decrypted at the start of the job and added to that job's redaction list (RFC-054). Before a job reads the repo, `runtime.EnsureCheckout` clones `https://<host>/<org>/<repo>.git` into the project's `repo/` when it is missing, else fetches the base branch. The token goes through askpass and the child environment, never argv.

### Plan job

1. Route `plan`. No model configured: set `planError` to `router_config` and stop. Until a model provider exists, the worker plans without a model instead (RFC-062).
2. When `draftPending`, the planner pack holds the PRD outline. `propose_requirements` writes rows with `source_kind = 'draft'` and `accepted_at` null, then clears `draftPending`.
3. Otherwise the pack holds accepted requirements without cards, decisions, existing card titles and dependencies, and the project's enabled skills as `SKILL <slug>: <use when>` lines (RFC-056). `create_card` takes `ref`, `title`, `requirementIds`, `decisionIds`, `dependsOn` (card ids or refs from this job), `risk`, `manual`, and `skillIds`.
4. The harness rejects a call that names an unknown or unaccepted requirement id, an unknown decision, an unknown dependency, or a dependency cycle. It assigns the card id, copies `checks.local` and `checks.dev` from settings, sets `max_attempts` from the router budget, and inserts the card in `backlog`. A command field in the arguments is ignored and logged as rejected. The owner is the `created_by` shared by the named requirements. Requirements with different creators are rejected with `ErrOwners`, so the planner splits them. An owner in the arguments is ignored (RFC-054). Each `skillIds` entry must be a skill enabled for the project and not disabled, else `ErrSkill` and no card is stored. The card's skills are stored in `card_skills` in the same transaction as the card (RFC-056).
5. Planner tokens are logged as `model_call` with no card id.

### Implement job

1. Take the card lock. For a real repo, require the company organization, bot login, and token, and the project repo, else block with `github_config`. Require an owner who is enabled and has a verified GitHub login and a token, else block with `owner_github` (`app.OwnerGitHub`). Require a sandbox image, else `sandbox_config`. Route the task class, and block with `router_config` when the route has no model.
2. When the worktree is missing, create it (git commands below) and store `branch`, `worktree_path`, and `worktree_gitdir`. On a later attempt, reuse it.
3. Record the local `refs/heads/<baseBranch>` SHA for the `base_moved` assertion.
4. Insert an attempt row: `n` is the card's highest `n` plus one, `parent_attempt_id` is the previous attempt, `base_sha` is the worktree HEAD.
5. If the card is `ready` or `changes_requested`, transition to `in_progress`. A card already `in_progress` with no open attempt stays there.
6. Read the card's rules and skills once with `conventions.ForCard`: enabled global rules, enabled rules of the project's enabled stacks, then the card's enabled skills. Append one `tool_call` event named `conventions_context` whose `items` field lists each `kind`, `id`, `version`, and `hash` (RFC-055). Build the implementer pack with them, the latest failure note of any source but `instructions`, the card's current instructions (`board.CurrentInstructions`, RFC-073), and no prior model messages. Run the tool loop.
7. Budget hit: end the attempt with outcome `budget`, add one to `attempts_used`, append `budget_stop` with `cap`, `turns`, and `maxTurns`, and block with `turn_cap`, `token_cap`, or `wall_cap` (RFC-081). Before that, when the loop stops without a commit and the last `run_local_check` exited 0 on the tree the worktree still has, the harness commits it with the card title and the trailer `Committed-By: harness after passing local check`, appending a `tool_call` named `commit` with `auto: true`. When there is still no commit but the worktree is clean and HEAD is ahead of the base branch, the attempt ends with `no_change` and HEAD goes to step 9 as the new commit (RFC-085).
8. No new commit since `base_sha`: end with `no_commit`, add one to `attempts_used`, and write a failure note with source `no_commit`. The card stays `in_progress`. If attempts remain, insert a card-scoped `runs` row with trigger `retry`, so step 10 starts the next attempt without a person pressing Run. If none remain, block with `attempt_cap` (RFC-071).
9. New commit: store `commit_sha`, transition `in_progress -> self_check`, and run the gating check on an export of that SHA (0006).
10. Exit `0`: `app.Publish` pushes the branch and ensures one pull request with the owner's token, rewrites its title and the harness region of its body (RFC-066), and requests the reviewers of the card's requirements. Then end with `checks_passed`, add one to `attempts_used` only after the publish succeeded (RFC-085), and transition `self_check -> pr_open`. An owner check that fails here blocks with `owner_github` before the push. A failed reviewer request appends `reviewers_requested` with `ok: false` and does not change the card.
11. Exit non-zero: store the log, run triage for a failure note, end with `checks_failed`, and add one to `attempts_used`. If attempts remain, transition `self_check -> in_progress` and insert a `retry` row as in step 8. If none remain, block with `attempt_cap`.
12. If the local base ref changed during the job, append `assertion_failed` and block with `base_moved`.
13. Release the card lock.

The implement job never calls `deploy`.

What runs today (RFC-063): `app.ImplementCard` moves `ready` or `changes_requested` to `in_progress` first, so the step 1 blocks are legal edges, and copies the project's `checks.local` onto a card that has none. The card lock is held during the tool loop only. A `maxWallMinutesPerAttempt` stop blocks with `wall_cap`. Step 11 writes the last 200 lines of the check as a `local_check` note instead of running triage. A check that cannot start blocks with `check_error`, and a failed push or pull request blocks with `publish_failed`. Since RFC-065, before step 2 on a real repo the job runs `<engine> version`, and a failure blocks with `sandbox_config` and a `job_error` without inserting an attempt. Every `job_error` goes through `app.jobError`, which redacts the message, drops NUL bytes, replaces invalid UTF-8, and keeps the first and last 4 KiB of a message over `app.JobErrorCap` (8 KiB). When the latest attempt ended with a commit and no outcome, or with `checks_passed` while the card has no pull request, and the worktree HEAD is that commit, the job skips steps 4 to 8 and gates and publishes that commit with that attempt. A `checks_passed` attempt is not counted again. A gating run whose engine exits 125 or 127 is the sandbox failing, not the check: the job writes a `job_error` and blocks with `sandbox_config` without adding to `attempts_used`. A provider error and a failed publish do not add to it either (RFC-085). Not built: triage and step 12.

### Tool loop

The provider receives the tool list for the profile. The harness executes each tool and returns the result as the next input of this attempt only.

| Profile | Tools |
| --- | --- |
| `planner` | `read_doc`, `propose_requirements`, `create_card` |
| `implementer` | `list_files`, `search_files`, `read_file`, `read_linked_file`, `replace_in_file`, `edit_file`, `delete_file`, `move_file`, `todo_write`, `run_local_check`, `run_command`, `commit` (RFC-064, RFC-080, RFC-082, RFC-084) |
| `reviewer` | `read_diff`, `comment_pr`, `verdict` |
| `triage` | `read_log`, `write_failure_note` |

`read_linked_file` takes `project` and `path`. `project` must be a linked project of the card's project (RFC-051). The harness exports that project's base SHA once per attempt to `exports/linked-<id>-<sha>/` and reads through `safePath` on that root, with the `read_file` cap. There is no write tool for a linked project.

The implementer pack (RFC-082) holds, inside the untrusted markers and after the rules and skills: each requirement as its heading (`REQ-x title`), its statement, and `acceptance:` with one `- ` line per acceptance item; the project's decisions whose statement names one of the card's requirement ids as `RFC-x: statement`, at most 20 in id order; the failure note and instructions; `PATH MAP` with the worktree's files to 4 levels, a directory at the last level shown as `dir/ (N files)`, the depth dropping until the map fits 12 KiB and then cut with `truncated: true`; and one `REPOSITORY FILE <path>` block per `AGENTS.md` and `CLAUDE.md` read from the root and from each existing directory a path in the card title or its requirements names, each cut at 16 KiB and all within 32 KiB. The path map and the walks of `list_files` and `search_files` never list `.git`, which is a file in a linked worktree, or `.orchestrator`. `todo_write(items)` replaces the attempt's list of up to 30 `{text, status}` items, status `pending`, `in_progress`, or `done` and text up to 200 bytes; the list goes in the newest turn's `Note` of every later request, outside the history budget, and touches no file. Consecutive read-only calls of one reply (`list_files`, `search_files`, `read_file`, `read_linked_file`) run concurrently; any other call runs alone and in order, and events and results keep the reply's order.

The implementer and reviewer packs add a `LINKED <name>` block per link inside the untrusted markers: that project's accepted requirements as `<project>:REQ-001`, its decisions, and the files named in the link's shared paths, read from the same export. The block is capped at 64 KiB per link, and the whole untrusted section stays under its cap. The attempt writes one `tool_call` event named `linked_context` whose `links` field lists each linked project id and SHA.

The implementer and reviewer packs open the untrusted section with the card's rules and skills (RFC-055, RFC-056). A rule renders as `CONVENTION <title> (<global or stack name>, <category>) v<n>` followed by its body. A skill renders as `SKILL <slug> v<n>: <title>`, a `use when:` line, and its body. Each block is cut at 16 KiB with `truncated: true`. The blocks share a 64 KiB section cap, and a block that does not fit becomes `CONVENTION <title> omitted: the conventions section is full` or the `SKILL` form of that line. The text passes through marker stripping and redaction like every untrusted section. It cannot add a tool: the tool list is the profile's constant.

Every file tool goes through `safePath` (0006). `list_files(path, glob, depth)` and `search_files(text, regex, path, glob, ignoreCase, context)` start from a `safePath` directory, skip `.git` and `.orchestrator`, skip `node_modules`, `vendor`, and `dist` below the start, do not follow symlinks, and return at most 500 paths or 100 matches (RFC-064, RFC-080). `read_file(path, offset, limit)` returns up to 2,000 numbered `N|text` lines and ends with `lines A-B of T` and, when lines remain, `more: read from line N`. `replace_in_file(path, old, new, replaceAll)` replaces exact text that occurs once, or every occurrence with `replaceAll`. `edit_file` refuses a history placeholder and empty text over a non-empty file. Both write a temporary file and rename it. `delete_file` and `move_file` take regular files only. A failed tool returns `error: <reason>`, which the `anthropic` client sends with `is_error`, and every result is redacted before the model sees it. A call whose arguments were not valid JSON returns an error to the model instead of ending the attempt (RFC-085). Each attempt first drops uncommitted changes in the worktree, so it starts from the last commit (RFC-064). `run_local_check` takes no arguments. It runs the card's `localCheck` on a working-tree export with network none, and returns the exit code and the last 200 lines. It never moves the card. `commit` stages the worktree and commits with the model's message plus fixed trailers:

`run_command(argv)` (RFC-084) takes an array of 1 to 64 strings, at most 4 KiB in total. `sandbox.ParseArgv` refuses a NUL, an `argv[0]` that is not a bare name, a shell or wrapper (0006), and a name off the project's `checks.commandExecutables`; with that list empty the reply is `error: run_command is not enabled for this project`. In fixture mode it replies `error: run_command needs the container sandbox`. An attempt runs at most 30 commands. Each call copies the worktree with `runtime.CopyWorktree` into a fresh `os.MkdirTemp` directory under `<data>/exports`, runs `sandbox.CheckExport` on it, and runs the argv through `sandbox.ExecuteArgv`, the same path `ExecuteCheck` uses after `ParseCheck`, in the pinned image with no network, a 5-minute timeout, and 1 MiB of output. The reply is `exit N` and the last 200 lines, redacted. The copy is deleted after the call, so file changes are discarded. The exit never sets the passing tree that the harness commits and never runs the card's check. `loop.Input.Exec` replaces `execx.Command` in tests.

```text
Orchestrator-Card: <card id>
Orchestrator-Attempt: <attempt id>
Model: <model id>
```

`verdict` accepts `recommend_approve` or `request_changes` only. Anything else is a parse failure. The second parse failure goes to the fallback model once. If that also fails to parse, the review is recorded as `failed` and the checks decide the card (RFC-069).

The loop ends after a `commit`, or at `maxModelTurnsPerAttempt` (default 40, at most 200), `maxTokensPerCard` summed across the card's attempts (default 2,000,000, checked before each model call), or `maxWallMinutesPerAttempt`. A reply without a tool call, or one cut at the token limit, gets a note asking for a tool call, at most twice per attempt; the third ends the loop with `no_commit`. When few turns remain (at most the larger of 3 and a fifth of the cap) the newest turn's `Note` says how many. The history sent back is capped by the routed model's `historyBudgetKiB` (default 16 KiB). Old results are replaced by a placeholder from the oldest first until the history fits in three quarters of the budget, and a replaced result stays replaced so the prefix does not change. A re-read of an unchanged file returns `unchanged since turn N`. Check output keeps its tail when cut (RFC-081).

### Review job

1. Take the card lock. Route `review` with the author model id of the latest attempt. Transition `pr_open -> agent_review`.
2. Build the reviewer pack from the card's rules and skills, read the same way as the implement job, linked requirements, decisions, the diff from base to head, the gating check output, the pull request body, and human comments. Omit author messages and tool traces. Write the pack JSON to `logs/<attempt-id>-review.json`.
3. Call the model. Store `review_model_id`, `review_prompt_hash`, `review_independent`, `review_verdict`, and `reviewed_sha` on the reviewed attempt. Post one pull request comment with the company token: the verdict, its reason, the model, whether the review is independent, and the reviewed SHA (RFC-066). A failed comment appends `job_error` and does not change the card.
4. `request_changes`: write a failure note with source `review` from the review body after `trace.Redact` with the job's secrets. Then `app.CardAIReviewMode` decides (RFC-072). On a `continue` card, `agent_review -> changes_requested` and a `retry` row in the same transaction, or `blocked` with `attempt_cap` when no attempts remain. On a `manual` card, apply the checks as in step 5, so green checks reach `human_review` with the verdict kept, and the pull request comment says a person decides. A breach of a rule or a skill checklist is a `request_changes` that names the rule or skill in the body.
5. `recommend_approve`: read required checks with `gh pr view --json statusCheckRollup`. No required checks: `human_review` with `ciRequired: false` in the gate. All required pass: `human_review` with `ciRequired: true`. Any required failure: `changes_requested` with a `ci` note, and in the same transaction a `retry` row, or `blocked` with `attempt_cap` when no attempts remain (`review.ApplyCIThen` with `app.queueOrCap`, RFC-077). A `manual` card gets no `retry` row (`app.capOnly`) but still blocks at the cap. Pending: stay in `agent_review` and let pass step 7 finish it.
6. No verdict (no routable review model, a parse failure after the fallback, a provider failure after retry and fallback, or another error while the job's context is live): append `job_error`, store `review_verdict = 'failed'` and `reviewed_sha`, post an "AI review: Not done" comment, then apply the checks as in step 5 (RFC-069).

A review never changes `attempts_used`.

What runs today (RFC-065): `app.ReviewCard` reads the pull request and the diff from local `refs/heads/<baseBranch>` to the attempt's commit, capped at 64 KiB, before step 1, and leaves the card in `pr_open` with a `job_error` when either fails. A missing company organization or token blocks with `github_config` after the move to `agent_review`. The route `different-family-from-author` picks a model through `router.ReviewModel`. The reviewer loop runs at most 8 turns, and `comment_pr` posts with the company token. Every reviewer request sets `MaxTokens` to `loop.ReviewMaxTokens` (32,768). The Anthropic client passes a request timeout of one hour per 128,000 tokens, at least 10 minutes, because without one the SDK refuses before sending any non-streaming request it estimates past 10 minutes. A reply with no tool call whose stop reason is `max_tokens` or `length` (`providers.Truncated`) is asked again once per run; a second one returns `loop.ErrReviewCut`, which wraps `ErrReviewParse`, so the fallback and the failed review apply as before. That failure writes the `job_error` "the reviewer gave no valid verdict: its reply hit the 32768-token limit twice" and the comment reason "The reviewer's reply hit the token limit before a verdict" (RFC-075). Every check on the head counts as required, except the harness's own `orchestrator/approval` status (RFC-068), and `ci_required` is stored on the attempt. Since RFC-069 the job reads the pull request again after the model before it applies the checks, keeping the first read when that fails or the head moved. `app.settleChecks` leaves a card with an empty check list in `agent_review` until `review.NoChecksWait` has passed since `board.PublishedAt`, the time of the latest `local_checks` gate. `JobDeps.Now` is the clock for that wait. `app.RefreshCard` finishes an `agent_review` card with no verdict through `finishStoppedReview` when it can take the card lock.

Retry (RFC-076): `app.RetryReview` takes the card lock, requires `agent_review` with a `failed` latest review (else `ErrReviewNotFailed`), and in one transaction moves `agent_review -> pr_open` with the attempt id and the actor as evidence and clears the review through `board.ClearReview`. That edge writes no `local_checks` gate, so `board.PublishedAt` keeps the first one. The worker's next pass starts this job for the `pr_open` card on the same attempt. HTTP `retry-review` and the CLI `retry-review CARD` call it.

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
| `Approve(actor, card, sha)` | `human_review` | Member, role may approve, GitHub login set, login is not the company bot, actor is not the card owner (RFC-054). With the project's `approval.solo` on, or when `CardSelfReview` is true because every requirement of the card has Self review on, the bot and owner checks are skipped and the approval stores `independent = false` (RFC-067, RFC-070). `sha` equals the refreshed head | `approved`, one approval row, `runs` row. Does not deploy. A new row posts the approval to GitHub (RFC-068) |
| `RequestChanges(actor, card, reason)` | `human_review`, `approved` | Member, role may request changes, reason at most 4000 characters | `changes_requested`, approval cleared, note with source `approver` holding the reason or `changes requested` in the same transaction. No `runs` row: the card waits for Run. With no attempts left, `blocked` with `attempt_cap` in the same transaction (RFC-077). From `approved`, the approval status turns `failure` |
| `SetInstructions(actor, card, text)` | any status but `done` | Member, role may run a card, text at most 4000 characters after CRLF becomes LF and outer whitespace is trimmed | No status change. A note with source `instructions`, or the same-second one updated, and an `instructions_saved` event. Blank text clears them, and unchanged text writes nothing. `QueueCardRun` with `instructions` does the same in the run's transaction (RFC-073) |
| `Reject(actor, card)` | `human_review`, `approved` | Member, role may reject | `blocked` with `rejected`. From `approved`, the approval status turns `failure` |
| `Deploy(actor, card)` | `approved` | Member, role is Lead Developer, `deploy` is in `actions` | `deploying_dev` through `Transition`, `runs` row with trigger `deploy` |
| `Unblock(actor, card, reset)` | `blocked` | Member, role may unblock, `unblockTo` is not null | `ready` or `human_review` per RFC-041, `runs` row |
| `Accept(actor, project, ids)` | intake | Member, role is Lead Developer, Senior Developer, QA Lead, or Product Owner (RFC-053) | `accepted_by` is the user id, `runs` row |
| `AddRequirement(actor, project, id, statement, acceptance, selfReview, aiReviewMode)` | intake | Member. A blank id takes the next free `REQ-` number under the project's `intake_state` row lock. `selfReview` and `aiReviewMode = 'manual'` need `CanSetSelfReview` (RFC-070, RFC-072) | `source_kind = 'manual'`, `created_by` is the user id, not accepted |
| `UpdateRequirement(actor, project, id, statement, acceptance)` | intake | Member, row is manual | A changed hash clears `accepted_at` and `accepted_by` |
| `DeleteRequirement(actor, project, id)` | intake | Member, row is manual, no card in the project lists the id | Row removed |
| `SetReviewers(actor, project, id, userIds)` | intake | Member, actor is the row's creator or a Lead Developer. Each reviewer is a tribe member with a verified login and is not the creator (RFC-054) | `requirement_reviewers` replaced for that row |
| `SetSelfReview(actor, project, id, on)` | intake | Member, `CanSetSelfReview`: the actor is a Lead Developer or an admin (RFC-070) | `requirements.self_review` set for that row |
| `SetAIReviewMode(actor, project, id, mode)` | intake | Member, `CanSetSelfReview`, mode is `continue` or `manual` (RFC-072) | `requirements.ai_review_mode` set for that row |
| `Ingest(actor, project)` | intake | Member | New parsed rows store `created_by` as the actor. The CLI passes no actor |

HTTP passes the session user. The break-glass CLI passes `--as`. A missing permission returns `app.ErrActionNotListed` when the name is not in `actions`, and `403` when the route is not a card action. A non-member is `404`.

Approving the same SHA twice finds the live approval row by the unique index and returns the card unchanged.

What runs today (RFC-068): `app.ApproveFresh` returns the card, whether it inserted a row, and whether the approval is independent. The HTTP route and the CLI call `app.AnnounceApproval` only for a new row. It posts a comment on the pull request and a commit status with context `app.ApprovalContext` (`orchestrator/approval`) and state `success` on `approvedSha`, with the company token, after the transition has committed. Request changes and Reject on an `approved` card call `app.WithdrawApproval`, which posts `failure` on the old `approvedSha`. Both detach from the request's cancel and stop after 30 seconds. Each failed post appends `job_error` with `approval comment:` or `approval status:` and leaves the card as it is. The company token needs commit-status write: `repo` on a classic token, or "Commit statuses: write" on a fine-grained one.

`app.ReadCardDiff` serves the card page's Changes panel. It reads `git diff --numstat` and `git diff --no-renames` from `refs/heads/<baseBranch>` to the head SHA in the project clone, fetching `refs/heads/orch/<cardId>` with the company token when the SHA is missing (`runtime.Git.FetchCard`). The text is capped at 256 KiB and each file at 1,500 lines, and is redacted with the model keys and the company token before `app.ParseDiff` splits it into files, hunks, and lines.

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
| Planner | PRD outline, requirement candidates, decision statements, card titles, dependencies, enabled skills by slug and "use when" |
| Implementer | This card, the card's rules and skills, linked statements, linked decisions, path list, `localCheck`, last failure note, the card's current instructions (RFC-073) |
| Reviewer | The card's rules and skills, linked statements, linked decisions, diff, gating check output, pull request body, human comments |
| Triage | Command, exit code, last 200 log lines, acceptance lines |

Snapshot fixtures live in `backend/internal/contextpack/testdata/`. The reviewer fixture contains the diff and does not contain the string `author-transcript`.

## Providers

```text
type Provider interface {
    Complete(ctx context.Context, req Request) (Response, error)
}
```

`Request` has `Model`, `System`, `User`, `Turns`, `Tools`, `MaxTokens`, `Thinking`, `ThinkingBudget`, and `Effort`. `Response` has text, tool calls, token counts, a stop reason, and `Reasoning`. Token counts are `InputTokens`, `OutputTokens`, and `CachedInputTokens`, the input read from the provider's prompt cache (RFC-083).

Prompt caching (RFC-083): the `anthropic` client marks `cache_control: {type: ephemeral}` on the system block, the last tool, and the last tool result of the newest turn, or on the pack when there is no turn yet. The newest turn's `Note` comes after that block, so the todo list and the turns-left note do not move the cached prefix, and the history keeps earlier turns byte-identical between requests until a chunk of old results is omitted (RFC-081). The client reports `cache_read_input_tokens` as `CachedInputTokens` and adds `cache_creation_input_tokens` to `InputTokens`. The `openai` client reads `prompt_tokens_details.cached_tokens` into `CachedInputTokens` and subtracts it from `InputTokens`; it sends no cache marks, because OpenAI-compatible servers cache a repeated prefix on their own. The implement loop adds each call's three counts to the attempt (`attempts.tokens_cached` for the cached one), and the token cap counts `input_tokens + output_tokens + tokens_cached / 10` over the card's attempts.

Thinking and effort come from the model's entry in the service `config.yaml` (RFC-079). `app.RoutedProvider` copies them onto each request from the model it is calling, so a fallback uses its own. Empty values send nothing. The anthropic client sends `thinking` and `output_config.effort`, and with `enabled` raises `max_tokens` by the budget (`providers.DefaultThinkingBudget`, 8,192, when zero). The openai client sends `reasoning_effort` and a `thinking` body field for compatible servers. Each client returns the reply's reasoning as `providers.Reasoning` tagged with the model: the signed `thinking` and `redacted_thinking` blocks, or `reasoning_content`. The loops keep it on the `Turn`, and a client sends a turn's reasoning back only when `Reasoning.Model` equals the request's model. It is never written to an event.

The `anthropic` protocol uses `anthropic-sdk-go`. The `openai` protocol uses `openai-go/v3` with the family's `baseUrl`, which also covers local OpenAI-compatible servers on loopback without a key. Keys come from the job's secrets snapshot. Logs receive the redacted request.

The clients live in `internal/providers/openai` and `internal/providers/anthropic`. Neither SDK retries or reads the environment. `Tool` has `Name`, `Description`, and `Parameters`, a JSON schema object. `internal/providers/llm` builds the client for a model from the family that lists it, and refuses a remote family without a key. `app.RoutedProvider` applies `router.Call` around those clients and wraps the final failure in `app.ErrProvider` (RFC-063).

Both clients, the fake, and the routed provider also implement `providers.Streamer` (RFC-087):

```go
type Streamer interface {
    Stream(ctx context.Context, req Request, onText func(string)) (Response, error)
}
```

`onText` gets each piece of reply text in order, and the `Response` is the whole reply with the same usage mapping as `Complete`. The anthropic client uses `Messages.NewStreaming` with the same params as `Complete`, calls `onText` for each `text_delta`, and rebuilds the message with `Message.Accumulate`. The openai client uses `Chat.Completions.NewStreaming` with `stream_options.include_usage`, calls `onText` for each content delta of choice 0, rebuilds the reply with `ChatCompletionAccumulator`, and reads usage, cached tokens included, from the final chunk. The routed stream keeps a `sent` flag: until the first text reaches the caller, the rate-limit wait, the retry, and the fallback behave as in `Complete`. After it, any error is wrapped in `router.ErrNoRetry`, which `router.Call` returns at once, because the caller has already shown part of the reply. A client without `Stream` is called through `Complete` and its text sent as one piece.

The routed provider also implements `providers.RestartStreamer` for the agent loops (RFC-088):

```go
type RestartStreamer interface {
    StreamRestart(ctx context.Context, req Request, onText func(string), onRestart func()) (Response, error)
}
```

`StreamRestart` keeps the rate-limit wait, the retry, and the fallback after text has been sent. Before it sends a reply again after text, it calls `onRestart`. Refine keeps `Stream` and its no-retry rule.

`trace.NewStreamRedactor(secrets, send)` redacts text that arrives in pieces. It drops NUL bytes, holds back the longest secret's length minus one byte, moves the cut back to the start of any secret that crosses it, and cuts on a UTF-8 boundary, so `send` never gets part of a secret. `Flush` sends the rest, and `Reset` drops it. Refine and the loops use it.

A fake implements `Provider` and replays a script file (0005). The REQ-043 fixture uses the fake. Live keys are not required for that suite.

## Git and GitHub

Every git call uses the prefix and environment in 0006, including the explicit `--git-dir` for worktree calls and `GIT_ASKPASS` for network calls.

Worktree creation for a real repo:

```text
git -C <repo> fetch <url> +refs/heads/<base>:refs/orchestrator/base
git -C <repo> worktree add -b orch/<card-id> <data>/worktrees/<card-id> refs/orchestrator/base
```

The fixture skips the fetch and uses the local base branch. `<url>` is `https://<host>/<owner>/<repo>.git`, built from settings. Fetching into `refs/orchestrator/base` leaves `refs/heads/<base>` alone.

Commit identity is the card owner (RFC-054): `-c user.name=<login> -c user.email=<id>+<login>@users.noreply.github.com`, with the owner's verified login and numeric GitHub id. A card without an owner, the fixture, and the break-glass CLI use the `runtime.Git.Author` they are given. Signing is off.

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

What runs today (RFC-066): the title is the requirement ids, then the card title, or the first line of the commit message when the card title names only the ids. The harness owns the text between `<!-- orchestrator:body:start -->` and `<!-- orchestrator:body:end -->` and rewrites it on each push: Summary (the commit message without trailers), Requirement (each id in bold on its own line, then the markdown statement as written, then its acceptance lines as a checklist; `closeFences` appends the closing line of a code fence the statement left open, so the statement cannot turn Changes and the rest into code), Changes (files with added and deleted lines, up to 50), Verification (the local check and the short SHA), Review, and the metadata block inside `<details>`. Text a person writes outside those markers is kept. A body with only the metadata markers is replaced. Cards carry no decision ids yet, so the metadata block has no `decisions` line.

`gh` commands, with `GH_TOKEN`, `GH_HOST`, and `GH_CONFIG_DIR` set:

```text
gh pr create --repo OWNER/REPO --base BASE --head orch/<card-id> --title TITLE --body-file BODY
gh pr edit URL --body-file BODY
gh pr comment URL --body-file COMMENT
gh pr view URL --json url,headRefOid,statusCheckRollup,reviews,comments
gh api user
gh api -X POST repos/OWNER/REPO/pulls/NUMBER/requested_reviewers -f reviewers[]=LOGIN
```

Push, `pr create`, `pr edit`, and the reviewer request carry the card owner's token. `pr view`, `pr comment`, and the company test carry the company token. If `cards.pr_url` is already set, skip `pr create`. The unique index is the second guard. Text from the model goes through `--body-file`, never through argv.

What runs today (RFC-066): `gh pr edit` and `gh pr comment` ask GraphQL for fields that need the `read:org` scope, so the harness writes through REST instead. The pull request edit is `gh api repos/OWNER/REPO/pulls/NUMBER --method PATCH --input FILE`, after reading the body with `gh api repos/OWNER/REPO/pulls/NUMBER --jq .body`. A comment is `gh api repos/OWNER/REPO/issues/NUMBER/comments --method POST --input FILE`, and the approval status is `gh api repos/OWNER/REPO/statuses/SHA --method POST --input FILE` (RFC-068). The file is JSON under the project's `logs/`, deleted after the call, so the title and text never reach argv.

## Settings

`settings.SaveLLM` and `SaveRouter` validate against the 0002 rules and the current state of the other section. Only an admin may call them (RFC-052). They write the service `config.yaml` and, only for keys that were sent, the service `secrets.json`. `settings.SaveProjectDeploy` validates against the 0002 rules for one project. A Lead Developer member may call it. It writes that project's `config.yaml` and, only when the deploy token was sent or cleared, that project's `secrets.json`. `company.Save` writes the company GitHub account and is admin only. The project repository and links are rows, saved by `tenancy.SaveRepository` and `tenancy.SaveLinks`. Files are written by temp file, `fsync`, and rename, with mode `0600`. Then they append `config_changed` with field names and `actorId`.

`settings.SaveUserGitHub` writes ciphertext for that user only. `settings.View*` copies config into response structs that have no secret fields, so `json.Marshal` cannot leak one.

`settings.Snapshot(projectID)` returns an immutable copy of that project's config and secrets for one job.

`conventions.SaveRule`, `SaveStack`, and `SaveSkill` are admin only (RFC-055, RFC-056). Each save validates against the 0002 rules, increments `version`, stores `content_hash`, and appends a `convention_revisions` row in one transaction. `conventions.Delete*` removes an unused item and disables one that a project or card still names. Revisions are never deleted. `conventions.SaveProject` sets a project's stacks added by hand and its enabled skills. A Lead Developer member may call it.

`app.Sync` runs `conventions.Detect` on the clone after a successful checkout. Detection walks the repo root and two directory levels below it. It skips `.git`, `node_modules`, `vendor`, and build output, does not follow links, and reads `package.json`, `pyproject.toml`, and `requirements*.txt` up to 1 MiB each. A stack matches when any of its marker files exists or any of its npm or Python dependencies is named. The sync replaces the project's `detected` rows, keeps its `manual` rows, and appends `stacks_detected` with the stack ids.

Saving `checks.local` or `checks.dev` updates cards in `backlog` and blocks cards past `backlog` (except `done`) with `source_changed`.

## Requirement refinement

`app.RefineField(ctx, data, db, projectID, actorID, in, provider, emit)` (RFC-086, RFC-087) refines one field, `in.Field` `statement` or `acceptance`. It checks the field name, checks the draft with `intake.Validate`, the save rules without saving, and loads the settings. With no model on `router.routes.implement` it returns `ErrRefineRoute`. These errors return before any `emit`. It then streams one request through the routed provider with that route's model and fallback, `TaskClass` `refine_requirement`, `MaxTokens` 4,096, a 2-minute timeout, no tools, and `contextpack.RefineStatementSystem` or `contextpack.RefineAcceptanceSystem`. Both ask for the new text only, with no preamble or code fence. `contextpack.RefineUser(field, ...)` puts the requirement id, the statement, the acceptance lines, and the member's guidance between the untrusted markers with marker lines stripped, and heads the field being refined "(refine this)" and the other "(context only)".

`emit("start", {modelId})` comes once, before the first text, with the model the routed provider is calling. Each piece of text passes through a redact buffer before `emit("delta", {text})`: it holds back the last `len(longest key) - 1` bytes, moves the cut back to the start of any key that crosses it, and never cuts inside a UTF-8 character, so a key split across pieces is replaced by `[redacted]` whole. When the reply ends, the buffer is flushed and the whole text, redacted, passes the save rules for its field: the statement through `intake.Validate`, or the non-blank lines as acceptance through `intake.Validate`, which drops list markers. A failure returns `ErrRefineProposal`; otherwise `emit("done", RefineDone{Text, Acceptance, ModelID})`. A provider error returns `ErrRefineProvider` with the keys redacted, and a canceled request context returns the cancel without an event. After a reply, the function appends a `requirement_refined` event with `field` and writes nothing else. `httpapi.Server.RefineProvider` replaces the routed provider in tests.

The handler writes the `200` and the `text/event-stream` headers on the first `emit`, then one `event:`/`data:` block per call, flushed through `http.Flusher`. No middleware wraps the writer and the server sets no write timeout, so each event leaves at once. An error before the first `emit` maps to JSON as in the table below; one after it is written as an `error` event. A closed connection cancels the request context, which stops the provider stream.

## HTTP

`internal/httpapi` maps sentinel errors:

| Sentinel | HTTP |
| --- | --- |
| `machine.ErrIllegal`, `machine.ErrEvidence`, `app.ErrActionNotListed` | 422 `transition_rejected` |
| `settings.ErrValidation` | 422 `validation` |
| `board.ErrNotFound` | 404 `not_found` |
| `board.ErrLocked`, `board.ErrConflict` | 409 `conflict` |
| `app.ErrForbidden` | 403 `forbidden` |
| `httpapi.errHost`, `httpapi.errOrigin` | 403 `forbidden` |
| `httpapi.errBody` | 413 `too_large` |
| `httpapi.errPaused` | 429 `rate_limited` |
| `app.ErrRefineRoute` | 409 `router_config` |
| `app.ErrRefineProposal`, `app.ErrRefineProvider` | 502 `provider_error` |

Middleware order: `Host` check, CORS and `Origin` check, body cap, bearer, project membership when the path has a project id, handler. The bearer lookup loads the user and rejects an expired or disabled session. CORS sends `Access-Control-Allow-Origin` with the exact configured origin, allows `Authorization` and `Content-Type`, and allows `GET`, `POST`, `PUT`, `DELETE`, and `OPTIONS`. A request with an `Origin` that does not match is `403`. Do not log passwords, bearer tokens, `tokenKey`, or GitHub tokens.

The worker runs on a context that is not the request context.

The trace route reads `trace.ListSince`, which orders the card's events by `ts` then `seq` and, with `since`, starts 10 seconds before it, and `board.CardAttempts`, which lists the card's attempts in the project by `n` with `max_turns`. `live` comes from the card's status, `app.CardRunState` for an open run request, and any attempt with no end (RFC-074). It runs no SQL of its own.

`live.Hub` holds each card's trace stream viewers and the turn being written (RFC-088). `Publish(cardID, frame)` never blocks: a viewer whose 256-frame buffer is full is dropped and its stream ends with `resync`. `Subscribe` returns the current turn's `turn` frame and its text so far (64 KiB at most), or `live.ErrFull` past 16 viewers for the card or 64 in all. `serve` makes one hub, sets it on `httpapi.Server.Live` and `JobDeps.Live`, and closes it in `http.Server.RegisterOnShutdown`, which ends every stream. `run-next` sets no hub.

`loop.Input.Live` and `loop.ReviewInput.Live` take a `live.Publisher`. When it is set, each model call goes through `StreamRestart`, else `Stream`, else `Complete`, and publishes `turn` with the attempt, turn, role, and requested model, then the reply text through `trace.NewStreamRedactor` with the job's secrets, `reset` on a restart, and `turn_end` after the `model_call` event is appended, or at once when the call fails. Reasoning and tool arguments are never published.

The trace stream route subscribes first, then reads the trace as the trace route does and sends it, then the hub snapshot. Its loop forwards hub frames, reads the card and `trace.ListSince` again every second and after `turn_end`, sends only events it has not sent, writes a ping comment every 15 seconds, and ends with `done` when `live` turns false or after 10 minutes. It has no 30-second handler timeout.

The usage route calls `app.Usage(ctx, db, data, actor, from, to, now)` (RFC-089), which returns `ErrForbidden` unless `actor.Admin` and then calls `app.ReadUsage`. `app.UsageWindow` checks the days and returns `UsageRangeError{Field, Message}`, which the handler maps to `422` with that field. `ReadUsage` reads `board.UsageRows(ctx, db, from, toExclusive)`, one SQL query over `events` with `kind IN ('model_call', 'requirement_refined')` and `ts` in the range, left-joined to `attempts`, `cards`, `projects`, and `users` and grouped by day, project, model, task, and user, then names each model's family from `settings.Load(data)`. It reads no attempt token columns, which miss the reviewer and refine. `board.UsageScope{TribeID, UserID}` (`app.UsageScope`) narrows the query to projects with that `tribe_id` and to calls whose user is that id; empty fields narrow nothing. A call counts in `COALESCE(cards.project_id, events.project_id)`, so an older card event stored with `proj_default` lands in the card's project (RFC-090). `app.TribeUsage(ctx, db, data, actor, tribeID, from, to, now)` calls `app.ManageTribe` (`tenancy.ManageTribe`: `ErrForbidden`, then `ErrNotFound`) before the range, and `app.MyUsage` reads with `UserID: actor.ID`. The three handlers share `serveUsage`, which maps `UsageRangeError` to `422` and the rest through `writeAccountErr`. `loop.modelCallPayload` writes the call's `taskClass`: the attempt's class in an implement session and `review` in the review loop.

The implement and review loops write `model_call.text` and `tool_call.args` through `trace.Clip` with the job's secrets. `JobDeps.Secrets` carries the `databaseUrl` and `tokenKey` text from `secrets.json`; `serve` and `run-next` set it, and both jobs add it to the model keys and GitHub tokens they redact.

## CLI

Cobra commands are thin. Example flow for approve:

1. Require `--project` and `--sha`.
2. Call `app.Approve(ctx, actor, cardID, sha)`.
3. Print the returned status and the SHA.
4. On error, print the message and exit `1`.

`usage` has no actor: the operator who can run the binary can read the board, so it calls `app.ReadUsage` with `--tribe` and `--user` as the scope, then `app.GroupUsage(rows, by)`, and prints one line per group with calls, input, output, cached, and total, largest total first. `--by` is `llm`, `model` (the default), `project`, `user`, or `task`; anything else exits `1`.

`users bootstrap` reads `ORCHESTRATOR_ADMIN_EMAIL` and `ORCHESTRATOR_ADMIN_PASSWORD`, refuses a short password, and inserts one admin only when `users` is empty.

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
| 13 | Users, sessions, account GitHub (Sprint 11) | Tribes |
| 14 | Tribes, projects, `project_id` (Sprint 12) | Per-project workers |
| 15 | Project workers, role gates, Lead deploy (Sprint 13) | |
| 16 | Manual requirements in `intake`, senior accept (Sprint 16) | |
| 17 | Card owner, owner pull requests, intake reviewers (Sprint 17) | |
| 18 | `conventions`, stack detection on Sync, rules and skills in packs (Sprint 18) | |

The first frontend starts after step 12. Account, tribe, and project pages land with steps 13 to 15.
