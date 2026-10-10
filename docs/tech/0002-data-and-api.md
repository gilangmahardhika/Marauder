# Data and API

Status: Accepted for implementation. Date: 2026-10-10. Revised after review.

This is the contract both processes implement. Stack choices are in [0001-implementation.md](0001-implementation.md). The backend build steps are in [0003-backend.md](0003-backend.md). The client is in [0004-frontend.md](0004-frontend.md). Security rules that constrain these fields are in [0006-security-and-harness.md](0006-security-and-harness.md).

If a field here and a PRD acceptance check disagree, the PRD wins. See [../README.md](../README.md).

## Conventions

- Times are UTC RFC3339 strings.
- Ids are text. Cards use `card_` plus 12 hex chars. Attempts use `att_`, events `evt_`, runs `run_`, each plus 12 hex chars.
- Requirement ids and decision ids are copied from markdown headings (`REQ-001`, `RFC-001`). The harness does not renumber them. Drafted ids from the planner use the same `REQ-` form and are stored with `source_kind = 'draft'`.
- SHAs are lowercase hex, 40 or 64 chars.
- SQLite booleans are integers `0` and `1`. JSON booleans are `true` and `false`.
- The API listens on `127.0.0.1:8787`. CORS allows the exact `--ui-origin`, default `http://127.0.0.1:4173`.
- The `Host` header must be `127.0.0.1:<port>`, `localhost:<port>`, or `[::1]:<port>`. Anything else is `403`. This blocks DNS rebinding.
- Every route except `POST /api/session` requires `Authorization: Bearer <token>`.
- JSON errors use one shape. Handlers do not invent a second shape.

```json
{
  "error": {
    "code": "validation",
    "message": "operatorLogin is not an approver",
    "fields": {
      "operatorLogin": "operatorLogin is not an approver"
    }
  }
}
```

`fields` is omitted when the error is not tied to an input.

| HTTP | `error.code` | When |
| --- | --- | --- |
| 401 | `unauthenticated` | Missing, unknown, or expired bearer, or a wrong startup token |
| 403 | `forbidden` | `Host` or `Origin` not allowed |
| 404 | `not_found` | Unknown card, family, or route |
| 409 | `conflict` | A lock is held, or the card status changed after it was read (RFC-040) |
| 413 | `too_large` | Body over 1 MiB |
| 422 | `validation` | Body failed a field check |
| 422 | `transition_rejected` | The action is not listed for the card, or evidence is missing |
| 429 | `rate_limited` | Session exchange paused after 5 failures |

A successful body never includes `error`.

## Statuses, groups, and actions

`group`, `actions`, and `unblockTo` are computed by the server on every read. The client displays them and does not recompute legality.

| Status | `group` | `actions` |
| --- | --- | --- |
| `backlog`, `ready` | `waiting` | `[]` |
| `in_progress`, `self_check` | `running` | `[]` |
| `pr_open`, `agent_review`, `changes_requested` | `review` | `[]` |
| `human_review` | `review` | `approve`, `request_changes`, `reject` |
| `approved` | `shipping` | `request_changes`, `reject` |
| `deploying_dev`, `verifying` | `shipping` | `[]` |
| `done` | `finished` | `[]` |
| `blocked` | `stopped` | `unblock`, unless release is refused |

## Blocked reasons

`blockedReason` is one of these. `unblockTo` follows RFC-041.

| Reason | Set when | `unblockTo` |
| --- | --- | --- |
| `turn_cap`, `token_cap`, `wall_cap` | A budget stopped the attempt | `ready` |
| `attempt_cap` | `attemptsUsed` reached `maxAttempts` | `ready` |
| `provider_error` | Primary and fallback both failed | `ready` |
| `review_parse` | Two reviewer replies had no parseable verdict | `ready` |
| `router_config` | The route for the needed task class has no model | `ready` |
| `github_config` | Owner, repo, bot login, or token missing | `ready` |
| `sandbox_config` | Sandbox image missing or not pinned, or a check command fails parsing | `ready` |
| `check_error` | A check could not start | `ready` |
| `base_moved` | The local base branch ref changed during a job | `ready` |
| `assertion` | A harness assertion failed | `ready` |
| `rejected` | The approver rejected the card | `ready` |
| `source_changed` | A linked requirement or a check command changed | `ready`, or `null` while a linked id is unaccepted |
| `deploy_config` | Deploy settings incomplete or invalid | `human_review` |
| `deploy_failed` | The deploy command exited non-zero. Rollback ran | `human_review` |
| `health` | The health probe failed. Rollback ran | `human_review` |

When `unblockTo` is `null` on a blocked card, `actions` is `[]`.

A check that times out or exceeds the output cap is a failed check with exit `124` or `125`. It is not `check_error`.

## SQLite

One file: `<data>/orchestrator.db`. Open it with foreign keys on, WAL, `busy_timeout=5000`, and one connection per process.

Migration `backend/migrations/00001_init.sql`:

```sql
-- +goose Up
CREATE TABLE requirements (
  id TEXT PRIMARY KEY,
  source_kind TEXT NOT NULL CHECK (source_kind IN ('prd', 'rfc', 'draft')),
  source_path TEXT NOT NULL,
  heading TEXT NOT NULL,
  statement TEXT NOT NULL,
  acceptance_json TEXT NOT NULL,
  non_goals_json TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  accepted_at TEXT,
  accepted_by TEXT
);

CREATE TABLE decisions (
  id TEXT PRIMARY KEY,
  source_path TEXT NOT NULL,
  statement TEXT NOT NULL,
  content_hash TEXT NOT NULL
);

CREATE TABLE cards (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  requirement_ids_json TEXT NOT NULL,
  decision_ids_json TEXT NOT NULL,
  depends_on_json TEXT NOT NULL,
  risk TEXT NOT NULL CHECK (risk IN ('low', 'high')),
  service TEXT NOT NULL DEFAULT 'default',
  status TEXT NOT NULL CHECK (status IN (
    'backlog', 'ready', 'in_progress', 'self_check', 'pr_open', 'agent_review',
    'changes_requested', 'human_review', 'approved', 'deploying_dev',
    'verifying', 'done', 'blocked')),
  manual INTEGER NOT NULL DEFAULT 0,
  local_check TEXT,
  dev_check TEXT,
  attempts_used INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL,
  branch TEXT,
  worktree_path TEXT,
  worktree_gitdir TEXT,
  pr_url TEXT,
  head_sha TEXT,
  approved_sha TEXT,
  deployed_sha TEXT,
  environment TEXT NOT NULL DEFAULT 'dev' CHECK (environment = 'dev'),
  failure_reason TEXT,
  blocked_reason TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX cards_branch ON cards(branch) WHERE branch IS NOT NULL;
CREATE UNIQUE INDEX cards_pr ON cards(pr_url) WHERE pr_url IS NOT NULL;
CREATE INDEX cards_status ON cards(status);

CREATE TABLE attempts (
  id TEXT PRIMARY KEY,
  card_id TEXT NOT NULL REFERENCES cards(id),
  n INTEGER NOT NULL,
  parent_attempt_id TEXT,
  task_class TEXT NOT NULL CHECK (task_class IN ('implement', 'implement_hard')),
  model_id TEXT,
  fallback_model_id TEXT,
  prompt_hash TEXT,
  system_hash TEXT,
  tool_profile TEXT,
  context_pack TEXT,
  max_turns INTEGER,
  input_tokens INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  base_sha TEXT,
  commit_sha TEXT,
  review_model_id TEXT,
  review_prompt_hash TEXT,
  review_independent INTEGER,
  review_verdict TEXT CHECK (review_verdict IN ('recommend_approve', 'request_changes')),
  reviewed_sha TEXT,
  ci_required INTEGER,
  started_at TEXT NOT NULL,
  ended_at TEXT,
  outcome TEXT CHECK (outcome IN (
    'checks_passed', 'checks_failed', 'no_commit', 'budget', 'error', 'cancelled')),
  UNIQUE (card_id, n)
);

CREATE TABLE locks (
  card_id TEXT PRIMARY KEY REFERENCES cards(id),
  holder TEXT NOT NULL,
  acquired_at TEXT NOT NULL
);

CREATE TABLE deploy_lock (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  card_id TEXT,
  holder TEXT,
  acquired_at TEXT
);
INSERT INTO deploy_lock (id) VALUES (1);

CREATE TABLE gates (
  id TEXT PRIMARY KEY,
  card_id TEXT NOT NULL REFERENCES cards(id),
  attempt_id TEXT,
  name TEXT NOT NULL CHECK (name IN (
    'local_checks', 'agent_review', 'human_approval', 'dev_health', 'dev_tests')),
  passed INTEGER NOT NULL,
  evidence_ref TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE events (
  id TEXT PRIMARY KEY,
  ts TEXT NOT NULL,
  card_id TEXT,
  attempt_id TEXT,
  kind TEXT NOT NULL,
  payload_json TEXT NOT NULL
);
CREATE INDEX events_card_ts ON events(card_id, ts);

-- +goose StatementBegin
CREATE TRIGGER events_no_update BEFORE UPDATE ON events
BEGIN
  SELECT RAISE(ABORT, 'events are insert-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER events_no_delete BEFORE DELETE ON events
BEGIN
  SELECT RAISE(ABORT, 'events are insert-only');
END;
-- +goose StatementEnd

CREATE TABLE approvals (
  id TEXT PRIMARY KEY,
  card_id TEXT NOT NULL REFERENCES cards(id),
  sha TEXT NOT NULL,
  approver TEXT NOT NULL,
  created_at TEXT NOT NULL,
  cleared_at TEXT
);
CREATE UNIQUE INDEX approvals_live ON approvals(card_id, sha) WHERE cleared_at IS NULL;

CREATE TABLE failure_notes (
  id TEXT PRIMARY KEY,
  card_id TEXT NOT NULL REFERENCES cards(id),
  attempt_id TEXT NOT NULL,
  source TEXT NOT NULL CHECK (source IN (
    'local_check', 'review', 'ci', 'approver', 'dev_check', 'no_commit')),
  command TEXT,
  exit_code INTEGER,
  note TEXT NOT NULL,
  log_ref TEXT,
  model_id TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE runs (
  id TEXT PRIMARY KEY,
  trigger TEXT NOT NULL CHECK (trigger IN (
    'api', 'cli', 'accept', 'approve', 'request_changes', 'unblock')),
  created_at TEXT NOT NULL,
  processed_at TEXT
);

-- +goose Down
DROP TABLE runs;
DROP TABLE failure_notes;
DROP TABLE approvals;
DROP TRIGGER events_no_delete;
DROP TRIGGER events_no_update;
DROP TABLE events;
DROP TABLE gates;
DROP TABLE deploy_lock;
DROP TABLE locks;
DROP TABLE attempts;
DROP TABLE cards;
DROP TABLE decisions;
DROP TABLE requirements;
```

v1 ships one migration. A later schema change is `00002_*.sql`. Do not edit `00001` after the first release.

Column notes:

- `content_hash` is SHA-256 hex of the statement plus the acceptance list, or of the decision statement. Intake uses it to detect edits (REQ-004).
- `attempts_used` counts attempts with a counted outcome (RFC-042). `cancelled` does not count. `attempts.n` keeps increasing across releases and resets.
- `worktree_gitdir` is read from the worktree's `.git` file once, right after `git worktree add`. Later git calls pass it explicitly (0006).
- Review columns sit on the implementer attempt whose commit was reviewed. `reviewed_sha` is the head at review time.
- The token cap sums `input_tokens + output_tokens` across every model call for the card, including review and triage calls, which are recorded in `model_call` events.

Event `kind` is one of: `route`, `model_call`, `tool_call`, `gate`, `transition`, `transition_rejected`, `deploy`, `health`, `verify`, `budget_stop`, `config_changed`, `assertion_failed`.

| Kind | Payload fields |
| --- | --- |
| `route` | `taskClass`, `modelId`, `fallback`, `familyFlag`, `reason` |
| `model_call` | `role`, `modelId`, `promptHash`, `inputTokens`, `outputTokens`, `stopReason` |
| `tool_call` | `name`, `ok`, `rejected`, `summary` |
| `transition` | `from`, `to`, `evidence`, plus `resetAttempts` and `maxAttempts` on release |
| `deploy` | `requestedSha`, `previousSha`, `devUrl`, `exitCode`, `reportedSha` |
| `health` | `passed`, `status`, `bytes`, `rollbackExit` |
| `verify` | `sha`, `mode` (`health_only` or `dev_check`), `exitCode`, `logRef` |
| `config_changed` | `section`, `fields` |
| `assertion_failed` | `name`, `detail` |

Every payload passes through `redact` before insert.

## Card JSON

```json
{
  "id": "card_0123456789ab",
  "title": "Parse PRD headings",
  "status": "human_review",
  "group": "review",
  "actions": ["approve", "request_changes", "reject"],
  "unblockTo": null,
  "requirementIds": ["REQ-001"],
  "decisionIds": ["RFC-018"],
  "dependsOn": [],
  "risk": "low",
  "service": "default",
  "manual": false,
  "localCheck": "go test ./...",
  "devCheck": null,
  "attemptsUsed": 1,
  "maxAttempts": 3,
  "branch": "orch/card_0123456789ab",
  "prUrl": "https://github.com/acme/widget/pull/12",
  "headSha": "9c1e5b7d0a4f2e6c8b3d1a7f5e9c2b4d6a8f0e1c",
  "reviewedSha": "9c1e5b7d0a4f2e6c8b3d1a7f5e9c2b4d6a8f0e1c",
  "reviewIndependent": true,
  "approvedSha": null,
  "deployedSha": null,
  "environment": "dev",
  "failureReason": null,
  "blockedReason": null,
  "updatedAt": "2026-10-10T05:00:00Z"
}
```

`reviewedSha` and `reviewIndependent` come from the latest attempt with a review verdict. `headSha` is the value from the last refresh, not a live call.

## Session

`POST /api/session`

```json
{ "token": "<token printed by serve>" }
```

```json
{ "bearer": "<random>", "expiresAt": "2026-10-10T17:00:00Z" }
```

The startup token is compared in constant time. The bearer is random, stored only in process memory, and dies when the process exits or after 12 hours. A wrong startup token is `401`. Five wrong tokens pause new exchanges for one minute with `429`.

## Board and intake

`GET /api/cards`

```json
{
  "canRun": true,
  "running": false,
  "cards": []
}
```

An empty store returns `canRun: false`, `running: false`, and `cards: []`. It does not return an error.

`canRun` is true when the worker could start something now: a planning job is needed, a `backlog` card can be promoted, a `ready` or `changes_requested` card is under the cap, a `pr_open` card awaits review, an `approved` card has a free deploy lock, or a `verifying` card awaits its check. `running` is true while an unprocessed `runs` row exists.

`GET /api/cards/{id}` returns one card object. Unknown id is `404`.

`GET /api/cards/{id}/trace`

```json
{
  "events": [
    {
      "id": "evt_0123456789ab",
      "ts": "2026-10-10T05:00:00Z",
      "attemptId": "att_0123456789ab",
      "kind": "transition",
      "payload": { "from": "ready", "to": "in_progress", "evidence": "lock held, route stored" }
    }
  ]
}
```

Oldest first. Payload is the redacted JSON already stored.

`GET /api/intake`

```json
{
  "draftPending": false,
  "planError": null,
  "requirements": [
    {
      "id": "REQ-001",
      "sourceKind": "prd",
      "sourcePath": "docs/prd/coding-orchestrator.md",
      "heading": "REQ-001 Parse PRD and RFC markdown into records",
      "statement": "The orchestrator reads markdown PRDs and RFCs.",
      "acceptance": ["A fixture PRD yields one record per id."],
      "accepted": false,
      "linkedCards": []
    }
  ],
  "decisions": [
    {
      "id": "RFC-001",
      "sourcePath": "docs/rfc/0001-v1-architecture.md",
      "statement": "Card status changes only inside the harness transition function."
    }
  ]
}
```

`POST /api/ingest` has an empty body. It reparses the configured PRD and RFC paths and returns the `GET /api/intake` shape. Unchanged hashes do not duplicate rows. A changed hash clears `accepted_at` for that id and blocks linked cards past `backlog` with `source_changed` (REQ-004). When a PRD has no `REQ-` headings, the response has `draftPending: true` and the worker runs the planner to draft ids (REQ-002). `planError` holds the last planner failure message, redacted, until the next successful plan.

`POST /api/accept`

```json
{ "requirementIds": ["REQ-001", "REQ-002"] }
```

Stores the operator login and the time on those ids, then inserts a `runs` row with trigger `accept` so the planner can create cards. Ids absent from the body are left as they are. Unknown ids are `422`.

## Runs and human gates

`POST /api/runs` has an empty body. It inserts a `runs` row and returns immediately. The worker processes it until idle (0003).

```json
{ "runId": "run_0123456789ab", "canRun": true }
```

When nothing can move, the response is still `200`:

```json
{ "runId": "run_0123456789ab", "canRun": false, "reason": "no card is ready" }
```

`POST /api/cards/{id}/approve`

```json
{ "sha": "9c1e5b7d0a4f2e6c8b3d1a7f5e9c2b4d6a8f0e1c" }
```

The approver is `operatorLogin` from settings. The server refreshes the head from GitHub, checks the allowlist, rejects the bot login, and checks that `sha` equals the head. Approving the same card and SHA twice returns the card and does not insert a second approval. A SHA mismatch is `422` with `fields.sha`. An empty `operatorLogin` is `422` with `fields.operatorLogin`.

`POST /api/cards/{id}/request-changes` and `POST /api/cards/{id}/reject` have empty bodies. Both work from `human_review` and `approved`. Request-changes clears approval and writes a failure note with source `approver`. Reject sets `blockedReason` to `rejected`.

`POST /api/cards/{id}/unblock`

```json
{ "resetAttempts": false }
```

The body is optional. The card goes to `unblockTo`. With `resetAttempts: false`, `attemptsUsed` stays, and `maxAttempts` rises to `attemptsUsed + 1` when the count is already at the cap. With `resetAttempts: true`, `attemptsUsed` becomes `0` and `maxAttempts` returns to the router budget.

Approve, request-changes, and unblock insert a `runs` row with the matching trigger so the board continues without a second click. Each route returns the card object. An action not listed for the card is `422` `transition_rejected`. A status change between read and write is `409` `conflict`.

## Settings

Secret fields are write-only.

| Rule | Behavior |
| --- | --- |
| GET | `tokenSet` or `keySet` is boolean. The secret value is absent |
| PUT field omitted or empty | Stored secret stays |
| PUT field with a new string | Stored secret is replaced. Minimum 8 characters |
| PUT `clear: true` | Stored secret is deleted and the flag becomes false |
| Event | `config_changed` records the section and the field names, not the values |
| In-flight job | Keeps the settings snapshot it started with |
| Failure | `422` with `fields`. Files on disk are unchanged |

`GET /api/settings/github`

```json
{
  "host": "github.com",
  "owner": "",
  "repo": "",
  "baseBranch": "main",
  "approvers": [],
  "botLogin": "",
  "tokenSet": false
}
```

`PUT /api/settings/github` accepts the same fields plus optional `token` and `clear`.

| Field | Rule |
| --- | --- |
| `host` | Hostname only, no scheme or path |
| `owner`, `repo` | `^[A-Za-z0-9._-]+$` |
| `baseBranch` | Valid by `git check-ref-format --branch`, and not `orch/*` |
| `approvers` | Unique logins. Must not contain `botLogin` |
| `botLogin` | Required before a real repo job runs. Used for the commit identity |

`POST /api/settings/github/test` uses the stored token and returns `{ "ok": true, "login": "octocat" }` or `{ "ok": false, "message": "authentication failed" }`. The token is in neither body. A completed check is `200`, including `ok: false`. No stored token is `422`.

`GET /api/settings/llm`

```json
{
  "families": [
    {
      "id": "anthropic",
      "protocol": "anthropic",
      "baseUrl": "https://api.anthropic.com",
      "models": [],
      "keySet": false
    }
  ]
}
```

`PUT /api/settings/llm`

```json
{
  "families": [
    {
      "id": "anthropic",
      "protocol": "anthropic",
      "baseUrl": "https://api.anthropic.com",
      "models": ["claude-example"],
      "apiKey": "optional-new-key",
      "clear": false
    }
  ]
}
```

| Field | Rule |
| --- | --- |
| `id` | `^[a-z0-9-]+$`, unique |
| `protocol` | `anthropic` or `openai` |
| `baseUrl` | `https`, or `http` on `127.0.0.1`, `localhost`, or `::1` for a local server |
| `models` | Non-empty strings, unique across all families, so a model id names one family |
| `apiKey` | Optional for a loopback `baseUrl`, required for the family to be usable otherwise |

Removing a family whose models are used by the router is `422` on `families`.

`POST /api/settings/llm/{family}/test` returns `{ "ok": true }` or `{ "ok": false, "message": "..." }` with `200`. Unknown family is `404`. A family that needs a key and has none is `422`.

`GET /api/settings/router`

```json
{
  "routes": {
    "plan": { "model": "", "fallback": "" },
    "implement": { "model": "", "fallback": "" },
    "implement_hard": { "model": "", "fallback": "" },
    "review": { "model": "different-family-from-author", "fallback": "" },
    "triage": { "model": "", "fallback": "" }
  },
  "budgets": {
    "maxAttempts": 3,
    "maxModelTurnsPerAttempt": 12,
    "maxTokensPerCard": 200000,
    "maxWallMinutesPerAttempt": 30
  },
  "reviewIndependentPossible": false
}
```

`reviewIndependentPossible` is true when two or more families each have at least one model and are usable.

`PUT /api/settings/router` sends `routes` and `budgets`. A non-empty model or fallback must exist on a saved family. The review sentinel `different-family-from-author` is allowed only on `review.model`. `fallback` may be empty, which means no fallback. An empty `model` may be saved. A job that needs that class then blocks with `router_config`. Budgets are integers greater than zero, with `maxAttempts` at most 10.

`GET /api/settings/deploy`

```json
{
  "environment": "dev",
  "devUrl": "",
  "devHosts": [],
  "devCommand": "",
  "rollbackCommand": "",
  "healthUrl": "",
  "deployTokenSet": false,
  "localCheck": "",
  "devCheck": "",
  "allowExecutables": [],
  "sandboxEngine": "podman",
  "sandboxImage": "",
  "implementationCap": 2,
  "prdPath": "docs/prd",
  "rfcPath": "docs/rfc",
  "operatorLogin": ""
}
```

`PUT /api/settings/deploy` accepts those fields plus optional `deployToken` and `clear`.

| Field | Rule when non-empty |
| --- | --- |
| `environment` | Always `dev` |
| `devHosts` | Lowercase hostnames or IP literals. No scheme, port, or wildcard |
| `devUrl`, `healthUrl` | Parse as a URL with no userinfo. Scheme `https`, or fixture `http://127.0.0.1`. Host in `devHosts` |
| `devCommand`, `rollbackCommand` | Absolute path to a regular file, outside the target repo and the data directory, owned by the current user, executable, not group or world writable (RFC-038) |
| `localCheck`, `devCheck` | Parses under the check rule in 0006. First word in `allowExecutables` |
| `allowExecutables` | Bare names, `^[A-Za-z0-9._+-]+$` |
| `sandboxEngine` | `podman` or `docker` |
| `sandboxImage` | Contains `@sha256:` followed by 64 hex chars |
| `implementationCap` | Integer from 1 to 8 |
| `prdPath`, `rfcPath` | Relative, no `..`, inside the target repo |
| `operatorLogin` | In the saved approver list |

Empty deploy fields may be saved. A deploy then blocks with `deploy_config` and runs no command. A non-empty `devCheck` with an empty `devUrl` is also `deploy_config` at deploy time. Changing `localCheck` or `devCheck` applies the rule in 0006 to existing cards.

## Files on disk

The data directory defaults to `~/.orchestrator/<repo-name>-<hash8>`, where `hash8` is the first 8 hex chars of SHA-256 of the absolute repo path (RFC-036).

```text
<data>/                     mode 0700
<data>/config.yaml          mode 0600
<data>/secrets.json         mode 0600
<data>/orchestrator.db
<data>/worker.lock
<data>/empty-hooks/
<data>/empty-gh/
<data>/askpass/
<data>/worktrees/<card-id>/
<data>/exports/<card-id>-<sha>/   deleted after each check
<data>/logs/<attempt-id>.log
<data>/logs/<attempt-id>-review.json
```

`config.yaml`:

```yaml
github:
  host: github.com
  owner: ""
  repo: ""
  baseBranch: main
  approvers: []
  botLogin: ""
llm:
  families:
    - id: anthropic
      protocol: anthropic
      baseUrl: https://api.anthropic.com
      models: []
    - id: openai
      protocol: openai
      baseUrl: https://api.openai.com/v1
      models: []
router:
  routes:
    plan: { model: "", fallback: "" }
    implement: { model: "", fallback: "" }
    implement_hard: { model: "", fallback: "" }
    review: { model: different-family-from-author, fallback: "" }
    triage: { model: "", fallback: "" }
  budgets:
    maxAttempts: 3
    maxModelTurnsPerAttempt: 12
    maxTokensPerCard: 200000
    maxWallMinutesPerAttempt: 30
deploy:
  environment: dev
  devUrl: ""
  devHosts: []
  devCommand: ""
  rollbackCommand: ""
  healthUrl: ""
checks:
  local: ""
  dev: ""
  allowExecutables: []
sandbox:
  engine: podman
  image: ""
scheduler:
  implementationCap: 2
paths:
  prd: docs/prd
  rfc: docs/rfc
operatorLogin: ""
```

The deploy settings route flattens `deploy`, `checks`, `sandbox`, `scheduler`, `paths`, and `operatorLogin` into one object, as shown above. `secrets.json`:

```json
{
  "githubToken": "",
  "deployToken": "",
  "llmKeys": {
    "anthropic": "",
    "openai": ""
  }
}
```

Empty string means unset. Both files are written to a temp file in the same directory, synced, and renamed. The API never decodes `secrets.json` into a struct that is encoded to the client. Edit `config.yaml` by hand only while `serve` is stopped. Startup validates it with the same rules as the PUT routes.

## CLI twins

The CLI calls the same `internal/app` functions, not the HTTP API. It prints the error `message` on stderr and exits `1` on any error. It exits `0` on success, including a run where `canRun` is false.

| CLI | API |
| --- | --- |
| `orchestrator ingest` | `POST /api/ingest` |
| `orchestrator accept` | `POST /api/accept` with every current unaccepted id |
| `orchestrator run-next` | `POST /api/runs`, and runs the worker itself when `serve` is not running (RFC-043) |
| `orchestrator status` | `GET /api/cards` |
| `orchestrator status CARD` | `GET /api/cards/{id}` |
| `orchestrator trace CARD` | `GET /api/cards/{id}/trace` |
| `orchestrator approve CARD --sha SHA` | `POST /api/cards/{id}/approve` |
| `orchestrator request-changes CARD` | `POST /api/cards/{id}/request-changes` |
| `orchestrator reject CARD` | `POST /api/cards/{id}/reject` |
| `orchestrator unblock CARD [--reset-attempts]` | `POST /api/cards/{id}/unblock` |
| `orchestrator serve` | Starts the API and the worker |

`approve` without `--sha` exits `1` before any transition. `status` with no cards prints `board is empty` and the next command `orchestrator ingest`. Settings have no CLI twin in v1. Use the UI, or edit `config.yaml` while `serve` is stopped.
