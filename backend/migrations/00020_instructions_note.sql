-- +goose Up
ALTER TABLE failure_notes DROP CONSTRAINT failure_notes_source_check;
ALTER TABLE failure_notes ADD CONSTRAINT failure_notes_source_check
  CHECK (source IN ('local_check', 'review', 'ci', 'approver', 'dev_check', 'no_commit', 'instructions'));

-- +goose Down
DELETE FROM failure_notes WHERE source = 'instructions';
ALTER TABLE failure_notes DROP CONSTRAINT failure_notes_source_check;
ALTER TABLE failure_notes ADD CONSTRAINT failure_notes_source_check
  CHECK (source IN ('local_check', 'review', 'ci', 'approver', 'dev_check', 'no_commit'));
