-- +goose Up
ALTER TABLE attempts ADD COLUMN tokens_cached INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE attempts DROP COLUMN tokens_cached;
