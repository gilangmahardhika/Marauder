-- +goose Up
ALTER TABLE runners ADD COLUMN agents_json TEXT NOT NULL DEFAULT '["builtin"]';

-- +goose Down
ALTER TABLE runners DROP COLUMN agents_json;
