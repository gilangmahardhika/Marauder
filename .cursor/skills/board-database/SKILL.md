---
name: board-database
description: PostgreSQL schema, goose migrations, and board transactions for the orchestrator. Use when editing migrations, SQL, internal/board, databaseUrl, or card, attempt, event, and lock rows.
---

# Board database

The board is PostgreSQL 16 or newer (RFC-044). Connection string: `databaseUrl` in `<data>/secrets.json`. The API never returns it. Driver: `database/sql` with `github.com/jackc/pgx/v5/stdlib`. Pool size at most 4. Goose dialect `postgres`.

There is no `orchestrator.db` and no SQLite.

## Schema

Migrations live in `backend/migrations/`. The first file is `00001_init.sql`, specified in `docs/tech/0002-data-and-api.md`.

- Ids are `TEXT`: `card_`, `att_`, `evt_`, `run_`, `user_`, `ses_`, `tribe_`, `proj_` plus 12 hex chars. Requirement and decision ids are copied from markdown and are unique per project. Do not renumber them.
- Identity tables are `users`, `sessions`, `user_github`, `tribes`, `tribe_members`, and `projects`, specified in `docs/tech/0002-data-and-api.md`. Board rows include `project_id`.
- Times are UTC RFC3339 strings.
- Flags are `BOOLEAN`: `manual`, `review_independent`, `ci_required`, `gates.passed`. Counts and token totals stay `INTEGER`.
- JSON payloads stay `TEXT` (`acceptance_json`, `payload_json`, and the other `*_json` columns).
- `events` is insert-only. The PL/pgSQL trigger `events_immutable` aborts `UPDATE` and `DELETE`.
- Placeholders are `$1`, `$2`, and so on.

## Writes

One transaction wraps a compare-and-swap, its gate insert, and its `transition` event:

```sql
UPDATE cards
SET status = $1, updated_at = $2
WHERE id = $3 AND status = $4 AND project_id = $5;
```

`RowsAffected` of 0 is `board.ErrConflict`. Do not continue the side effect.

Card locks use `INSERT ... ON CONFLICT (card_id) DO NOTHING` and delete only `WHERE card_id = $1 AND holder = $2`. The deploy lock is one row per project, not one row for the whole database.

## Tests

`ORCHESTRATOR_TEST_DATABASE_URL` is required. Each test migrates that database and isolates its rows. Do not start testcontainers. Do not embed Postgres.
