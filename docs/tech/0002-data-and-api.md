# Data and API

Status: Accepted for implementation. Date: 2026-10-10. Revised after review.

This is the contract both processes implement. Stack choices are in [0001-implementation.md](0001-implementation.md). The backend build steps are in [0003-backend.md](0003-backend.md). The client is in [0004-frontend.md](0004-frontend.md). Security rules that constrain these fields are in [0006-security-and-harness.md](0006-security-and-harness.md).

If a field here and a PRD acceptance check disagree, the PRD wins. See [../README.md](../README.md).

## Conventions

- Times are UTC RFC3339 strings.
- Ids are text. Cards use `card_` plus 12 hex chars. Attempts use `att_`, events `evt_`, runs `run_`, users `user_`, sessions `ses_`, tribes `tribe_`, and projects `proj_`, each plus 12 hex chars.
- Requirement ids and decision ids are copied from markdown headings (`REQ-001`, `RFC-001`). The harness does not renumber them. Drafted ids from the planner use the same `REQ-` form and are stored with `source_kind = 'draft'`. Ids added by hand use the same form and are stored with `source_kind = 'manual'` (RFC-053). A blank id on add gets the next free number in that project.
- SHAs are lowercase hex, 40 or 64 chars.
- PostgreSQL booleans are `true` and `false`. JSON booleans are `true` and `false`.
- Without `ORCHESTRATOR_CLOUD`, the API listens on `127.0.0.1:8787`. CORS allows the exact `--ui-origin`, default `http://127.0.0.1:4173`. The `Host` header must be `127.0.0.1:<port>`, `localhost:<port>`, or `[::1]:<port>`. Anything else is `403`.
- With `ORCHESTRATOR_CLOUD=1`, the API may bind the configured address. TLS is terminated in front of the process. The `Host` header must be on the configured allowlist. CORS allows the configured UI origin. A missing cloud flag still refuses a non-loopback `--addr` (RFC-045).
- Every route except `POST /api/session` requires `Authorization: Bearer <token>`. The bearer names a user.
- Board, intake, run, card, and project-settings routes are under `/api/projects/{projectId}/`. A caller who is not a member of that project's tribe gets `404`. An unknown project id gets the same `404`. The one exception: a company admin may open the `settings/*` routes and `sync` of any project (RFC-059). Cards, intake, and runs still need membership.
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
| `backlog` | `waiting` | `[]` |
| `ready` | `waiting` | `run` when every `dependsOn` card is `done` and the card has no open run request (RFC-061) |
| `in_progress` | `running` | `run` when the card has no open attempt, its dependencies are `done`, and it has no open run request (RFC-063) |
| `self_check` | `running` | `[]` |
| `changes_requested` | `review` | `run` under the same rule as `in_progress` (RFC-063) |
| `pr_open`, `agent_review` | `review` | `[]` |
| `human_review` | `review` | `approve`, `request_changes`, `reject` |
| `approved` | `shipping` | `request_changes`, `reject`, `deploy` |
| `deploying_dev`, `verifying` | `shipping` | `[]` |
| `done` | `finished` | `[]` |
| `blocked` | `stopped` | `unblock`, unless release is refused |

`deploy` remains only for a Lead Developer member. `approve` remains for a Lead Developer, QA Lead, or Product Owner member who is not the card owner (RFC-054). `request_changes` remains for those three plus a Senior Developer member. `reject` remains for a Lead Developer, QA Lead, or Product Owner member. `unblock` remains for a Lead Developer or Product Owner member. `run` remains for a Lead Developer, Senior Developer, Developer, or QA member.

After the table, every card that is not `done` also lists `instructions` for those same four roles (RFC-073). It names the instructions route, not a status change.

## Blocked reasons

`blockedReason` is one of these. `unblockTo` follows RFC-041.

| Reason | Set when | `unblockTo` |
| --- | --- | --- |
| `turn_cap`, `token_cap`, `wall_cap` | A budget stopped the attempt | `ready` |
| `attempt_cap` | `attemptsUsed` reached `maxAttempts` | `ready` |
| `provider_error` | Primary and fallback both failed in the implement job | `ready` |
| `review_parse` | Two reviewer replies had no parseable verdict. Since RFC-069 the review job records a `failed` verdict instead; older cards may still carry it | `ready` |
| `router_config` | The route for the implement task class has no model. The review job records a `failed` verdict instead (RFC-069) | `ready` |
| `github_config` | The company organization, bot login, or token is missing, or the project names no repo | `ready` |
| `owner_github` | The card has no owner, or the owner is disabled, has no verified GitHub login, or has no token (RFC-054) | `ready` |
| `sandbox_config` | Sandbox image missing or not pinned, or a check command fails parsing | `ready` |
| `check_error` | A check could not start | `ready` |
| `publish_failed` | The push or the pull request call failed after the gating check passed (RFC-063) | `ready` |
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
  review_verdict TEXT CHECK (review_verdict IN ('recommend_approve', 'request_changes', 'failed')),
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
-- 00016: insert order, so events in the same second keep their order
ALTER TABLE events ADD COLUMN seq BIGSERIAL;
CREATE INDEX events_card_seq ON events (card_id, ts, seq);

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
-- 00015: ALTER TABLE approvals ADD COLUMN independent BOOLEAN NOT NULL DEFAULT true; false for an owner or bot-login approval under Solo approval (RFC-067)

CREATE TABLE failure_notes (
  id TEXT PRIMARY KEY,
  card_id TEXT NOT NULL REFERENCES cards(id),
  attempt_id TEXT NOT NULL,
  source TEXT NOT NULL CHECK (source IN (
    'local_check', 'review', 'ci', 'approver', 'dev_check', 'no_commit', 'instructions')),
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

`00014_card_runs.sql` adds `runs.card_id TEXT REFERENCES cards(id) ON DELETE CASCADE` and the partial unique index `runs_open_card ON runs(card_id) WHERE card_id IS NOT NULL AND processed_at IS NULL`. A row with `card_id` asks for that one card. A card has at most one open row (RFC-061).

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

Event `kind` is one of: `route`, `model_call`, `tool_call`, `gate`, `transition`, `transition_rejected`, `deploy`, `health`, `verify`, `budget_stop`, `config_changed`, `assertion_failed`, `reviewers_requested`, `stacks_detected`, `job_error`, `instructions_saved`.

| Kind | Payload fields |
| --- | --- |
| `route` | `taskClass`, `modelId`, `fallback`, `familyFlag`, `reason` |
| `model_call` | `role`, `modelId`, `inputTokens`, `outputTokens`, `stopReason`, and `text`, the model's reply text, when it wrote any (RFC-074). The prompt is not stored; `attempts.prompt_hash` and `review_prompt_hash` keep its hash |
| `tool_call` | `name`, `ok`, `rejected`, `summary`, and `args` for the tools below (RFC-074) |
| `transition` | `from`, `to`, `evidence`, optional `actorId` on rows written after RFC-046, plus `resetAttempts` and `maxAttempts` on release |
| `deploy` | `requestedSha`, `previousSha`, `devUrl`, `exitCode`, `reportedSha` |
| `health` | `passed`, `status`, `bytes`, `rollbackExit` |
| `verify` | `sha`, `mode` (`health_only` or `dev_check`), `exitCode`, `logRef` |
| `config_changed` | `section`, `fields` |
| `assertion_failed` | `name`, `detail` |
| `reviewers_requested` | `reviewers` (GitHub logins), `ok`, and `message` when `ok` is false (RFC-054) |
| `stacks_detected` | `stacks` (stack ids found by the last Sync) (RFC-055) |
| `job_error` | `message`, the redacted error that stopped an implement job (RFC-063). NUL bytes are dropped and invalid UTF-8 is replaced. A message over 8 KiB keeps its first and last 4 KiB around `... (N bytes cut) ...` |
| `instructions_saved` | `length`, the character count of the saved instructions, `0` when they were cleared. The text stays in the note. The row's `actor_id` is the person who saved them (RFC-073) |

A `tool_call` named `conventions_context` adds `items`, a list of `{ "kind": "rule" or "skill", "id", "version", "hash" }` for every rule and skill in the attempt's pack (RFC-055). A `tool_call` named `linked_context` adds `links`.

`tool_call.args` holds what a person needs to follow the step (RFC-074):

| Tool | `args` |
| --- | --- |
| `list_files`, `read_file` | `path` |
| `read_linked_file` | `project`, `path` |
| `search_files` | `text` |
| `edit_file` | `path`, `bytes`, `lines`. Never the file text |
| `run_local_check` | `command`, the card's `local_check` |
| `commit` | `message`, as the model wrote it |
| `comment_pr`, `verdict` | `body` |

`model_call.text` is at most 4 KiB and each string in `args` at most 1 KiB (`trace.TextCap`, `trace.ArgCap`). `trace.Clip` drops NUL bytes, replaces invalid UTF-8, redacts with the job's secrets, and then keeps the head and tail around `... (N bytes cut) ...`, in that order, so a secret across the cut is gone before the cut. `job_error.message` uses the same function at 8 KiB. No pack builder reads `events`.

Every payload passes through `redact` before insert. A new transition may include `actorId`. Existing rows stay as stored. Nothing updates an event to backfill it.

## Identity, tribes, and projects

Migration `00003` and any later file in that series. Passwords use argon2id. A password shorter than 12 characters is `422`. User GitHub tokens and the company GitHub token are AES-256-GCM ciphertext. The key is `tokenKey` in the service `secrets.json`, 32 bytes, and the API never returns it.

```sql
CREATE TABLE users (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL DEFAULT '',
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
  admin BOOLEAN NOT NULL DEFAULT false,
  PRIMARY KEY (tribe_id, user_id)
);

CREATE TABLE projects (
  id TEXT PRIMARY KEY,
  tribe_id TEXT NOT NULL REFERENCES tribes(id),
  name TEXT NOT NULL,
  repo_path TEXT NOT NULL,
  created_at TEXT NOT NULL,
  github_repo TEXT NOT NULL DEFAULT '',
  github_base TEXT NOT NULL DEFAULT 'main'
);
CREATE UNIQUE INDEX projects_repo ON projects(github_repo) WHERE github_repo <> '';

CREATE TABLE workspace_github (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  host TEXT NOT NULL DEFAULT 'github.com',
  org TEXT NOT NULL DEFAULT '',
  bot_login TEXT NOT NULL DEFAULT '',
  token_ciphertext TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);

CREATE TABLE project_links (
  project_id TEXT NOT NULL REFERENCES projects(id),
  linked_project_id TEXT NOT NULL REFERENCES projects(id),
  shared_paths_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, linked_project_id),
  CHECK (project_id <> linked_project_id)
);
```

`00005` added host, owner, bot login, and bot token columns to `projects`. `00006` drops them and creates `workspace_github` (RFC-049). `repo_path` is set by the server to `<data>/projects/<id>/repo`, the clone (RFC-050). `00007` creates `project_links`. Both projects in a link share a tribe. The server checks that, because a foreign key cannot compare two rows.

`project_id TEXT NOT NULL REFERENCES projects(id)` is added to `requirements`, `decisions`, `cards`, `attempts`, `locks`, `gates`, `events`, `approvals`, `failure_notes`, `runs`, `intake_state`, and `deploy_lock`. `events.actor_id` is nullable text. `intake_state` and `deploy_lock` use `project_id` as their primary key, one row each per project. Child rows that already reference `card_id` keep that id and also store `project_id` so a list does not join through every card.

`approvals.approver` stores the user's GitHub login. `requirements.accepted_by` stores the user id.

`00018` adds `requirements.self_review BOOLEAN NOT NULL DEFAULT false` (RFC-070). Only the self-review route and a manual add write it. Ingest leaves it alone.

`00019` adds `requirements.ai_review_mode TEXT NOT NULL DEFAULT 'continue'`, checked to `('continue', 'manual')` (RFC-072). Only the ai-review-mode route and a manual add write it. Ingest leaves it alone. It also widens the `runs.trigger` check with `retry`, the row the implement and review jobs write to queue the next attempt (RFC-071). Its Down section deletes `retry` rows before narrowing the check.

`00020` widens the `failure_notes.source` check with `instructions` (RFC-073). An instructions row has `attempt_id = 'instructions'`. The newest one is the card's current instructions, and a blank one clears them. The implement job's failure note is the newest row of any other source. Its Down section deletes `instructions` rows before narrowing the check.

`00008` widens the `requirements.source_kind` check to `('prd', 'rfc', 'draft', 'manual')` and adds `requirements.created_by TEXT`, the user id that added a manual row (RFC-053). It is null for parsed and drafted rows. A manual row has `source_path = ''` and `heading` equal to its id.

`00009` is the card owner (RFC-054):

```sql
ALTER TABLE cards ADD COLUMN owner_id TEXT REFERENCES users(id);

CREATE TABLE requirement_reviewers (
  project_id TEXT NOT NULL,
  requirement_id TEXT NOT NULL,
  user_id TEXT NOT NULL REFERENCES users(id),
  added_by TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, requirement_id, user_id),
  FOREIGN KEY (project_id, requirement_id) REFERENCES requirements(project_id, id) ON DELETE CASCADE
);

ALTER TABLE user_github ADD COLUMN github_id BIGINT;
ALTER TABLE user_github ADD COLUMN verified_at TEXT;
CREATE UNIQUE INDEX user_github_verified_login ON user_github(host, lower(login)) WHERE verified_at IS NOT NULL;
```

From `00009`, `requirements.created_by` is also set on a parsed row: the user who ran the ingest that first inserted it. A re-ingest keeps it. A CLI ingest leaves it null. `cards.owner_id` is the `created_by` shared by every requirement the card lists, set by `create_card`. It is null when that value is null. `user_github.verified_at` and `github_id` are set by the token test and cleared when the token or host changes.

`00010` is the company coding rules and integration skills (RFC-055, RFC-056):

```sql
CREATE TABLE stacks (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  detect_json TEXT NOT NULL DEFAULT '{}',
  version INTEGER NOT NULL DEFAULT 1,
  content_hash TEXT NOT NULL,
  disabled BOOLEAN NOT NULL DEFAULT false,
  updated_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX stacks_name ON stacks(lower(name));

CREATE TABLE convention_rules (
  id TEXT PRIMARY KEY,
  scope TEXT NOT NULL CHECK (scope IN ('global', 'stack')),
  stack_id TEXT REFERENCES stacks(id),
  category TEXT NOT NULL CHECK (category IN ('naming', 'database', 'style', 'framework', 'security', 'testing')),
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1,
  content_hash TEXT NOT NULL,
  disabled BOOLEAN NOT NULL DEFAULT false,
  updated_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK ((scope = 'global') = (stack_id IS NULL))
);

CREATE TABLE skills (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  use_when TEXT NOT NULL,
  body TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1,
  content_hash TEXT NOT NULL,
  disabled BOOLEAN NOT NULL DEFAULT false,
  updated_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE convention_revisions (
  kind TEXT NOT NULL CHECK (kind IN ('rule', 'stack', 'skill')),
  item_id TEXT NOT NULL,
  version INTEGER NOT NULL,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  created_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (kind, item_id, version)
);

CREATE TABLE project_stacks (
  project_id TEXT NOT NULL REFERENCES projects(id),
  stack_id TEXT NOT NULL REFERENCES stacks(id),
  source TEXT NOT NULL CHECK (source IN ('detected', 'manual')),
  added_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, stack_id)
);

CREATE TABLE project_skills (
  project_id TEXT NOT NULL REFERENCES projects(id),
  skill_id TEXT NOT NULL REFERENCES skills(id),
  added_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, skill_id)
);

CREATE TABLE card_skills (
  project_id TEXT NOT NULL REFERENCES projects(id),
  card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
  skill_id TEXT NOT NULL REFERENCES skills(id),
  PRIMARY KEY (project_id, card_id, skill_id)
);
```

Ids use the prefixes `stk_`, `rule_`, and `skl_`. `detect_json` is `{ "files": [], "npmDeps": [], "pyDeps": [] }`. `content_hash` is the SHA-256 hex of the item's text fields joined by the byte `0x1e`: name and `detect_json` for a stack; scope, stack id, category, title, and body for a rule; slug, title, use when, and body for a skill. A revision stores the text the hash covers: `title` is the name, title, or slug, and `body` is `detect_json`, the rule body, or `use when:` plus the skill body. The migration seeds the stacks Go, TypeScript, SvelteKit, React, Python, and Django with their markers, the global rules "Naming" and "Database tables and columns", and the skill `oauth2-integration`, each at version 1 with a revision row.

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
    "name": "Ada Lovelace",
    "email": "ada@example.com",
    "role": "lead_developer",
    "admin": false
  }
}
```

`name` is the display name (RFC-058). It is `""` for a user created before names existed. Pages show `name`, then the GitHub login, then the email.

A wrong password is `401`. Five wrong passwords for that email pause further attempts for one minute with `429`. A disabled user is `401`. The bearer is 32 random bytes. The row stores its SHA-256. The process checks the hash and `expires_at` on every request. `DELETE /api/session` requires the bearer and deletes that row.

`GET /api/me` returns the `user` object plus `githubLogin`, `githubTokenSet`, `githubVerified`, and `tribeAdminOf`, the ids of the tribes where the caller is a tribe admin (RFC-057).

`PUT /api/me` with `{ "name": "Ada Lovelace" }` sets the caller's own name and returns the same body as `GET /api/me`. The name is trimmed, 1 to 80 characters, and has no control characters. It is not unique. Anything else is `422` on `name`.

`PUT /api/me/password`

```json
{ "currentPassword": "a long passphrase", "newPassword": "another long passphrase" }
```

A wrong current password is `401`. A short new password is `422` on `newPassword`.

`GET /api/me/github` and `PUT /api/me/github` are the caller's GitHub settings.

```json
{ "host": "github.com", "login": "ada", "tokenSet": false, "verified": false }
```

PUT accepts `host`, `login`, optional `token`, and `clear`. The same host and token rules as a secret field apply. One user cannot call this route for someone else. A new token, a cleared token, a changed host, or a changed login sets `verified` to false until the next test.

`GET /api/users` and `POST /api/users` require the admin flag. A non-admin is `403`.

```json
{
  "name": "Ada Lovelace",
  "email": "ada@example.com",
  "password": "a long passphrase",
  "role": "product_owner",
  "admin": false
}
```

`name` is required on create, with the same rules as `PUT /api/me`. `PUT /api/users/{id}` accepts optional `name`, `role`, `admin`, `disabled`, and optional `password`. A `name` that is sent and empty is `422` on `name`. The response user object has no password and no token. Removing or disabling the last admin is `422`.

`GET /api/tribes` returns tribes the caller belongs to. An admin receives every tribe. `POST /api/tribes` with `{ "name": "Platform" }` requires admin.

Every route under `/api/tribes/{id}/` below allows an admin or a tribe admin of that tribe (RFC-057). Anyone else is `403`. A missing tribe is `404`.

`GET /api/tribes/{id}/members` lists members. `tribeAdmin` is the per-tribe flag. `admin` is the company flag.

```json
{ "members": [{ "id": "user_0123456789ab", "name": "Ada Lovelace", "email": "ada@example.com", "role": "lead_developer", "admin": false, "disabled": false, "tribeAdmin": true }] }
```

`GET /api/tribes/{id}/candidates` lists enabled users who are not members, for the add picker: `{ "users": [{ "id": "user_0123456789ab", "name": "Grace Hopper", "email": "grace@example.com", "role": "developer" }] }`.

`POST /api/tribes/{id}/members` with `{ "userId": "user_0123456789ab" }` adds a member. `DELETE /api/tribes/{id}/members/{userId}` removes one. A tribe admin who is not an admin cannot remove themselves (`422` on `userId`).

`PUT /api/tribes/{id}/members/{userId}` with `{ "tribeAdmin": true }` sets or clears the flag and returns `204`. A user who is not a member is `404`. A tribe admin who is not an admin cannot clear their own flag (`422` on `tribeAdmin`).

`GET /api/projects` returns projects whose tribe includes the caller. An admin receives every project, which does not grant card access. `POST /api/projects` requires an admin or a tribe admin of `tribeId`. Anyone else is `403`.

```json
{
  "tribeId": "tribe_0123456789ab",
  "name": "Widget",
  "repo": "widget",
  "baseBranch": "main",
  "links": [{ "projectId": "proj_0123456789ab", "sharedPaths": [] }]
}
```

`repo` is a repository name in the company organization, `^[A-Za-z0-9._-]+$`. A repo another project already names is `422` on `repo`. The server then asks GitHub with the company token (RFC-060): `gh api repos/{org}/{repo}` and `gh api repos/{org}/{repo}/branches/{baseBranch}`. No company organization or token, a repository GitHub does not find, an archived repository, or one owned by another organization is `422` on `repo`. A branch GitHub does not find is `422` on `baseBranch`. Any other failed call is `502`. The name GitHub reports is stored. `baseBranch` follows the branch rule below. `links` is optional and follows the PUT links rules below, checked against `tribeId`. The project and its links are written in one transaction, so a bad link is `422` and stores no project. When links were stored, the server appends `config_changed` with `links`. The `201` response is the project object plus `links` in the GET links shape. The server creates `<data>/projects/<project-id>/` at mode `0700`. The clone happens on the first Sync or ingest.

A project object is:

```json
{
  "id": "proj_0123456789ab",
  "tribeId": "tribe_0123456789ab",
  "name": "Widget",
  "repo": "widget",
  "baseBranch": "main"
}
```

## Card JSON

```json
{
  "id": "card_0123456789ab",
  "title": "Parse PRD headings",
  "status": "human_review",
  "group": "review",
  "actions": ["approve", "request_changes", "reject"],
  "projectId": "proj_0123456789ab",
  "owner": { "id": "user_0123456789ab", "name": "Ada Lovelace", "email": "ada@example.com", "githubLogin": "ada" },
  "reviewers": [{ "id": "user_0123456789ac", "name": "Grace Hopper", "email": "grace@example.com", "githubLogin": "grace" }],
  "skills": [{ "id": "skl_0123456789ab", "slug": "oauth2-integration", "title": "OAuth 2.0 integration", "useWhen": "A requirement adds sign-in with an outside identity provider." }],
  "unblockTo": null,
  "requirementIds": ["REQ-001"],
  "decisionIds": ["RFC-018"],
  "dependsOn": [],
  "queued": false,
  "waitingOn": [],
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
  "reviewVerdict": "recommend_approve",
  "selfReview": false,
  "aiReviewMode": "continue",
  "approvedSha": null,
  "deployedSha": null,
  "environment": "dev",
  "failureReason": null,
  "blockedReason": null,
  "updatedAt": "2026-10-10T05:00:00Z"
}
```

`reviewedSha`, `reviewIndependent`, and `reviewVerdict` come from the latest attempt with a review verdict. `reviewVerdict` is `recommend_approve`, `request_changes`, `failed` when the AI review gave no verdict and the checks decided (RFC-069), or `null` before a review. `reviewIndependent` is `null` for a failed review. `selfReview` is true when the card lists at least one requirement and every one of them has Self review on in the card's project (RFC-070). `aiReviewMode` is `manual` when any requirement the card lists has that mode in the card's project, and `continue` otherwise (RFC-072). `headSha` is the value from the last refresh, not a live call. `owner` is `null` when the card has no owner. `githubLogin` is the owner's verified login, or `null`. `reviewers` lists the enabled tribe members named as reviewers on the card's requirements in the same project, without the owner, sorted by email, `[]` when there are none. A reviewer's `githubLogin` is `""` when it is not verified. `name` on the owner and each reviewer is `""` when the user has no name (RFC-058). The board reads every card's reviewers in one query. `skills` lists the card's skills with their `useWhen` text, `[]` when it has none (RFC-056). `queued` is true while the card has an open card-scoped `runs` row. `waitingOn` lists the `dependsOn` ids whose card is not `done`, `[]` when there are none (RFC-061).

`GET /api/projects/{projectId}/cards/{id}` and every card action that returns one card add two fields that the board list leaves out:

```json
{
  "requirements": [
    {
      "id": "REQ-001",
      "heading": "CSV export",
      "statement": "Export orders as CSV.",
      "sourceKind": "prd",
      "sourcePath": "docs/prd/orders.md",
      "accepted": true
    }
  ],
  "rules": [{ "id": "rule_naming", "title": "Naming", "label": "global", "category": "naming" }],
  "notes": [
    {
      "source": "review",
      "note": "The frontend job does not run the lint scripts.",
      "createdAt": "2026-10-12T05:00:00Z",
      "attemptN": 2
    }
  ],
  "instructions": "Run the lint scripts in every package."
}
```

`requirements` lists the rows of `requirementIds` in the card's project, in the card's order. An id with no row in that project is left out. `rules` lists the rules the card's packs carry, as `conventions.ForCard` reads them (RFC-055): global rules first, then each stack's rules. `label` is `global` or the stack name. The rule body is not returned.

`notes` lists the card's newest 10 `failure_notes` rows in the card's project, newest first, so the first note is the one the next attempt reads. `source` is the stored source. `attemptN` is the `n` of the attempt that wrote the note, or `0` when the note has no attempt in that project, such as an approver's note. The board list does not return `notes`.

`instructions` is the text of the card's newest `instructions` note, or `null` when there is none or it is blank (RFC-073). Every later attempt reads it next to the newest note of any other source. The board list does not return `instructions`.

## Board and intake

Routes in this section include `/api/projects/{projectId}/`. A non-member is `404` before the handler reads the card.

`GET /api/projects/{projectId}/cards`

```json
{
  "canRun": true,
  "readyCount": 2,
  "running": false,
  "cards": []
}
```

An empty store returns `canRun: false`, `readyCount: 0`, `running: false`, and `cards: []`. It does not return an error.

`readyCount` counts the project's `ready` cards whose dependencies are all `done` and that have no open run request, the cards that list `run`. `canRun` is true when `readyCount` is above zero and the caller's role may run (RFC-061). `running` is true while that project has an unprocessed `runs` row with no `card_id`. The worker closes those rows once a pass changes nothing (RFC-062), and the board polls while `running` is true.

`GET /api/projects/{projectId}/cards/{id}` returns one card object. Unknown id, or a card in another project, is `404`.

`GET /api/projects/{projectId}/cards/{id}/trace` and `GET /api/projects/{projectId}/cards/{id}/trace?since=2026-10-10T05:00:00Z` (RFC-074)

```json
{
  "live": false,
  "attempts": [
    {
      "id": "att_0123456789ab",
      "n": 1,
      "taskClass": "implement",
      "modelId": "example-model",
      "fallbackModelId": "",
      "inputTokens": 11883,
      "outputTokens": 2523,
      "maxTurns": 12,
      "commitSha": "ae5d4418f1f5c54e02c2ced82de2a5b66ba65753",
      "reviewModelId": "other-model",
      "reviewVerdict": "recommend_approve",
      "reviewIndependent": false,
      "startedAt": "2026-10-10T05:00:00Z",
      "endedAt": "2026-10-10T05:01:08Z",
      "outcome": "checks_passed"
    }
  ],
  "events": [
    {
      "id": "evt_0123456789ab",
      "seq": 4182,
      "ts": "2026-10-10T05:00:00Z",
      "attemptId": "att_0123456789ab",
      "kind": "model_call",
      "payload": { "role": "implementer", "modelId": "example-model", "inputTokens": 932, "outputTokens": 84, "stopReason": "tool_use", "text": "I will read the login page first." }
    }
  ]
}
```

Events are oldest first by `ts`, then by insert order (`seq`), so events in the same second keep the order they were written in. Each event has `seq`. `attemptId` is left out when the event has none.

`since` is an optional RFC3339 time. With it, the route returns only events with `ts` at or after 10 seconds before `since`, so an event that committed late or in the same second is read again; the client merges events by `id`. A value that is not RFC3339 is `422` with `fields.since`. `attempts` is always the full list. `live` is true while the card is queued (an open `runs` row), is in `in_progress`, `self_check`, `agent_review`, `deploying_dev`, or `verifying`, or has an attempt with no `endedAt`. The Trace page polls while `live` is true. `maxTurns` is the attempt's turn budget, `0` when it has none. Payload is the redacted JSON already stored. New transition payloads may include `actorId`. Events are read by card id after the route checks that the card is in the project, because older events may carry `proj_default` as their project.

`attempts` lists the card's attempts in the project by `n`. A value the attempt does not have yet is `""`, and `reviewIndependent` is `null` until a review ran. `outcome` is `""` while the attempt runs, then one of `checks_passed`, `checks_failed`, `no_commit`, `budget`, `error`, or `cancelled`. Tokens are the implementer's calls only; the reviewer's calls are in `model_call` events with `role: reviewer`, under the same `attemptId`.

`GET /api/projects/{projectId}/cards/{id}/diff` (RFC-068)

```json
{
  "sha": "ae5d4418f1f5c54e02c2ced82de2a5b66ba65753",
  "base": "main",
  "compareUrl": "https://github.com/acme/widget/compare/main...ae5d4418f1f5c54e02c2ced82de2a5b66ba65753",
  "missing": false,
  "truncated": false,
  "files": [
    {
      "path": "resources/js/Pages/Auth/Login.tsx",
      "added": 1,
      "deleted": 1,
      "binary": false,
      "truncated": false,
      "hunks": [
        {
          "header": "@@ -67,7 +67,7 @@",
          "lines": [
            { "kind": "ctx", "text": "<div>" },
            { "kind": "del", "text": "Username atau Email" },
            { "kind": "add", "text": "Username" }
          ]
        }
      ]
    }
  ]
}
```

Any member of the project's tribe may read it. A non-member, an unknown id, or a card in another project is `404`. `sha` is the card's `headSha`, or the commit of its last attempt when there is no head. With neither, `sha` is empty and `files` is `[]`. The diff runs from `refs/heads/<baseBranch>` to `sha` in the project clone, with renames shown as a delete and an add. A SHA not in the clone is fetched from `refs/heads/orch/<cardId>` with the company token. If it is still missing, or the project has no clone, `missing` is `true` and `files` is `[]`.

`kind` is `add`, `del`, `ctx`, or `meta`. An `add`, `del`, or `ctx` line's `text` drops the leading `+`, `-`, or space. A `meta` line is git's `\ No newline at end of file` as written. A binary file has `binary: true`, counts of `0`, and no hunks. The diff text is capped at 256 KiB. When it is cut, the response sets `truncated`, and the file it stopped in and the files it never reached set their own `truncated`, with counts from `git diff --numstat`. A file keeps at most 1,500 lines and sets `truncated` past that. Model keys and the company token are redacted before parsing. `compareUrl` is empty when the company account has no host or organization, or the project has no repository.

`GET /api/projects/{projectId}/intake`

```json
{
  "draftPending": false,
  "planError": null,
  "canAccept": true,
  "requirements": [
    {
      "id": "REQ-001",
      "sourceKind": "prd",
      "sourcePath": "docs/prd/coding-orchestrator.md",
      "heading": "REQ-001 Parse PRD and RFC markdown into records",
      "statement": "The orchestrator reads markdown PRDs and RFCs.",
      "acceptance": ["A fixture PRD yields one record per id."],
      "accepted": false,
      "createdBy": null,
      "canEditReviewers": true,
      "selfReview": false,
      "canSetSelfReview": true,
      "aiReviewMode": "continue",
      "reviewers": [{ "id": "user_0123456789ab", "name": "Grace Hopper", "email": "grace@example.com", "githubLogin": "grace" }],
      "linkedCards": []
    }
  ],
  "reviewerOptions": [{ "id": "user_0123456789ab", "name": "Grace Hopper", "email": "grace@example.com", "githubLogin": "grace" }],
  "decisions": [
    {
      "id": "RFC-001",
      "sourcePath": "docs/rfc/0001-v1-architecture.md",
      "statement": "Card status changes only inside the harness transition function."
    }
  ]
}
```

`linkedCards` lists the ids of that project's cards whose `requirementIds` name the requirement, oldest card first. It is `[]` when no card names it. A card in another project is never listed.

`POST /api/projects/{projectId}/ingest` has an empty body. It reparses that project's PRD and RFC paths and returns the intake shape. Unchanged hashes do not duplicate rows. A changed hash clears `accepted_at` for that id in that project and blocks linked cards past `backlog` with `source_changed` (REQ-004). When a PRD has no `REQ-` headings, the response has `draftPending: true` and the project's worker runs the planner to draft ids (REQ-002). `planError` holds the last planner failure message, redacted, until the next successful plan. A caller who is not a member is `404`. Ingest does not overwrite a manual row. When the repository defines an id that is manual in that project, the row is kept and the response adds `"skipped": ["REQ-012"]`. The field is absent when nothing was skipped.

`canAccept` is true when the caller's role may accept (RFC-053). `createdBy` is the email of the user who added the row by hand or first ingested it, and `null` when no user is recorded (RFC-054). `reviewers` are the users named for that requirement. `canEditReviewers` is true when the caller is the row's creator or a Lead Developer member. `selfReview` is the row's Self review flag. `canSetSelfReview` is true when the caller is a Lead Developer or an admin (RFC-070), and the same caller may change `aiReviewMode`, which is `continue` or `manual` (RFC-072). `reviewerOptions` are the members of the project's tribe with a verified GitHub login who are not disabled. The client leaves out the row's creator.

`PUT /api/projects/{projectId}/requirements/{id}/reviewers` replaces the reviewers of one requirement, parsed or manual, and returns the intake shape.

```json
{ "userIds": ["user_0123456789ab"] }
```

Allowed for the row's creator or a Lead Developer member. Anyone else is `403`, and a non-member is `404`. An unknown id is `404`. A user id that is not a tribe member with a verified login, or is the row's creator, is `422` on `userIds`. At most 15 reviewers.

`PUT /api/projects/{projectId}/requirements/{id}/self-review` turns Self review on or off for one requirement, parsed or manual, and returns the intake shape (RFC-070).

```json
{ "selfReview": true }
```

Allowed for a Lead Developer or an admin who is a member. Anyone else is `403`, including the row's creator, and a non-member is `404`. An unknown id is `404`.

`PUT /api/projects/{projectId}/requirements/{id}/ai-review-mode` sets what an AI request for changes does on that requirement's cards, and returns the intake shape (RFC-072).

```json
{ "aiReviewMode": "manual" }
```

`continue` sends the card back with a `retry` row. `manual` lets the pull request checks decide, so a person sees the verdict in `human_review`. The same callers as the self-review route are allowed, with the same `403` and `404` answers. A value other than `continue` or `manual` is `422` on `aiReviewMode`.

`POST /api/projects/{projectId}/requirements` adds a manual requirement. Any member may call it.

```json
{ "id": "", "statement": "Export the board as CSV.", "acceptance": ["The file has one row per card."], "selfReview": false, "aiReviewMode": "continue" }
```

`selfReview` is optional and defaults to `false`. `true` from a caller who is not a Lead Developer or an admin is `403` and adds nothing (RFC-070). `aiReviewMode` is optional and defaults to `continue`. `manual` from that caller is `403`, and an unknown value is `422` on `aiReviewMode` (RFC-072).

`id` is optional. A non-empty id must match `^REQ-[0-9]+$`. A blank id gets one above the highest `REQ-` number in the project, at least three digits. The number is chosen while holding the project's `intake_state` row, so two adds do not take the same id. `statement` is required, at most 10000 characters. It is markdown text and is stored as written, with CR and CRLF line endings turned into LF. Over the limit, `fields.statement` is "Keep the statement to 10000 characters or fewer." `acceptance` is a list of non-empty lines, at most 50, each at most 1000 characters ("Keep each acceptance line to 1000 characters or fewer."). Each line is inline markdown. A line break inside a line becomes a space, and one leading list or checkbox marker (`- `, `* `, `+ `, `1. `, `1) `, optionally followed by `[ ]` or `[x]`) is dropped, because the pull request writes each line as `- [ ] `. Each element is one check; the client decides how its text is split into checks. The row is not accepted. The response is the intake shape. A bad field is `422` with `fields` keyed `id`, `statement`, or `acceptance`. An id that already exists in that project is `409`.

`PUT /api/projects/{projectId}/requirements/{id}` takes `{ "statement": "...", "acceptance": [] }` with the same rules. It changes only a manual row. A changed statement or acceptance list clears `accepted_at` and `accepted_by` (REQ-004). A row parsed from the repository or drafted by the planner is `409`. An unknown id is `404`.

`DELETE /api/projects/{projectId}/requirements/{id}` removes a manual row and returns the intake shape. A row that is not manual, or a manual row that a card in that project lists in `requirementIds`, is `409`. An unknown id is `404`.

`POST /api/projects/{projectId}/accept`

```json
{ "requirementIds": ["REQ-001", "REQ-002"] }
```

Allowed for a Lead Developer, Senior Developer, QA Lead, or Product Owner member (RFC-053). A Developer or QA member is `403`, and a non-member is `404`. Stores that user's id and the time on those ids, then inserts a `runs` row with trigger `accept` so the planner can create cards. Ids absent from the body are left as they are. Unknown ids are `422`.

## Runs and human gates

`POST /api/projects/{projectId}/runs` has an empty body. A Lead Developer, Senior Developer, Developer, or QA member may call it. Anyone else is `403`. It inserts one card-scoped `runs` row for every card of that project that lists `run`, in one transaction, and returns immediately. `queued` is how many rows it added. `readyCount` is what the board reads afterwards. When nothing can move, the response is still `200` with `queued: 0` (RFC-061).

```json
{ "queued": 2, "readyCount": 0 }
```

`POST /api/projects/{projectId}/cards/{id}/run` takes an optional body `{ "instructions": "..." }`. The same roles may call it. It locks the card row, then inserts one `runs` row with `card_id` set and returns the card object with `queued: true`. A card that already has an open row keeps that one row, and the call still returns `200`. A card that is not `ready`, `changes_requested`, or `in_progress`, that has an open attempt, that has used all its attempts (RFC-071), or that has a dependency not `done`, is `409` `conflict` (RFC-063). The card's status does not change. The worker starts the implement job for the row under the project's cap and sets `processed_at` when it starts. When the body has `instructions`, the call saves them as the instructions route does, in the same transaction as the `runs` row, so a refused run saves nothing (RFC-073). A body that is not JSON, or an `instructions` that is not a string, is `422` on `body`.

`PUT /api/projects/{projectId}/cards/{id}/instructions` sets the text every later attempt of the card reads, and returns the card object (RFC-073). The roles that may run a card may call it. Anyone else is `403`.

```json
{ "instructions": "Run the lint scripts in every package." }
```

The server turns CRLF into LF and trims outer whitespace. Blank text clears the instructions. Text equal to the current instructions writes nothing. Otherwise it inserts a failure note with source `instructions`, or updates the instructions note written in the same second, and writes an `instructions_saved` event with the caller as actor and `{ "length": n }` as payload. More than 4000 characters is `422` with `fields.instructions` `Keep the instructions to 4000 characters or fewer.`. A `done` card is `409`. A card in another project is `404`. A method other than `PUT` is `405`. An attempt that already started keeps the prompt it started with.

`POST /api/projects/{projectId}/cards/{id}/approve`

```json
{ "sha": "9c1e5b7d0a4f2e6c8b3d1a7f5e9c2b4d6a8f0e1c" }
```

The approver is the session user. The role must be Lead Developer, QA Lead, or Product Owner, and the user must be a member. The server refreshes the head from GitHub with the company token, rejects a missing GitHub login, rejects the company bot login, rejects the card owner (`403`, RFC-054), and checks that `sha` equals the head. Approving the same card and SHA twice returns the card and does not insert a second approval. A SHA mismatch is `422` with `fields.sha`. A missing GitHub login is `422` with `fields.login`. Approval does not start deploy. When the card's project has `approval.solo` on, or every requirement of the card has Self review on, the owner and the bot login are not rejected, and their approval row stores `independent = false` (RFC-067, RFC-070).

After an approval that inserted a new row, the server posts with the company token a comment on the card's pull request (`POST repos/{owner}/{repo}/issues/{n}/comments`) and a commit status on the approved SHA (`POST repos/{owner}/{repo}/statuses/{sha}`) with `state: success`, `context: orchestrator/approval`, and `description: Approved by @<login> in the orchestrator`. The posts take at most 30 seconds and do not change the response: a failed post writes a `job_error` event whose message starts with `approval comment:` or `approval status:`, and the card stays `approved` (RFC-068).

`POST /api/projects/{projectId}/cards/{id}/request-changes` takes an optional body `{ "reason": "..." }`. `POST /api/projects/{projectId}/cards/{id}/reject` has an empty body. Both work from `human_review` and `approved` when the role allows them. Request-changes clears approval and, in the same transaction, writes a failure note with source `approver`. The note is the reason with CRLF turned into LF and outer whitespace trimmed, or `changes requested` when the reason is blank. A reason over 4000 characters is `422` with `fields.reason` `Keep the reason to 4000 characters or fewer.`, and the card does not move. Reject sets `blockedReason` to `rejected`. When the card was `approved`, either one posts `orchestrator/approval` with `state: failure` and `description: Approval withdrawn` on the approved SHA (RFC-068).

`POST /api/projects/{projectId}/cards/{id}/deploy` has an empty body. It is allowed when `deploy` is in `actions`. The card moves from `approved` toward `deploying_dev` through `machine.Transition`, and a `runs` row with trigger `deploy` wakes that project's worker.

`POST /api/projects/{projectId}/cards/{id}/unblock`

```json
{ "resetAttempts": false }
```

The body is optional. The card goes to `unblockTo`. With `resetAttempts: false`, `attemptsUsed` stays, and `maxAttempts` rises to `attemptsUsed + 1` when the count is already at the cap. With `resetAttempts: true`, `attemptsUsed` becomes `0` and `maxAttempts` returns to the router budget.

Approve, deploy, and unblock insert a `runs` row with the matching trigger so the board continues without a second click. Request-changes does not: the card waits in `changes_requested` until someone presses Run. Only an AI request for changes on a `continue` card queues the next attempt by itself (RFC-071, RFC-072). Each route returns the card object. An action not listed for the card is `422` `transition_rejected`. A status change between read and write is `409` `conflict`. A role that cannot perform the action is `403`.

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
| Caller | For project sections, a Lead Developer member, a tribe admin of the project's tribe, or a company admin, member or not (RFC-059). Admin for the company GitHub account and the conventions catalog. The account GitHub route is the signed-in user. Another member is `403`. A non-member who is not a company admin is `404` |
| `canEdit` | Every project settings `GET` and `PUT` body includes it. True when the caller may save that section |

Account GitHub is `GET` and `PUT /api/me/github`, described above. `POST /api/me/github/test` runs `gh api user` with the caller's stored token in the child environment. On success it stores the login and numeric id GitHub reports, sets `verified_at`, and returns `{ "ok": true, "login": "octocat" }`. A failure returns `{ "ok": false, "message": "authentication failed" }` and leaves the row unverified. A login another user already holds as verified on that host is `{ "ok": false, "message": "login already verified by another user" }`.

`GET /api/settings/github` is the company GitHub account (RFC-049). Admin only.

```json
{
  "host": "github.com",
  "org": "acme",
  "botLogin": "acme-bot",
  "tokenSet": true
}
```

`PUT /api/settings/github` accepts `host`, `org`, optional `token`, and `clear`. `botLogin` is not accepted. The test sets it.

| Field | Rule |
| --- | --- |
| `host` | Hostname only, no scheme or path |
| `org` | `^[A-Za-z0-9._-]+$` |

`POST /api/settings/github/test` runs `gh api --include user`, `gh api orgs/{org}`, and the repository list below, with `GH_TOKEN` and `GH_HOST` in the child environment and an empty `GH_CONFIG_DIR`. It stores the login as `botLogin` and returns what the token sees, or `{ "ok": false, "message": "..." }`.

```json
{
  "ok": true,
  "login": "acme-bot",
  "tokenKind": "classic",
  "scopes": ["public_repo", "workflow"],
  "repoCount": 0,
  "privateCount": 0,
  "hint": "The token has no repo scope, so private repositories are hidden."
}
```

`tokenKind` comes from the token prefix: `ghp_` is `classic`, `github_pat_` is `fine-grained`, anything else is `other`. `scopes` is the `X-OAuth-Scopes` header, empty for a fine-grained token. `hint` is empty when the token sees the organization's repositories. Otherwise it names the likely cause: a classic token without `repo`; a fine-grained token that sees none, which needs the organization as its resource owner; any other token that sees none, which may need SAML SSO authorization. No response contains the token. A completed check is `200`, including `ok: false`. No stored token is `422`.

`GET /api/settings/github/repos` lists the organization's repositories for an admin, a Lead Developer, or a tribe admin: `{ "repos": [{ "name": "widget", "defaultBranch": "main", "archived": false, "private": true }] }`. It merges `orgs/{org}/repos?type=all` with `user/repos?affiliation=owner,collaborator,organization_member` kept to repositories the organization owns, so a repository the token's user reaches as a collaborator is listed too. Names are deduplicated without case and sorted. No stored token is `422`. A failed call is `502`.

`GET /api/projects/{projectId}/settings/repository`

```json
{
  "host": "github.com",
  "org": "acme",
  "repo": "widget",
  "baseBranch": "main",
  "cloned": true,
  "headSha": "0123456789abcdef0123456789abcdef01234567",
  "canEdit": true
}
```

`PUT /api/projects/{projectId}/settings/repository` accepts `repo` and `baseBranch`. `host` and `org` come from the company account and are read only here. When either one changes, the server runs the same GitHub check as `POST /api/projects`, with the same `422` and `502` answers. A save that changes neither makes no GitHub call. A Lead Developer member, a tribe admin of the project's tribe, or a company admin may save (RFC-059). Another member is `403`.

| Field | Rule |
| --- | --- |
| `repo` | `^[A-Za-z0-9._-]+$`, not named by another project |
| `baseBranch` | Valid by `git check-ref-format --branch`, and not `orch/*` |

`POST /api/projects/{projectId}/sync` clones the repository into `<data>/projects/<id>/repo` when it is missing, otherwise fetches `baseBranch`, then returns the repository object. The token is passed through askpass and the child environment. A missing company token is `422`. A failed git call is `502` with a redacted message. Lead Developer, Senior Developer, and Developer members may sync, and so may anyone who may save the project's settings (RFC-059).

`GET /api/projects/{projectId}/settings/links` returns the projects this project reads from (RFC-051).

```json
{
  "links": [
    { "projectId": "proj_0123456789ab", "name": "Widget API", "repo": "widget-api", "sharedPaths": ["api/openapi.yaml"] }
  ],
  "canEdit": true
}
```

`PUT /api/projects/{projectId}/settings/links` accepts `{ "links": [{ "projectId": "...", "sharedPaths": ["..."] }] }` and replaces the list. A Lead Developer member, a tribe admin of the project's tribe, or a company admin may save (RFC-059). Another member is `403`.

| Field | Rule |
| --- | --- |
| `projectId` | Another project in the same tribe. Not this project. Unknown is `422` |
| `sharedPaths` | At most 20 per link. Each is relative, has no `..`, and is not under `.git` or `.orchestrator` |

LLM APIs and the router are one company setting (RFC-052). Every signed-in user may GET them. Only an admin may PUT them or run a family test. Anyone else gets `403`. A successful PUT appends `config_changed` with the field names and `actorId`.

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

`POST /api/settings/llm/{family}/test` is admin only. It returns `{ "ok": true }` or `{ "ok": false, "message": "..." }` with `200`. Unknown family is `404`. A family that needs a key and has none is `422`.

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

`PUT /api/settings/router` is admin only. It sends `routes` and `budgets`. A non-empty model or fallback must exist on a saved family. The review sentinel `different-family-from-author` is allowed only on `review.model`. `fallback` may be empty, which means no fallback. An empty `model` may be saved. A job that needs that class then blocks with `router_config`. Budgets are integers greater than zero, with `maxAttempts` at most 10.

The coding rules, stacks, and integration skills are one company catalog (RFC-055, RFC-056). Every signed-in user may GET it. Only an admin may create, update, or delete an item. Anyone else gets `403`. A successful write appends `config_changed` with section `conventions` and the changed field names.

`GET /api/conventions`

```json
{
  "rules": [
    {
      "id": "rule_0123456789ab",
      "scope": "stack",
      "stackId": "stk_go",
      "category": "naming",
      "title": "Go names",
      "body": "Exported names are MixedCaps ...",
      "version": 2,
      "contentHash": "<sha256 hex>",
      "disabled": false,
      "updatedBy": "ada@example.com",
      "updatedAt": "2026-10-10T05:00:00Z"
    }
  ],
  "stacks": [
    {
      "id": "stk_go",
      "name": "Go",
      "detect": { "files": ["go.mod"], "npmDeps": [], "pyDeps": [] },
      "version": 1,
      "contentHash": "<sha256 hex>",
      "disabled": false,
      "updatedBy": null,
      "updatedAt": "2026-10-10T05:00:00Z"
    }
  ],
  "skills": [
    {
      "id": "skl_oauth2",
      "slug": "oauth2-integration",
      "title": "OAuth 2.0 integration",
      "useWhen": "A requirement adds sign-in with an outside identity provider",
      "body": "Steps ...",
      "version": 1,
      "contentHash": "<sha256 hex>",
      "disabled": false,
      "updatedBy": null,
      "updatedAt": "2026-10-10T05:00:00Z"
    }
  ]
}
```

`POST /api/conventions/rules` creates a rule and returns it with `201`. `PUT /api/conventions/rules/{id}` replaces its fields and returns it. The body is `scope`, `stackId`, `category`, `title`, `body`, and `disabled`. Stacks use `POST /api/conventions/stacks` and `PUT /api/conventions/stacks/{id}` with `name`, `detect`, and `disabled`. Skills use `POST /api/conventions/skills` and `PUT /api/conventions/skills/{id}` with `slug`, `title`, `useWhen`, `body`, and `disabled`. A PUT whose hashed fields are unchanged keeps the version. `DELETE /api/conventions/{rules|stacks|skills}/{id}` returns `204`: it deletes an item nothing names, and disables a stack that a rule or project names or a skill that a project or card names. An unknown id is `404`.

| Field | Rule |
| --- | --- |
| `scope` | `global` or `stack` |
| `stackId` | Empty for `global`. An existing stack for `stack` |
| `category` | `naming`, `database`, `style`, `framework`, `security`, or `testing` |
| `title` | 1 to 120 characters |
| `body` | 1 to 8000 characters |
| `name` | 1 to 60 characters, unique ignoring case |
| `detect.files` | At most 20 file names or globs, `^[A-Za-z0-9._*+-]+$`, no slash |
| `detect.npmDeps` | At most 20 npm package names |
| `detect.pyDeps` | At most 20 Python package names, `^[A-Za-z0-9._-]+$` |
| `slug` | `^[a-z0-9][a-z0-9-]{1,59}$`, unique |
| `useWhen` | 1 to 500 characters |

`GET /api/projects/{projectId}/settings/conventions` is the project's stacks, enabled skills, and the rules that apply. Every member, and a company admin, may read it.

```json
{
  "stacks": [
    { "id": "stk_go", "name": "Go", "source": "detected" },
    { "id": "stk_django", "name": "Django", "source": "manual" }
  ],
  "skillIds": ["skl_oauth2"],
  "rules": [
    { "id": "rule_0123456789ab", "title": "Go names", "scope": "stack", "stackName": "Go", "category": "naming", "version": 2 }
  ],
  "canEdit": true
}
```

`PUT /api/projects/{projectId}/settings/conventions` accepts `{ "manualStackIds": [], "skillIds": [] }`, at most 30 of each, and returns the GET body. Each id must name an enabled stack or skill, else `422` on that field. A manual id that is already detected stays detected. A Lead Developer member, a tribe admin of the project's tribe, or a company admin may save (RFC-059). Another member is `403`. A save appends `config_changed` with section `conventions` for the project.

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
  "canEdit": true,
  "localCheck": "",
  "devCheck": "",
  "allowExecutables": [],
  "sandboxEngine": "podman",
  "sandboxImage": "",
  "implementationCap": 2,
  "prdPath": "docs/prd",
  "rfcPath": "docs/rfc",
  "soloApproval": false
}
```

`PUT /api/projects/{projectId}/settings/deploy` accepts those fields plus optional `deployToken` and `clear`. A Lead Developer member, a tribe admin of the project's tribe, or a company admin may save (RFC-059). Another member is `403`.

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
| `soloApproval` | Boolean. Lets the card owner and the company bot login approve (RFC-067) |

Empty deploy fields may be saved. A deploy then blocks with `deploy_config` and runs no command. A non-empty `devCheck` with an empty `devUrl` is also `deploy_config` at deploy time. Changing `localCheck` or `devCheck` applies the rule in 0006 to existing cards.

## Files on disk

The service data directory defaults to `~/.orchestrator/service` (RFC-047). Each project directory must sit outside that project's repo (RFC-036, RFC-048).

```text
<data>/                                      mode 0700
<data>/config.yaml                           mode 0600   LLM families and router
<data>/secrets.json                          mode 0600
<data>/projects/<project-id>/                mode 0700
<data>/projects/<project-id>/config.yaml     mode 0600
<data>/projects/<project-id>/secrets.json    mode 0600
<data>/projects/<project-id>/worker.lock
<data>/projects/<project-id>/repo/                       the clone of the project repository
<data>/projects/<project-id>/empty-hooks/
<data>/projects/<project-id>/empty-gh/
<data>/projects/<project-id>/askpass/
<data>/projects/<project-id>/worktrees/<card-id>/
<data>/projects/<project-id>/exports/<card-id>-<sha>/   deleted after each check
<data>/projects/<project-id>/exports/linked-<linked-id>-<sha>/   read-only linked context
<data>/projects/<project-id>/logs/<attempt-id>.log
<data>/projects/<project-id>/logs/<attempt-id>-review.json
```

The board is not a file in this directory. It is the PostgreSQL database named by `databaseUrl` in the service secrets file (RFC-044, RFC-047).

Service `config.yaml` holds the company LLM families and router:

```yaml
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
```

Project `config.yaml`:

```yaml
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
approval:
  solo: false
```

The deploy settings route flattens `deploy`, `checks`, `sandbox`, `scheduler`, `paths`, and `approval` into one object, as shown above. Service `secrets.json`:

```json
{
  "databaseUrl": "",
  "tokenKey": "",
  "llmKeys": {
    "anthropic": "",
    "openai": ""
  }
}
```

Project `secrets.json`:

```json
{
  "deployToken": ""
}
```

`databaseUrl` is the PostgreSQL URL for every project's board (RFC-047). `tokenKey` encrypts user GitHub tokens and the company GitHub token. `llmKeys` are the company model keys. Empty string means unset. The API never returns these values. No GitHub token is in either secrets file. The repository and base branch are project columns, not `config.yaml` keys. Both secret files are written to a temp file in the same directory, synced, and renamed. The API never decodes a secrets file into a struct that is encoded to the client. Edit a project's `config.yaml` by hand only while that project's worker is stopped. Startup validates it with the same rules as the PUT routes.

## CLI twins

The CLI calls the same `internal/app` functions, not the HTTP API. It prints the error `message` on stderr and exits `1` on any error. It exits `0` on success, including a run where `canRun` is false.

| CLI | API |
| --- | --- |
| `orchestrator users bootstrap` | Creates the first admin when `users` is empty. No HTTP twin |
| `orchestrator ingest --project ID` | `POST /api/projects/{id}/ingest` |
| `orchestrator accept --project ID` | `POST /api/projects/{id}/accept` with every current unaccepted id |
| `orchestrator run-next --project ID` | Inserts a `cli` run row. When that project's lock is free, runs worker passes until idle and prints `created N` and `promoted M`, then runs the implement jobs of queued cards, waits for them, and prints `started N` when it started any, then `no card is ready` when none is. When the lock is held, prints `queued for the running server` (RFC-048, RFC-062, RFC-063) |
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
