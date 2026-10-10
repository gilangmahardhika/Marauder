# RFC-0004: PostgreSQL board

Status: Accepted for v1. Date: 2026-10-10.

This RFC adds `RFC-044`. It supersedes the store engine in RFC-002 and RFC-022, and the words "the SQLite board" in RFC-036. Those statements stay in their files with a pointer here.

Wording fix made in place, which does not change RFC-040: the compare-and-swap example uses PostgreSQL `$1` placeholders.

## RFC-044 The board is a PostgreSQL database

PostgreSQL 16 or newer stores cards, attempts, gates, approvals, and events. The connection string is `databaseUrl` in `secrets.json` inside the data directory. The API does not return that URL. GitHub still stores the pull request and is not the board.

A database file in the data directory forces a single connection and ties the board to one disk. PostgreSQL keeps compare-and-swap transitions and the insert-only event log, and the process uses a small pool. The connection string stays with the other secrets, so a pull request cannot point the harness at another database.

Set aside: staying on SQLite. Also set aside: putting `databaseUrl` in the target repo or in `config.yaml`.

Consequences: each data directory names its own database, so two target repos do not share a board. `orchestrator status` reads PostgreSQL. Losing that database loses the board even if the pull requests remain. Migrations use goose with the postgres dialect. The Go driver is `github.com/jackc/pgx/v5/stdlib` behind `database/sql`. `databaseUrl` joins the RFC-030 secret list and is redacted from packs and events. Tests use `ORCHESTRATOR_TEST_DATABASE_URL` and do not embed a database engine.

Superseded in part by RFC-047 in [0005-teams-and-cloud.md](0005-teams-and-cloud.md). One service `databaseUrl` stores every project. `databaseUrl` is still absent from the API. The driver, the dialect, and the test URL stay.

Requirements: REQ-005, REQ-039, REQ-042, REQ-045.
