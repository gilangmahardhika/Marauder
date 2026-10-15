-- +goose Up
ALTER TABLE intake_state ADD COLUMN unnumbered_json TEXT NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE intake_state DROP COLUMN unnumbered_json;
