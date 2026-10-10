-- +goose Up
CREATE TABLE workspace_github (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  host TEXT NOT NULL DEFAULT 'github.com',
  org TEXT NOT NULL DEFAULT '',
  bot_login TEXT NOT NULL DEFAULT '',
  token_ciphertext TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
INSERT INTO workspace_github (id, updated_at) VALUES (1, '2026-10-10T00:00:00Z');

ALTER TABLE projects DROP COLUMN bot_token_ciphertext;
ALTER TABLE projects DROP COLUMN bot_login;
ALTER TABLE projects DROP COLUMN github_owner;
ALTER TABLE projects DROP COLUMN github_host;

UPDATE projects p SET github_repo = ''
WHERE p.github_repo <> '' AND EXISTS (
  SELECT 1 FROM projects q WHERE q.github_repo = p.github_repo AND q.id < p.id
);
CREATE UNIQUE INDEX projects_repo ON projects(github_repo) WHERE github_repo <> '';

-- +goose Down
DROP INDEX projects_repo;
ALTER TABLE projects ADD COLUMN github_host TEXT NOT NULL DEFAULT 'github.com';
ALTER TABLE projects ADD COLUMN github_owner TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN bot_login TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN bot_token_ciphertext TEXT NOT NULL DEFAULT '';
DROP TABLE workspace_github;
