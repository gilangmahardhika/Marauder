-- +goose Up
ALTER TABLE cards DROP CONSTRAINT cards_status_check;
ALTER TABLE cards ADD CONSTRAINT cards_status_check CHECK (status IN (
  'backlog', 'ready', 'in_progress', 'self_check', 'pr_open', 'agent_review',
  'changes_requested', 'human_review', 'approved', 'deploying_dev',
  'verifying', 'done', 'blocked', 'wont_fix'));

-- +goose Down
UPDATE cards SET status = 'blocked' WHERE status = 'wont_fix';
ALTER TABLE cards DROP CONSTRAINT cards_status_check;
ALTER TABLE cards ADD CONSTRAINT cards_status_check CHECK (status IN (
  'backlog', 'ready', 'in_progress', 'self_check', 'pr_open', 'agent_review',
  'changes_requested', 'human_review', 'approved', 'deploying_dev',
  'verifying', 'done', 'blocked'));
