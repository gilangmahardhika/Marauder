# Data and API

Status: Accepted for implementation. Date: 2026-10-10. Revised after review.

This is the contract both processes implement. Stack choices are in [0001-implementation.md](0001-implementation.md). The backend build steps are in [0003-backend.md](0003-backend.md). The client is in [0004-frontend.md](0004-frontend.md). Security rules that constrain these fields are in [0006-security-and-harness.md](0006-security-and-harness.md).

If a field here and a PRD acceptance check disagree, the PRD wins. See [../README.md](../README.md).

## Conventions

- Times are UTC RFC3339 strings.
- Ids are text. Cards use `card_` plus 12 hex chars. Attempts use `att_`, events `evt_`, runs `run_`, users `user_`, sessions `ses_`, tribes `tribe_`, and projects `proj_`, each plus 12 hex chars.
- Requirement ids and decision ids are copied from markdown headings (`REQ-001`, `RFC-001`). The harness does not renumber them. Drafted ids from the planner use the same `REQ-` form and are stored with `source_kind = 'draft'`.
- SHAs are lowercase hex, 40 or 64 chars.
- PostgreSQL booleans are `true` and `false`. JSON booleans are `true` and `false`.
- Without `ORCHESTRATOR_CLOUD`, the API listens on `127.0.0.1:8787`. CORS allows the exact `--ui-origin`, default `http://127.0.0.1:4173`. The `Host` header must be `127.0.0.1:<port>`, `localhost:<port>`, or `[::1]:<port>`. Anything else is `403`.
- With `ORCHESTRATOR_CLOUD=1`, the API may bind the configured address. TLS is terminated in front of the process. The `Host` header must be on the configured allowlist. CORS allows the configured UI origin. A missing cloud flag still refuses a non-loopback `--addr` (RFC-045).
- Every route except `POST /api/session` requires `Authorization: Bearer <token>`. The bearer names a user.
- Board, intake, run, card, and project-settings routes are under `/api/projects/{projectId}/`. A caller who is not a member of that project's tribe gets `404`. An unknown project id gets the same `404`.
- JSON errors use one shape. Handlers do not invent a second shape.

```json
{
  "error": {
    "code": "validation",
    "message": "password must be at least 12 characters",
    "fields": {
      "password": "password must be at least 12 characters"
    }
  }
}
```

`fields` is omitted when the error is not tied to an input.

| HTTP | `error.code` | When |
| --- | --- | --- |
| 401 | `unauthenticated` | Missing, unknown, or expired bearer, or a wrong password |
| 403 | `forbidden` | `Host` or `Origin` not allowed, or the caller's role cannot perform the action |
| 404 | `not_found` | Unknown card or family, or a project the caller cannot see |
| 409 | `conflict` | A lock is held, or the card status changed after it was read (RFC-040) |
| 413 | `too_large` | Body over 1 MiB |
| 422 | `validation` | Body failed a field check |
| 422 | `transition_rejected` | The action is not listed for the card, or evidence is missing |
| 429 | `rate_limited` | That email is paused after 5 bad passwords |

A successful body never includes `error`.

## Statuses, groups, and actions

`group`, `actions`, and `unblockTo` are computed by the server on every read. The client displays them and does not recompute legality. The status table is the maximum set. The server then drops every name the caller's job role cannot use (RFC-046). A non-member does not receive the card.

| Status | `group` | `actions`, before the role filter |
| --- | --- | --- |
| `backlog`, `ready` | `waiting` | `[]` |
| `in_progress`, `self_check` | `running` | `[]` |
| `pr_open`, `agent_review`, `changes_requested` | `review` | `[]` |
| `human_review` | `review` | `approve`, `request_changes`, `reject` |
| `approved` | `shipping` | `request_changes`, `reject`, `deploy` |
| `deploying_dev`, `verifying` | `shipping` | `[]` |
| `done` | `finished` | `[]` |
| `blocked` | `stopped` | `unblock`, unless release is refused |

`deploy` remains only for a Lead Developer member. `approve` remains for a Lead Developer, QA Lead, or Product Owner member. `request_changes` remains for those three plus a Senior Developer member. `reject` remains for a Lead Developer, QA Lead, or Product Owner member. `unblock` remains for a Lead Developer or Product Owner member.

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

## PostgreSQL

PostgreSQL 16 or newer. The connection string is `databaseUrl` in `<data>/secrets.json` (RFC-044). The API does not return it. On the first start, when `secrets.json` is missing, startup copies `ORCHESTRATOR_DATABASE_URL` into `databaseUrl` if that variable is set. After that the file wins. An empty `databaseUrl` exits before any connection.

Open it with `database/sql` and `github.com/jackc/pgx/v5/stdlib`. Use a pool of at most 4 connections. One transaction wraps a compare-and-swap status update, its gate insert, and its transition event. Run goose with the postgres dialect.

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
  manual BOOLEAN NOT NULL DEFAULT false,
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
  review_independent BOOLEAN,
  review_verdict TEXT CHECK (review_verdict IN ('recommend_approve', 'request_changes')),
  reviewed_sha TEXT,
  ci_required BOOLEAN,
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
  passed BOOLEAN NOT NULL,
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
CREATE FUNCTION events_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'events are insert-only';
END;
$$;

CREATE TRIGGER events_no_update
BEFORE UPDATE ON events
FOR EACH ROW EXECUTE FUNCTION events_immutable();
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER events_no_delete
BEFORE DELETE ON events
FOR EACH ROW EXECUTE FUNCTION events_immutable();
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
DROP TRIGGER events_no_delete ON events;
DROP TRIGGER events_no_update ON events;
DROP FUNCTION events_immutable();
DROP TABLE events;
DROP TABLE gates;
DROP TABLE deploy_lock;
DROP TABLE locks;
DROP TABLE attempts;
DROP TABLE cards;
DROP TABLE decisions;
DROP TABLE requirements;
```

`00001_init.sql` and `00002_intake_state.sql` stay as shipped. Do not edit them. Identity, tribes, and projects are later migrations, starting at `00003`. Those migrations add `project_id` and replace the global unique indexes above with `(project_id, branch)` and `(project_id, pr_url)`. Requirement and decision primary keys become `(project_id, id)`. `runs.trigger` gains `deploy`.

`draftPending` and `planError` live on the single row `intake_state.id = 1`, created by `00002_intake_state.sql`. `draft_pending` is true after ingest finds a PRD file with no `REQ-` heading. `plan_error` stays null until the planner records a failure.

```sql
-- +goose Up
CREATE TABLE intake_state (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  draft_pending BOOLEAN NOT NULL DEFAULT false,
  plan_error TEXT
);
INSERT INTO intake_state (id, draft_pending) VALUES (1, false);

-- +goose Down
DROP TABLE intake_state;
```

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
| `transition` | `from`, `to`, `evidence`, optional `actorId` on rows written after RFC-046, plus `resetAttempts` and `maxAttempts` on release |
| `deploy` | `requestedSha`, `previousSha`, `devUrl`, `exitCode`, `reportedSha` |
| `health` | `passed`, `status`, `bytes`, `rollbackExit` |
| `verify` | `sha`, `mode` (`health_only` or `dev_check`), `exitCode`, `logRef` |
| `config_changed` | `section`, `fields` |
| `assertion_failed` | `name`, `detail` |

Every payload passes through `redact` before insert. A new transition may include `actorId`. Existing rows stay as stored. Nothing updates an event to backfill it.

## Identity, tribes, and projects

Migration `00003` and any later file in that series. Passwords use argon2id. A password shorter than 12 characters is `422`. User GitHub tokens and project bot tokens are AES-256-GCM ciphertext. The key is `tokenKey` in the service `secrets.json`, 32 bytes, and the API never returns it.

```sql
CREATE TABLE users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN (
    'lead_developer', 'senior_developer', 'developer',
    'qa_lead', 'qa', 'product_owner')),
  admin BOOLEAN NOT NULL DEFAULT false,
  disabled BOOLEAN NOT NULL DEFAULT false,
  created_at TEXT NOT NULL
);

CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  bearer_hash TEXT NOT NULL UNIQUE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE user_github (
  user_id TEXT PRIMARY KEY REFERENCES users(id),
  host TEXT NOT NULL,
  login TEXT NOT NULL DEFAULT '',
  token_ciphertext TEXT NOT NULL DEFAULT ''
);

CREATE TABLE tribes (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE tribe_members (
  tribe_id TEXT NOT NULL REFERENCES tribes(id),
  user_id TEXT NOT NULL REFERENCES users(id),
  PRIMARY KEY (tribe_id, user_id)
);

CREATE TABLE projects (
  id TEXT PRIMARY KEY,
  tribe_id TEXT NOT NULL REFERENCES tribes(id),
  name TEXT NOT NULL,
  repo_path TEXT NOT NULL,
  created_at TEXT NOT NULL
);
```

`project_id TEXT NOT NULL REFERENCES projects(id)` is added to `requirements`, `decisions`, `cards`, `attempts`, `locks`, `gates`, `events`, `approvals`, `failure_notes`, `runs`, `intake_state`, and `deploy_lock`. `events.actor_id` is nullable text. `intake_state` and `deploy_lock` use `project_id` as their primary key, one row each per project. Child rows that already reference `card_id` keep that id and also store `project_id` so a list does not join through every card.

`approvals.approver` stores the user's GitHub login. `requirements.accepted_by` stores the user id.

## Accounts and membership API

`POST /api/session`

```json
{ "email": "ada@example.com", "password": "a long passphrase" }
```

```json
{
  "bearer": "<random>",
  "expiresAt": "2026-10-10T17:00:00Z",
  "user": {
    "id": "user_0123456789ab",
    "email": "ada@example.com",
    "role": "lead_developer",
    "admin": false
  }
}
```

A wrong password is `401`. Five wrong passwords for that email pause further attempts for one minute with `429`. A disabled user is `401`. The bearer is 32 random bytes. The row stores its SHA-256. The process checks the hash and `expires_at` on every request. `DELETE /api/session` requires the bearer and deletes that row.

`GET /api/me` returns the `user` object plus `githubLogin` and `githubTokenSet`.

`PUT /api/me/password`

```json
{ "currentPassword": "a long passphrase", "newPassword": "another long passphrase" }
```

A wrong current password is `401`. A short new password is `422` on `newPassword`.

`GET /api/me/github` and `PUT /api/me/github` are the caller's GitHub settings.

```json
{ "host": "github.com", "login": "ada", "tokenSet": false }
```

PUT accepts `host`, `login`, optional `token`, and `clear`. The same host and token rules as a secret field apply. One user cannot call this route for someone else.

`GET /api/users` and `POST /api/users` require the admin flag. A non-admin is `403`.

```json
{
  "email": "ada@example.com",
  "password": "a long passphrase",
  "role": "product_owner",
  "admin": false
}
```

`PUT /api/users/{id}` accepts `role`, `admin`, `disabled`, and optional `password`. The response user object has no password and no token. Removing or disabling the last admin is `422`.

`GET /api/tribes` returns tribes the caller belongs to. An admin receives every tribe. `POST /api/tribes` with `{ "name": "Platform" }` requires admin.

`POST /api/tribes/{id}/members` with `{ "userId": "user_0123456789ab" }` and `DELETE /api/tribes/{id}/members/{userId}` require admin. A non-admin is `403`. A missing tribe is `404`.

`GET /api/projects` returns projects whose tribe includes the caller. An admin receives every project, which does not grant card access. `POST /api/projects` requires admin.

```json
{ "tribeId": "tribe_0123456789ab", "name": "Widget", "repoPath": "/srv/widget" }
```

`repoPath` must be an absolute directory outside the service data root and outside every other project's repo. The server creates `<data>/projects/<project-id>/` at mode `0700`.

## Card JSON

```json
{
  "id": "card_0123456789ab",
  "title": "Parse PRD headings",
  "status": "human_review",
  "group": "review",
  "actions": ["approve", "request_changes", "reject"],
  "projectId": "proj_0123456789ab",
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

## Board and intake

Routes in this section include `/api/projects/{projectId}/`. A non-member is `404` before the handler reads the card.

`GET /api/projects/{projectId}/cards`

```json
{
  "canRun": true,
  "running": false,
  "cards": []
}
```

An empty store returns `canRun: false`, `running: false`, and `cards: []`. It does not return an error.

`canRun` is true when that project's worker could start something now: a planning job is needed, a `backlog` card can be promoted, a `ready` or `changes_requested` card is under the cap, a `pr_open` card awaits review, or a `verifying` card awaits its check. An `approved` card does not set `canRun`. Deploy waits for a Lead Developer. `running` is true while an unprocessed `runs` row exists for that project.

`GET /api/projects/{projectId}/cards/{id}` returns one card object. Unknown id, or a card in another project, is `404`.

`GET /api/projects/{projectId}/cards/{id}/trace`

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

Oldest first. Payload is the redacted JSON already stored. New transition payloads may include `actorId`.

`GET /api/projects/{projectId}/intake`

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

`POST /api/projects/{projectId}/ingest` has an empty body. It reparses that project's PRD and RFC paths and returns the intake shape. Unchanged hashes do not duplicate rows. A changed hash clears `accepted_at` for that id in that project and blocks linked cards past `backlog` with `source_changed` (REQ-004). When a PRD has no `REQ-` headings, the response has `draftPending: true` and the project's worker runs the planner to draft ids (REQ-002). `planError` holds the last planner failure message, redacted, until the next successful plan. A caller who is not a member is `404`.

`POST /api/projects/{projectId}/accept`

```json
{ "requirementIds": ["REQ-001", "REQ-002"] }
```

Allowed for a Product Owner member. Anyone else is `403`, or `404` when they are not a member. Stores that user's id and the time on those ids, then inserts a `runs` row with trigger `accept` so the planner can create cards. Ids absent from the body are left as they are. Unknown ids are `422`.

## Runs and human gates

`POST /api/projects/{projectId}/runs` has an empty body. A Lead Developer, Senior Developer, Developer, or QA member may call it. Anyone else is `403`. It inserts a `runs` row for that project and returns immediately. That project's worker processes it until idle (0003).

```json
{ "runId": "run_0123456789ab", "canRun": true }
```

When nothing can move, the response is still `200`:

```json
{ "runId": "run_0123456789ab", "canRun": false, "reason": "no card is ready" }
```

`POST /api/projects/{projectId}/cards/{id}/approve`

```json
{ "sha": "9c1e5b7d0a4f2e6c8b3d1a7f5e9c2b4d6a8f0e1c" }
```

The approver is the session user. The role must be Lead Developer, QA Lead, or Product Owner, and the user must be a member. The server refreshes the head from GitHub with the project bot token, rejects a missing GitHub login, rejects the project bot login, and checks that `sha` equals the head. Approving the same card and SHA twice returns the card and does not insert a second approval. A SHA mismatch is `422` with `fields.sha`. A missing GitHub login is `422` with `fields.login`. Approval does not start deploy.

`POST /api/projects/{projectId}/cards/{id}/request-changes` and `POST /api/projects/{projectId}/cards/{id}/reject` have empty bodies. Both work from `human_review` and `approved` when the role allows them. Request-changes clears approval and writes a failure note with source `approver`. Reject sets `blockedReason` to `rejected`.

`POST /api/projects/{projectId}/cards/{id}/deploy` has an empty body. It is allowed when `deploy` is in `actions`. The card moves from `approved` toward `deploying_dev` through `machine.Transition`, and a `runs` row with trigger `deploy` wakes that project's worker.

`POST /api/projects/{projectId}/cards/{id}/unblock`

```json
{ "resetAttempts": false }
```

The body is optional. The card goes to `unblockTo`. With `resetAttempts: false`, `attemptsUsed` stays, and `maxAttempts` rises to `attemptsUsed + 1` when the count is already at the cap. With `resetAttempts: true`, `attemptsUsed` becomes `0` and `maxAttempts` returns to the router budget.

Approve, request-changes, deploy, and unblock insert a `runs` row with the matching trigger so the board continues without a second click. Each route returns the card object. An action not listed for the card is `422` `transition_rejected`. A status change between read and write is `409` `conflict`. A role that cannot perform the action is `403`.

## Settings

Secret fields are write-only.

| Rule | Behavior |
| --- | --- |
| GET | `tokenSet` or `keySet` is boolean. The secret value is absent |
| PUT field omitted or empty | Stored secret stays |
| PUT field with a new string | Stored secret is replaced. Minimum 8 characters |
| PUT `clear: true` | Stored secret is deleted and the flag becomes false |
| Event | `config_changed` records the section and the field names, not the values |
| In-flight job | Keeps that project's settings snapshot it started with |
| Failure | `422` with `fields`. Files on disk are unchanged |
| Caller | Lead Developer member for project sections. The account GitHub route is the signed-in user. Anyone else is `403`, or `404` when they are not a member |

Account GitHub is `GET` and `PUT /api/me/github`, described above. `POST /api/me/github/test` uses the caller's stored token and returns `{ "ok": true, "login": "octocat" }` or `{ "ok": false, "message": "authentication failed" }`.

`GET /api/projects/{projectId}/settings/github`

```json
{
  "host": "github.com",
  "owner": "",
  "repo": "",
  "baseBranch": "main",
  "botLogin": "",
  "tokenSet": false
}
```

`PUT /api/projects/{projectId}/settings/github` accepts the same fields plus optional `token` and `clear`. The token is the project bot token.

| Field | Rule |
| --- | --- |
| `host` | Hostname only, no scheme or path |
| `owner`, `repo` | `^[A-Za-z0-9._-]+$` |
| `baseBranch` | Valid by `git check-ref-format --branch`, and not `orch/*` |
| `botLogin` | Required before a real repo job runs. Used for the commit identity. Must not equal a login that is asked to approve |

`POST /api/projects/{projectId}/settings/github/test` uses the stored bot token and returns the same shape as the account test. The token is in neither body. A completed check is `200`, including `ok: false`. No stored token is `422`.

`GET /api/projects/{projectId}/settings/llm`

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

`PUT /api/projects/{projectId}/settings/llm`

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

`POST /api/projects/{projectId}/settings/llm/{family}/test` returns `{ "ok": true }` or `{ "ok": false, "message": "..." }` with `200`. Unknown family is `404`. A family that needs a key and has none is `422`.

`GET /api/projects/{projectId}/settings/router`

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

`PUT /api/projects/{projectId}/settings/router` sends `routes` and `budgets`. A non-empty model or fallback must exist on a saved family. The review sentinel `different-family-from-author` is allowed only on `review.model`. `fallback` may be empty, which means no fallback. An empty `model` may be saved. A job that needs that class then blocks with `router_config`. Budgets are integers greater than zero, with `maxAttempts` at most 10.

`GET /api/projects/{projectId}/settings/deploy`

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
  "rfcPath": "docs/rfc"
}
```

`PUT /api/projects/{projectId}/settings/deploy` accepts those fields plus optional `deployToken` and `clear`.

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
| `prdPath`, `rfcPath` | Relative, no `..`, inside the project's repo |

Empty deploy fields may be saved. A deploy then blocks with `deploy_config` and runs no command. A non-empty `devCheck` with an empty `devUrl` is also `deploy_config` at deploy time. Changing `localCheck` or `devCheck` applies the rule in 0006 to existing cards.

## Files on disk

The service data directory defaults to `~/.orchestrator/service` (RFC-047). Each project directory must sit outside that project's repo (RFC-036, RFC-048).

```text
<data>/                                      mode 0700
<data>/secrets.json                          mode 0600
<data>/projects/<project-id>/                mode 0700
<data>/projects/<project-id>/config.yaml     mode 0600
<data>/projects/<project-id>/secrets.json    mode 0600
<data>/projects/<project-id>/worker.lock
<data>/projects/<project-id>/empty-hooks/
<data>/projects/<project-id>/empty-gh/
<data>/projects/<project-id>/askpass/
<data>/projects/<project-id>/worktrees/<card-id>/
<data>/projects/<project-id>/exports/<card-id>-<sha>/   deleted after each check
<data>/projects/<project-id>/logs/<attempt-id>.log
<data>/projects/<project-id>/logs/<attempt-id>-review.json
```

The board is not a file in this directory. It is the PostgreSQL database named by `databaseUrl` in the service secrets file (RFC-044, RFC-047).

`config.yaml`:

```yaml
github:
  host: github.com
  owner: ""
  repo: ""
  baseBranch: main
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
```

The deploy settings route flattens `deploy`, `checks`, `sandbox`, `scheduler`, and `paths` into one object, as shown above. Service `secrets.json`:

```json
{
  "databaseUrl": "",
  "tokenKey": ""
}
```

Project `secrets.json`:

```json
{
  "deployToken": "",
  "llmKeys": {
    "anthropic": "",
    "openai": ""
  }
}
```

`databaseUrl` is the PostgreSQL URL for every project's board (RFC-047). `tokenKey` encrypts user GitHub tokens and project bot tokens. Empty string means unset. The API never returns either value. The project bot token is not in this file. Both secret files are written to a temp file in the same directory, synced, and renamed. The API never decodes a secrets file into a struct that is encoded to the client. Edit a project's `config.yaml` by hand only while that project's worker is stopped. Startup validates it with the same rules as the PUT routes.

## CLI twins

The CLI calls the same `internal/app` functions, not the HTTP API. It prints the error `message` on stderr and exits `1` on any error. It exits `0` on success, including a run where `canRun` is false.

| CLI | API |
| --- | --- |
| `orchestrator users bootstrap` | Creates the first admin when `users` is empty. No HTTP twin |
| `orchestrator ingest --project ID` | `POST /api/projects/{id}/ingest` |
| `orchestrator accept --project ID` | `POST /api/projects/{id}/accept` with every current unaccepted id |
| `orchestrator run-next --project ID` | `POST /api/projects/{id}/runs`, and runs that project's worker when its lock is free (RFC-048) |
| `orchestrator status --project ID` | `GET /api/projects/{id}/cards` |
| `orchestrator status --project ID CARD` | `GET /api/projects/{id}/cards/{card}` |
| `orchestrator trace --project ID CARD` | `GET /api/projects/{id}/cards/{card}/trace` |
| `orchestrator approve --project ID CARD --sha SHA` | `POST /api/projects/{id}/cards/{card}/approve` |
| `orchestrator request-changes --project ID CARD` | `POST /api/projects/{id}/cards/{card}/request-changes` |
| `orchestrator reject --project ID CARD` | `POST /api/projects/{id}/cards/{card}/reject` |
| `orchestrator unblock --project ID CARD [--reset-attempts]` | `POST /api/projects/{id}/cards/{card}/unblock` |
| `orchestrator serve` | Starts the API and one worker per project |

Break-glass CLI commands that can read the service data directory attribute the actor from `--as EMAIL` when the command is a human gate. HTTP gates use the session user. The fixture may omit `--as` and use the fixture user.

`approve` without `--sha` exits `1` before any transition. `status` with no cards prints `board is empty` and the next command `orchestrator ingest`. Project settings have no CLI twin. Use the UI, or edit that project's `config.yaml` while its worker is stopped.
