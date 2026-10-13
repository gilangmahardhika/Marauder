-- +goose Up
CREATE TABLE tribe_mcp_servers (
  id TEXT PRIMARY KEY,
  tribe_id TEXT NOT NULL REFERENCES tribes(id),
  slug TEXT NOT NULL,
  transport TEXT NOT NULL CHECK (transport IN ('http', 'stdio')),
  url TEXT NOT NULL DEFAULT '',
  image TEXT NOT NULL DEFAULT '',
  argv_json TEXT NOT NULL DEFAULT '[]',
  enabled BOOLEAN NOT NULL DEFAULT true,
  secret_names_json TEXT NOT NULL DEFAULT '[]',
  secrets_ciphertext TEXT NOT NULL DEFAULT '',
  updated_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (tribe_id, slug)
);

CREATE TABLE tribe_mcp_tools (
  server_id TEXT NOT NULL REFERENCES tribe_mcp_servers(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  schema_json TEXT NOT NULL,
  schema_sha256 TEXT NOT NULL,
  PRIMARY KEY (server_id, name)
);

-- +goose Down
DROP TABLE tribe_mcp_tools;
DROP TABLE tribe_mcp_servers;
