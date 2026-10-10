-- +goose Up
CREATE TABLE stacks (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  detect_json TEXT NOT NULL DEFAULT '{}',
  version INTEGER NOT NULL DEFAULT 1,
  content_hash TEXT NOT NULL,
  disabled BOOLEAN NOT NULL DEFAULT false,
  updated_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX stacks_name ON stacks(lower(name));

CREATE TABLE convention_rules (
  id TEXT PRIMARY KEY,
  scope TEXT NOT NULL CHECK (scope IN ('global', 'stack')),
  stack_id TEXT REFERENCES stacks(id),
  category TEXT NOT NULL CHECK (category IN ('naming', 'database', 'style', 'framework', 'security', 'testing')),
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1,
  content_hash TEXT NOT NULL,
  disabled BOOLEAN NOT NULL DEFAULT false,
  updated_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK ((scope = 'global') = (stack_id IS NULL))
);
CREATE INDEX convention_rules_stack ON convention_rules(stack_id);

CREATE TABLE skills (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  use_when TEXT NOT NULL,
  body TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1,
  content_hash TEXT NOT NULL,
  disabled BOOLEAN NOT NULL DEFAULT false,
  updated_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE convention_revisions (
  kind TEXT NOT NULL CHECK (kind IN ('rule', 'stack', 'skill')),
  item_id TEXT NOT NULL,
  version INTEGER NOT NULL,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  created_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (kind, item_id, version)
);

CREATE TABLE project_stacks (
  project_id TEXT NOT NULL REFERENCES projects(id),
  stack_id TEXT NOT NULL REFERENCES stacks(id),
  source TEXT NOT NULL CHECK (source IN ('detected', 'manual')),
  added_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, stack_id)
);
CREATE INDEX project_stacks_stack ON project_stacks(stack_id);

CREATE TABLE project_skills (
  project_id TEXT NOT NULL REFERENCES projects(id),
  skill_id TEXT NOT NULL REFERENCES skills(id),
  added_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, skill_id)
);
CREATE INDEX project_skills_skill ON project_skills(skill_id);

CREATE TABLE card_skills (
  project_id TEXT NOT NULL REFERENCES projects(id),
  card_id TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
  skill_id TEXT NOT NULL REFERENCES skills(id),
  PRIMARY KEY (project_id, card_id, skill_id)
);
CREATE INDEX card_skills_skill ON card_skills(skill_id);

-- +goose StatementBegin
INSERT INTO stacks (id, name, detect_json, content_hash, created_at, updated_at) VALUES
  ('stk_go', 'Go', '{"files":["go.mod"],"npmDeps":[],"pyDeps":[]}', '', '2026-10-10T00:00:00Z', '2026-10-10T00:00:00Z'),
  ('stk_typescript', 'TypeScript', '{"files":["tsconfig.json"],"npmDeps":["typescript"],"pyDeps":[]}', '', '2026-10-10T00:00:00Z', '2026-10-10T00:00:00Z'),
  ('stk_sveltekit', 'SvelteKit', '{"files":["svelte.config.js","svelte.config.ts"],"npmDeps":["@sveltejs/kit"],"pyDeps":[]}', '', '2026-10-10T00:00:00Z', '2026-10-10T00:00:00Z'),
  ('stk_react', 'React', '{"files":[],"npmDeps":["react"],"pyDeps":[]}', '', '2026-10-10T00:00:00Z', '2026-10-10T00:00:00Z'),
  ('stk_python', 'Python', '{"files":["pyproject.toml","requirements.txt","setup.py","Pipfile"],"npmDeps":[],"pyDeps":[]}', '', '2026-10-10T00:00:00Z', '2026-10-10T00:00:00Z'),
  ('stk_django', 'Django', '{"files":["manage.py"],"npmDeps":[],"pyDeps":["django"]}', '', '2026-10-10T00:00:00Z', '2026-10-10T00:00:00Z');
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO convention_rules (id, scope, stack_id, category, title, body, content_hash, created_at, updated_at) VALUES
  ('rule_naming', 'global', NULL, 'naming', 'Naming', $body$- Names say what a thing is or does. Avoid abbreviations except common ones such as id, url, and http.
- Follow the language's own case style: Go MixedCaps, TypeScript camelCase for values and PascalCase for types and components, Python snake_case for functions and variables.
- Booleans read as a yes or no question: isActive, hasToken, canApprove.
- Limits and configuration values are named constants, never bare numbers in the code.
- Files are named after their main type, component, or route, in the casing the framework expects.
- Do not put the type in the name, as in userList or strName.$body$, '', '2026-10-10T00:00:00Z', '2026-10-10T00:00:00Z'),
  ('rule_database', 'global', NULL, 'database', 'Database tables and columns', $body$- Tables are plural snake_case nouns: users, project_links, card_skills.
- Columns are snake_case. A foreign key is the singular table name plus _id, for example project_id or user_id.
- Every table has a primary key. Unless the project already uses another scheme, ids are opaque text with a type prefix, for example user_0123456789ab.
- Timestamps are created_at and updated_at in UTC. Boolean columns are plain adjectives without an is_ prefix: disabled, archived.
- A row that belongs to a tenant or project carries that id, and every query filters by it.
- Every schema change is a new numbered migration with an up and a down step. Never edit a migration that has already run.
- Queries use parameters. Never build SQL from user text.
- Index every foreign key and every column a list filters or sorts on.$body$, '', '2026-10-10T00:00:00Z', '2026-10-10T00:00:00Z');
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO skills (id, slug, title, use_when, body, content_hash, created_at, updated_at) VALUES
  ('skl_oauth2', 'oauth2-integration', 'OAuth 2.0 integration',
   'A requirement adds sign-in or account linking with an outside identity provider, such as Google, GitHub, Microsoft, Okta, or any OAuth 2.0 or OpenID Connect provider.',
   $body$Steps
1. Use the authorization code flow with PKCE. Never the implicit flow.
2. Read the client id, client secret, redirect URL, and issuer or endpoints from configuration or the secret store. Never commit them.
3. Create a random state value and a PKCE verifier for each sign-in. Keep them on the server or in an HttpOnly cookie, and check the state on the callback.
4. Exchange the code on the server. For OpenID Connect, verify the ID token signature, issuer, audience, expiry, and nonce.
5. Match the provider account to a local user by the provider's stable subject id, not by email alone.
6. Store refresh tokens encrypted, or not at all. Never send provider tokens to the browser.
7. Start a new local session after sign-in, with a new session id.

Configuration
- Client id, client secret, redirect URL, and issuer, named in the project's configuration style.

Security
- The redirect URL is registered with the provider as an exact match.
- After sign-in, redirect only to a relative path inside the app.
- Request the smallest scopes the feature needs.
- Log the outcome of a sign-in, never the code, a token, or the secret.

Test checklist
- A callback with a missing or wrong state is rejected.
- A reused authorization code is rejected.
- An expired or wrongly signed ID token is rejected.
- A first sign-in creates or links exactly one local user, and a second sign-in reuses it.
- Sign-out ends the local session.$body$, '', '2026-10-10T00:00:00Z', '2026-10-10T00:00:00Z');
-- +goose StatementEnd

UPDATE stacks SET content_hash = encode(sha256(convert_to(name || chr(30) || detect_json, 'UTF8')), 'hex');
UPDATE convention_rules SET content_hash = encode(sha256(convert_to(
  scope || chr(30) || COALESCE(stack_id, '') || chr(30) || category || chr(30) || title || chr(30) || body, 'UTF8')), 'hex');
UPDATE skills SET content_hash = encode(sha256(convert_to(
  slug || chr(30) || title || chr(30) || use_when || chr(30) || body, 'UTF8')), 'hex');

INSERT INTO convention_revisions (kind, item_id, version, title, body, content_hash, created_by, created_at)
  SELECT 'stack', id, version, name, detect_json, content_hash, NULL, created_at FROM stacks;
INSERT INTO convention_revisions (kind, item_id, version, title, body, content_hash, created_by, created_at)
  SELECT 'rule', id, version, title, body, content_hash, NULL, created_at FROM convention_rules;
INSERT INTO convention_revisions (kind, item_id, version, title, body, content_hash, created_by, created_at)
  SELECT 'skill', id, version, slug, 'use when: ' || use_when || chr(10) || chr(10) || body, content_hash, NULL, created_at FROM skills;

-- +goose Down
DROP TABLE card_skills;
DROP TABLE project_skills;
DROP TABLE project_stacks;
DROP TABLE convention_revisions;
DROP TABLE skills;
DROP TABLE convention_rules;
DROP TABLE stacks;
