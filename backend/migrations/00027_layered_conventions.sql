-- +goose Up
CREATE TABLE tribe_conventions (
  tribe_id TEXT NOT NULL REFERENCES tribes(id),
  kind TEXT NOT NULL,
  item_id TEXT NOT NULL,
  mode TEXT NOT NULL,
  added_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (tribe_id, kind, item_id),
  CHECK ((kind IN ('stack', 'skill') AND mode = 'on') OR (kind = 'rule' AND mode = 'off'))
);
CREATE INDEX tribe_conventions_item ON tribe_conventions(kind, item_id);

CREATE TABLE project_conventions (
  project_id TEXT NOT NULL REFERENCES projects(id),
  kind TEXT NOT NULL CHECK (kind IN ('stack', 'skill', 'rule')),
  item_id TEXT NOT NULL,
  mode TEXT NOT NULL CHECK (mode IN ('on', 'off')),
  added_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, kind, item_id)
);
CREATE INDEX project_conventions_item ON project_conventions(kind, item_id);

CREATE TABLE card_context (
  project_id TEXT NOT NULL REFERENCES projects(id),
  card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('skill', 'rule', 'mcp')),
  item_id TEXT NOT NULL,
  mode TEXT NOT NULL CHECK (mode IN ('attach', 'detach')),
  added_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, card_id, kind, item_id)
);
CREATE INDEX card_context_item ON card_context(kind, item_id);

INSERT INTO project_conventions (project_id, kind, item_id, mode, added_by, created_at)
SELECT project_id, 'skill', skill_id, 'on', added_by, created_at FROM project_skills;

INSERT INTO project_conventions (project_id, kind, item_id, mode, added_by, created_at)
SELECT project_id, 'stack', stack_id, 'on', added_by, created_at FROM project_stacks WHERE source = 'manual';

DELETE FROM project_stacks WHERE source = 'manual';

DROP TABLE project_skills;

-- +goose Down
CREATE TABLE project_skills (
  project_id TEXT NOT NULL REFERENCES projects(id),
  skill_id TEXT NOT NULL REFERENCES skills(id),
  added_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, skill_id)
);
CREATE INDEX project_skills_skill ON project_skills(skill_id);

INSERT INTO project_skills (project_id, skill_id, added_by, created_at)
SELECT c.project_id, c.item_id, c.added_by, c.created_at FROM project_conventions c
JOIN skills k ON k.id = c.item_id WHERE c.kind = 'skill' AND c.mode = 'on';

INSERT INTO project_stacks (project_id, stack_id, source, added_by, created_at)
SELECT c.project_id, c.item_id, 'manual', c.added_by, c.created_at FROM project_conventions c
JOIN stacks s ON s.id = c.item_id WHERE c.kind = 'stack' AND c.mode = 'on'
ON CONFLICT (project_id, stack_id) DO NOTHING;

DROP TABLE card_context;
DROP TABLE project_conventions;
DROP TABLE tribe_conventions;
