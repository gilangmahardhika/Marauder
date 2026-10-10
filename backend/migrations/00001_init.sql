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
