-- +goose Up
ALTER TABLE attempts DROP CONSTRAINT attempts_task_class_check;
ALTER TABLE attempts ADD CONSTRAINT attempts_task_class_check
  CHECK (task_class IN ('implement', 'implement_hard', 'plan'));

-- +goose Down
UPDATE attempts SET task_class = 'implement' WHERE task_class = 'plan';
ALTER TABLE attempts DROP CONSTRAINT attempts_task_class_check;
ALTER TABLE attempts ADD CONSTRAINT attempts_task_class_check
  CHECK (task_class IN ('implement', 'implement_hard'));
