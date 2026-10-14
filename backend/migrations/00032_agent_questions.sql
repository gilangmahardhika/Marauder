-- +goose Up
ALTER TABLE cards DROP CONSTRAINT cards_status_check;
ALTER TABLE cards ADD CONSTRAINT cards_status_check CHECK (status IN (
  'backlog', 'ready', 'in_progress', 'self_check', 'pr_open', 'agent_review',
  'changes_requested', 'human_review', 'approved', 'deploying_dev',
  'verifying', 'done', 'blocked', 'wont_fix', 'waiting_answer'));

ALTER TABLE attempts DROP CONSTRAINT attempts_outcome_check;
ALTER TABLE attempts ADD CONSTRAINT attempts_outcome_check CHECK (outcome IN (
  'checks_passed', 'checks_failed', 'no_commit', 'no_change', 'budget', 'error', 'cancelled',
  'question'));

ALTER TABLE cards ADD COLUMN question_mode TEXT NOT NULL DEFAULT 'manual'
  CHECK (question_mode IN ('manual', 'auto'));

CREATE TABLE card_questions (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id),
  card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
  attempt_id TEXT NOT NULL REFERENCES attempts(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  question TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('single', 'multiple', 'text')),
  options_json TEXT NOT NULL,
  allow_other BOOLEAN NOT NULL DEFAULT false,
  recommended_json TEXT NOT NULL,
  why TEXT NOT NULL DEFAULT '',
  answer_json TEXT,
  answer_source TEXT CHECK (answer_source IN ('person', 'suggestion', 'auto')),
  answered_by TEXT REFERENCES users(id),
  answered_at TEXT,
  created_at TEXT NOT NULL,
  UNIQUE (attempt_id, position)
);
CREATE INDEX card_questions_card ON card_questions (project_id, card_id);

-- +goose Down
DROP TABLE card_questions;
ALTER TABLE cards DROP COLUMN question_mode;

UPDATE attempts SET outcome = 'no_commit' WHERE outcome = 'question';
ALTER TABLE attempts DROP CONSTRAINT attempts_outcome_check;
ALTER TABLE attempts ADD CONSTRAINT attempts_outcome_check CHECK (outcome IN (
  'checks_passed', 'checks_failed', 'no_commit', 'no_change', 'budget', 'error', 'cancelled'));

UPDATE cards SET status = 'ready' WHERE status = 'waiting_answer';
ALTER TABLE cards DROP CONSTRAINT cards_status_check;
ALTER TABLE cards ADD CONSTRAINT cards_status_check CHECK (status IN (
  'backlog', 'ready', 'in_progress', 'self_check', 'pr_open', 'agent_review',
  'changes_requested', 'human_review', 'approved', 'deploying_dev',
  'verifying', 'done', 'blocked', 'wont_fix'));
