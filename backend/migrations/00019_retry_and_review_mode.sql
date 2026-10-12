-- +goose Up
ALTER TABLE requirements ADD COLUMN ai_review_mode TEXT NOT NULL DEFAULT 'continue'
  CHECK (ai_review_mode IN ('continue', 'manual'));
ALTER TABLE runs DROP CONSTRAINT runs_trigger_check;
ALTER TABLE runs ADD CONSTRAINT runs_trigger_check
  CHECK (trigger IN ('api', 'cli', 'accept', 'approve', 'request_changes', 'unblock', 'retry'));

-- +goose Down
DELETE FROM runs WHERE trigger = 'retry';
ALTER TABLE runs DROP CONSTRAINT runs_trigger_check;
ALTER TABLE runs ADD CONSTRAINT runs_trigger_check
  CHECK (trigger IN ('api', 'cli', 'accept', 'approve', 'request_changes', 'unblock'));
ALTER TABLE requirements DROP COLUMN ai_review_mode;
