-- +goose Up
ALTER TABLE attempts DROP CONSTRAINT attempts_outcome_check;
ALTER TABLE attempts ADD CONSTRAINT attempts_outcome_check CHECK (outcome IN (
  'checks_passed', 'checks_failed', 'no_commit', 'no_change', 'budget', 'error', 'cancelled',
  'question', 'plan'));

ALTER TABLE cards ADD COLUMN plan_mode TEXT NOT NULL DEFAULT 'required'
  CHECK (plan_mode IN ('required', 'off'));

CREATE TABLE card_plans (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id),
  card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
  attempt_id TEXT NOT NULL REFERENCES attempts(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  steps_json TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('open', 'accepted')),
  accepted_by TEXT REFERENCES users(id),
  accepted_at TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX card_plans_card ON card_plans (project_id, card_id);
CREATE UNIQUE INDEX card_plans_one_open ON card_plans (card_id) WHERE status = 'open';

-- +goose Down
DROP TABLE card_plans;
ALTER TABLE cards DROP COLUMN plan_mode;

UPDATE attempts SET outcome = 'no_commit' WHERE outcome = 'plan';
ALTER TABLE attempts DROP CONSTRAINT attempts_outcome_check;
ALTER TABLE attempts ADD CONSTRAINT attempts_outcome_check CHECK (outcome IN (
  'checks_passed', 'checks_failed', 'no_commit', 'no_change', 'budget', 'error', 'cancelled',
  'question'));
