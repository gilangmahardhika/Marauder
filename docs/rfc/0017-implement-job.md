# RFC-0017: Queued cards run the implement job with real models

Status: Accepted for implementation. Date: 2026-10-11.

This RFC adds decision `RFC-063`. It supersedes in part step 10 of the worker pass in [tech/0003](../tech/0003-backend.md), which starts implement jobs for every `ready` card under the cap. It also supersedes in part RFC-061, where only a `ready` card can be queued. RFC-062's "a queued card stays queued" no longer holds.

Sprint 26 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-063 | The worker runs the implement job for queued cards, with real `openai` and `anthropic` clients, up to `pr_open` | tech/0003 pass step 10 for cards nobody queued; RFC-061's ready-only Run |

## RFC-063 The worker runs the implement job for queued cards, with real `openai` and `anthropic` clients, up to `pr_open`

### Model clients

`internal/providers/openai` uses `openai-go/v3` against the family `baseUrl`, which covers OpenAI, Groq, and any OpenAI-compatible server. `internal/providers/anthropic` uses `anthropic-sdk-go` against the family `baseUrl`, or the Anthropic API when it is empty, which covers Anthropic and Anthropic-compatible servers such as Deepseek. Neither SDK retries and neither reads credentials from the environment. Tools carry a description and a JSON schema. A family without a key is accepted only when its `baseUrl` is a loopback address. Errors are redacted with the key before they leave the client.

`llm.ForModel` finds the family that lists a model and builds the client for its protocol. The implement job sends each model call through the router: the route's model, one retry, then the route's fallback. When both fail, the attempt ends with outcome `error` and the card is blocked with `provider_error`.

`POST /api/settings/llm/{family}/test` sends one short request to the family's first model with a 20 second timeout. It returns `{"ok": true}`, or `{"ok": false, "message": ...}` with the redacted provider error. An unknown family is 404.

### Which cards run

The worker starts an implement job only for a card with an open card-scoped `runs` row (RFC-061), oldest first, while that project's running implement jobs are under `scheduler.implementationCap` (default 2). It sets `processed_at` on the row when the job starts. A `ready` card nobody queued does not start, so no model tokens are spent until a person presses Run.

Run is offered and accepted for a card in `ready`, `changes_requested`, or `in_progress` with no open attempt, whose dependencies are `done`. "Run ready cards (N)" still queues only `ready` cards.

### The job

1. Move `ready` or `changes_requested` to `in_progress`, so a preflight failure is a legal block.
2. Preflight, each failure blocks with its reason: company organization, bot login, token, and project repo (`github_config`); the owner's verified GitHub token (`owner_github`); a local check that parses and is allowlisted, and outside fixture mode a `podman` or `docker` binary and an image pinned by digest (`sandbox_config`); a route for the task class whose model has a client (`router_config`).
3. A card made before the project had a local check gets the project's current `checks.local` copied onto it.
4. Fetch the base branch with the company token, then run one attempt of the tool loop with the card owner as author, `maxModelTurnsPerAttempt`, and a `maxWallMinutesPerAttempt` deadline. A deadline stop adds one to `attempts_used` and blocks with `wall_cap`.
5. No new commit: the card stays `in_progress` with a `no_commit` note, or is blocked with `attempt_cap` when no attempts remain.
6. New commit: `in_progress -> self_check`, then the gating check runs on an export of that SHA. Outside fixture mode it runs as `<engine> run --rm --pull=never --network=none ...` with the export mounted at `/work` (tech/0006). Every gating run adds one to `attempts_used`.
7. Exit `0`: outcome `checks_passed`, then `app.Publish` pushes with the owner's token, opens the pull request, and moves `self_check -> pr_open` with the exit code and the URL.
8. Exit non-zero: outcome `checks_failed`, a `local_check` failure note with the last 200 lines, then `self_check -> in_progress`, or `attempt_cap` when no attempts remain.
9. A check that cannot start blocks with `check_error`. A push or pull request failure blocks with `publish_failed`.

Each failure that is not a status change appends a `job_error` event with the redacted message. When a project loop takes its lock, it ends that project's open attempts as `cancelled` and deletes its card locks, because no job of that project is alive.

Before this, Run only recorded a request, no code called a model, and a real check never ran, so a queued card stayed queued.

Set aside: the AI review job after `pr_open`, triage for failure notes, the `turn_cap` and `token_cap` stops, and the `base_moved` assertion. The check runs with no network on a fresh export, so its image must already hold the toolchain and the dependencies the check needs.

Consequences: an admin's Test connection now spends a few tokens. A Lead Developer must set a local check, its executable, the engine, and a pinned image before a non-fixture card can run. When the company bot login is the same account as the card owner, the pull request author and the approver are one person.

Requirements: REQ-069.
