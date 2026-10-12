-- +goose Up
ALTER TABLE attempts DROP CONSTRAINT attempts_outcome_check;
ALTER TABLE attempts ADD CONSTRAINT attempts_outcome_check CHECK (outcome IN (
  'checks_passed', 'checks_failed', 'no_commit', 'no_change', 'budget', 'error', 'cancelled'));

-- +goose Down
UPDATE attempts SET outcome = 'no_commit' WHERE outcome = 'no_change';
ALTER TABLE attempts DROP CONSTRAINT attempts_outcome_check;
ALTER TABLE attempts ADD CONSTRAINT attempts_outcome_check CHECK (outcome IN (
  'checks_passed', 'checks_failed', 'no_commit', 'budget', 'error', 'cancelled'));
