# Backend

Go module `orchestrator/backend`. This tree is the harness: the PostgreSQL board, compare-and-swap transitions, the insert-only event log, and process startup.

Sprint 1 is the board and the gate. `serve` migrates and returns. It does not listen, and it does not run a worker.

## Requirements

- Go 1.26
- PostgreSQL 16 or newer

The tests do not start a database server.

## Environment

Copy the two variables into `backend/.env`. That file is gitignored. [`github.com/joho/godotenv`](https://github.com/joho/godotenv) loads the nearest `.env`, walking up from the working directory to the repository root. A variable already set in the shell is left as-is.

```text
ORCHESTRATOR_TEST_DATABASE_URL=postgres://USER:PASSWORD@127.0.0.1:5432/orchestrator_test?sslmode=disable
ORCHESTRATOR_DATABASE_URL=postgres://USER:PASSWORD@127.0.0.1:5432/orchestrator_test?sslmode=disable
```

`ORCHESTRATOR_TEST_DATABASE_URL` is required by every test that writes a card. A missing value fails that test with this name before any insert.

`ORCHESTRATOR_DATABASE_URL` is copied into `<data>/secrets.json` as `databaseUrl` only when that file does not exist yet. After the file exists, the file wins. The API does not return this URL.

## Tests

From this directory:

```text
go test ./...
```

## Serve

`--data` must be outside `--repo`. `--addr` must be `127.0.0.1` or `::1`. The default address is `127.0.0.1:8787`. The default data directory is `~/.orchestrator/<repo-name>-<hash8>`.

```text
go run ./cmd/orchestrator serve --repo /path/to/target-repo --data /path/outside/the/repo
```
