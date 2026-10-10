-- +goose Up
CREATE TABLE project_links (
  project_id TEXT NOT NULL REFERENCES projects(id),
  linked_project_id TEXT NOT NULL REFERENCES projects(id),
  shared_paths_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, linked_project_id),
  CHECK (project_id <> linked_project_id)
);

-- +goose Down
DROP TABLE project_links;
