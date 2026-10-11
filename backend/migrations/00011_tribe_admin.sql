-- +goose Up
ALTER TABLE tribe_members ADD COLUMN admin BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE tribe_members DROP COLUMN admin;
