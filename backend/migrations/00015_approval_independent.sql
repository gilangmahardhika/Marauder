-- +goose Up
ALTER TABLE approvals ADD COLUMN independent BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE approvals DROP COLUMN independent;
