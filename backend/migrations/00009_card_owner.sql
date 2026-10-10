-- +goose Up
ALTER TABLE cards ADD COLUMN owner_id TEXT REFERENCES users(id);

CREATE TABLE requirement_reviewers (
  project_id TEXT NOT NULL,
  requirement_id TEXT NOT NULL,
  user_id TEXT NOT NULL REFERENCES users(id),
  added_by TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, requirement_id, user_id),
  FOREIGN KEY (project_id, requirement_id) REFERENCES requirements(project_id, id) ON DELETE CASCADE
);

ALTER TABLE user_github ADD COLUMN github_id BIGINT;
ALTER TABLE user_github ADD COLUMN verified_at TEXT;
CREATE UNIQUE INDEX user_github_verified_login ON user_github(host, lower(login)) WHERE verified_at IS NOT NULL;

-- +goose Down
DROP INDEX user_github_verified_login;
ALTER TABLE user_github DROP COLUMN verified_at;
ALTER TABLE user_github DROP COLUMN github_id;
DROP TABLE requirement_reviewers;
ALTER TABLE cards DROP COLUMN owner_id;
