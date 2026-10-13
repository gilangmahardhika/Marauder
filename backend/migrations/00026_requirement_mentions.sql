-- +goose Up
ALTER TABLE requirements ADD COLUMN mentions_json TEXT NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE requirements DROP COLUMN mentions_json;
