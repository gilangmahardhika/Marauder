-- +goose Up
CREATE TABLE runner_registrations (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  created_by TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  used_at TEXT,
  runner_id TEXT
);

CREATE TABLE runners (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  labels_json TEXT NOT NULL DEFAULT '[]',
  slots INTEGER NOT NULL CHECK (slots BETWEEN 1 AND 16),
  version TEXT NOT NULL DEFAULT '',
  protocol INTEGER NOT NULL DEFAULT 0,
  secret_hash TEXT NOT NULL UNIQUE,
  state TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'draining', 'removed')),
  heartbeat_at TEXT,
  created_at TEXT NOT NULL,
  created_by TEXT REFERENCES users(id)
);

CREATE TABLE runner_leases (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  runner_id TEXT NOT NULL REFERENCES runners(id),
  card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
  project_id TEXT NOT NULL REFERENCES projects(id),
  attempt_id TEXT,
  model_id TEXT NOT NULL DEFAULT '',
  fallback_model_id TEXT NOT NULL DEFAULT '',
  task_class TEXT NOT NULL DEFAULT '',
  leased_until TEXT NOT NULL,
  created_at TEXT NOT NULL,
  ended_at TEXT,
  outcome TEXT
);
CREATE UNIQUE INDEX runner_leases_open_card ON runner_leases(card_id) WHERE ended_at IS NULL;
CREATE INDEX runner_leases_open_runner ON runner_leases(runner_id) WHERE ended_at IS NULL;

ALTER TABLE cards ADD COLUMN runner_affinity TEXT REFERENCES runners(id);

-- +goose Down
ALTER TABLE cards DROP COLUMN runner_affinity;
DROP TABLE runner_leases;
DROP TABLE runners;
DROP TABLE runner_registrations;
