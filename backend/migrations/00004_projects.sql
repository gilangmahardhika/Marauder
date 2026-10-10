-- +goose Up
CREATE TABLE tribes (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE projects (
  id TEXT PRIMARY KEY,
  tribe_id TEXT NOT NULL REFERENCES tribes(id),
  name TEXT NOT NULL,
  repo_path TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE tribe_members (
  tribe_id TEXT NOT NULL REFERENCES tribes(id),
  user_id TEXT NOT NULL REFERENCES users(id),
  PRIMARY KEY (tribe_id, user_id)
);

INSERT INTO tribes (id, name, created_at) VALUES ('tribe_default', 'Default', '2026-10-10T00:00:00Z');
INSERT INTO projects (id, tribe_id, name, repo_path, created_at)
VALUES ('proj_default', 'tribe_default', 'Default', '', '2026-10-10T00:00:00Z');

ALTER TABLE requirements ADD COLUMN project_id TEXT NOT NULL DEFAULT 'proj_default' REFERENCES projects(id);
ALTER TABLE requirements DROP CONSTRAINT requirements_pkey;
ALTER TABLE requirements ADD PRIMARY KEY (project_id, id);

ALTER TABLE decisions ADD COLUMN project_id TEXT NOT NULL DEFAULT 'proj_default' REFERENCES projects(id);
ALTER TABLE decisions DROP CONSTRAINT decisions_pkey;
ALTER TABLE decisions ADD PRIMARY KEY (project_id, id);

ALTER TABLE cards ADD COLUMN project_id TEXT NOT NULL DEFAULT 'proj_default' REFERENCES projects(id);
DROP INDEX cards_branch;
DROP INDEX cards_pr;
CREATE UNIQUE INDEX cards_branch ON cards(project_id, branch) WHERE branch IS NOT NULL;
CREATE UNIQUE INDEX cards_pr ON cards(project_id, pr_url) WHERE pr_url IS NOT NULL;
CREATE INDEX cards_project_status ON cards(project_id, status);

ALTER TABLE attempts ADD COLUMN project_id TEXT NOT NULL DEFAULT 'proj_default' REFERENCES projects(id);
ALTER TABLE locks ADD COLUMN project_id TEXT NOT NULL DEFAULT 'proj_default' REFERENCES projects(id);
ALTER TABLE gates ADD COLUMN project_id TEXT NOT NULL DEFAULT 'proj_default' REFERENCES projects(id);
ALTER TABLE approvals ADD COLUMN project_id TEXT NOT NULL DEFAULT 'proj_default' REFERENCES projects(id);
ALTER TABLE failure_notes ADD COLUMN project_id TEXT NOT NULL DEFAULT 'proj_default' REFERENCES projects(id);
ALTER TABLE runs ADD COLUMN project_id TEXT NOT NULL DEFAULT 'proj_default' REFERENCES projects(id);

ALTER TABLE events ADD COLUMN project_id TEXT NOT NULL DEFAULT 'proj_default' REFERENCES projects(id);
ALTER TABLE events ADD COLUMN actor_id TEXT;

ALTER TABLE intake_state DROP CONSTRAINT intake_state_pkey;
ALTER TABLE intake_state ADD COLUMN project_id TEXT NOT NULL DEFAULT 'proj_default' REFERENCES projects(id);
ALTER TABLE intake_state ADD PRIMARY KEY (project_id);

ALTER TABLE deploy_lock DROP CONSTRAINT deploy_lock_pkey;
ALTER TABLE deploy_lock DROP CONSTRAINT deploy_lock_id_check;
ALTER TABLE deploy_lock ADD COLUMN project_id TEXT NOT NULL DEFAULT 'proj_default' REFERENCES projects(id);
ALTER TABLE deploy_lock ADD PRIMARY KEY (project_id);

-- +goose Down
ALTER TABLE deploy_lock DROP CONSTRAINT deploy_lock_pkey;
ALTER TABLE deploy_lock DROP COLUMN project_id;
ALTER TABLE deploy_lock ADD PRIMARY KEY (id);
ALTER TABLE deploy_lock ADD CONSTRAINT deploy_lock_id_check CHECK (id = 1);

ALTER TABLE intake_state DROP CONSTRAINT intake_state_pkey;
ALTER TABLE intake_state DROP COLUMN project_id;
ALTER TABLE intake_state ADD PRIMARY KEY (id);

ALTER TABLE events DROP COLUMN actor_id;
ALTER TABLE events DROP COLUMN project_id;
ALTER TABLE runs DROP COLUMN project_id;
ALTER TABLE failure_notes DROP COLUMN project_id;
ALTER TABLE approvals DROP COLUMN project_id;
ALTER TABLE gates DROP COLUMN project_id;
ALTER TABLE locks DROP COLUMN project_id;
ALTER TABLE attempts DROP COLUMN project_id;

DROP INDEX cards_project_status;
DROP INDEX cards_branch;
DROP INDEX cards_pr;
ALTER TABLE cards DROP COLUMN project_id;
CREATE UNIQUE INDEX cards_branch ON cards(branch) WHERE branch IS NOT NULL;
CREATE UNIQUE INDEX cards_pr ON cards(pr_url) WHERE pr_url IS NOT NULL;

ALTER TABLE decisions DROP CONSTRAINT decisions_pkey;
ALTER TABLE decisions DROP COLUMN project_id;
ALTER TABLE decisions ADD PRIMARY KEY (id);

ALTER TABLE requirements DROP CONSTRAINT requirements_pkey;
ALTER TABLE requirements DROP COLUMN project_id;
ALTER TABLE requirements ADD PRIMARY KEY (id);

DROP TABLE tribe_members;
DROP TABLE projects;
DROP TABLE tribes;
