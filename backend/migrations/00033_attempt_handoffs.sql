-- +goose Up
CREATE TABLE attempt_handoffs (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id),
  card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
  attempt_id TEXT NOT NULL UNIQUE REFERENCES attempts(id) ON DELETE CASCADE,
  summary TEXT NOT NULL DEFAULT '',
  summary_source TEXT NOT NULL CHECK (summary_source IN ('model', 'harness')),
  todos_json TEXT NOT NULL DEFAULT '[]',
  patch TEXT NOT NULL DEFAULT '',
  patch_base_sha TEXT NOT NULL DEFAULT '',
  patch_files INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  applied_at TEXT,
  applied_ok BOOLEAN,
  applied_reason TEXT NOT NULL DEFAULT ''
);
CREATE INDEX attempt_handoffs_card ON attempt_handoffs (project_id, card_id, created_at);

-- +goose Down
DROP TABLE attempt_handoffs;
