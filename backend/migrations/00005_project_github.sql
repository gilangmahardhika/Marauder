-- +goose Up
ALTER TABLE projects ADD COLUMN github_host TEXT NOT NULL DEFAULT 'github.com';
ALTER TABLE projects ADD COLUMN github_owner TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN github_repo TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN github_base TEXT NOT NULL DEFAULT 'main';
ALTER TABLE projects ADD COLUMN bot_login TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN bot_token_ciphertext TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE projects DROP COLUMN bot_token_ciphertext;
ALTER TABLE projects DROP COLUMN bot_login;
ALTER TABLE projects DROP COLUMN github_base;
ALTER TABLE projects DROP COLUMN github_repo;
ALTER TABLE projects DROP COLUMN github_owner;
ALTER TABLE projects DROP COLUMN github_host;
