-- +goose Up
ALTER TABLE attempts ADD COLUMN pr_title TEXT;
ALTER TABLE attempts ADD COLUMN pr_body TEXT;

-- +goose Down
ALTER TABLE attempts DROP COLUMN pr_body;
ALTER TABLE attempts DROP COLUMN pr_title;
