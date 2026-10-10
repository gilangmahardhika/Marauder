-- +goose Up
CREATE TABLE users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN (
    'lead_developer', 'senior_developer', 'developer',
    'qa_lead', 'qa', 'product_owner')),
  admin BOOLEAN NOT NULL DEFAULT false,
  disabled BOOLEAN NOT NULL DEFAULT false,
  created_at TEXT NOT NULL
);

CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  bearer_hash TEXT NOT NULL UNIQUE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE user_github (
  user_id TEXT PRIMARY KEY REFERENCES users(id),
  host TEXT NOT NULL,
  login TEXT NOT NULL DEFAULT '',
  token_ciphertext TEXT NOT NULL DEFAULT ''
);

CREATE TABLE login_lockouts (
  email TEXT PRIMARY KEY,
  fails INTEGER NOT NULL DEFAULT 0,
  paused_until TEXT
);

-- +goose Down
DROP TABLE login_lockouts;
DROP TABLE user_github;
DROP TABLE sessions;
DROP TABLE users;
