-- +goose Up
ALTER TABLE requirements ADD COLUMN self_review BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE requirements DROP COLUMN self_review;
