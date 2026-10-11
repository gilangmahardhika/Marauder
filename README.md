# AI Coding Orchestrator

A Go harness keeps a PostgreSQL board of coding cards. A SvelteKit client talks to that API. The browser shows the actions the API returns. It does not change card status on its own.

People sign in with email and password. An admin creates users. A user has one job role and may belong to several tribes. A tribe's projects are hidden from everyone else. Each project has its own repo, settings, and worker, and two projects may run at the same time. The worker turns each accepted requirement into a card and moves cards to Ready without a model (RFC-062). When someone presses Run on a card, the worker runs the implement job with the routed model, runs the project's local check in the sandbox image, and opens the pull request (RFC-063).

Specification lives in [docs/README.md](docs/README.md).

## Requirements

- Go 1.26
- PostgreSQL 16 or newer
- Git
- Node.js and npm, for the client only

`gh`, Podman, or Docker are needed when a real target repo opens pull requests or runs container checks. The local UI and `go test` do not need them.

## 1. Create the databases

```sql
CREATE DATABASE orchestrator;
CREATE DATABASE orchestrator_test;
```

`orchestrator` is the board `serve` uses. Every project shares that database. `orchestrator_test` is for `go test` and `go run ./cmd/fixture`. Tests create a private schema in that database and do not start PostgreSQL for you.

## 2. Point the harness at PostgreSQL

From `backend/`, create `backend/.env`. That file is gitignored. The process loads the nearest `.env` and does not override variables already set in the shell.

```text
ORCHESTRATOR_DATABASE_URL=postgres://USER:PASSWORD@127.0.0.1:5432/orchestrator?sslmode=disable
ORCHESTRATOR_TEST_DATABASE_URL=postgres://USER:PASSWORD@127.0.0.1:5432/orchestrator_test?sslmode=disable
```

On the first start, when `<data>/secrets.json` does not exist yet, `serve` copies `ORCHESTRATOR_DATABASE_URL` into that file as `databaseUrl`. After the file exists, the file wins. The API never returns this URL.

## 3. Run the API and the client

From the repository root, after `backend/.env` exists:

```text
./run.sh
```

That starts `serve` on loopback for this repository and the client at [http://127.0.0.1:4173/](http://127.0.0.1:4173/). The first run installs frontend dependencies when `frontend/node_modules` is missing. Ctrl-C stops both processes. `./run.sh` does not set `ORCHESTRATOR_CLOUD`.

Pass another target repo when this repository is not the one you want on the board:

```text
./run.sh /path/to/target-repo
```

`ORCHESTRATOR_DATA`, when set, is the service data directory. The script creates it with mode `0700`. Leave it unset on a current build to use `~/.orchestrator/<repo-name>-<hash8>`. After Sprint 13, the default service root is `~/.orchestrator/service`, and each project lives under `projects/<project-id>/`. `ORCHESTRATOR_ADDR` changes the listen address. Without `ORCHESTRATOR_CLOUD=1` it still has to be loopback.

Cloud mode is explicit. Set `ORCHESTRATOR_CLOUD=1` only when TLS terminates in front of the process. That is the only way to listen off loopback.

Sign in with email and password after Sprint 11. Until that sprint, paste the `UI token:` line from the API log into the login page. Five wrong attempts pause new logins for one minute.

Then fill settings in this order: your GitHub login and token, then the project's GitHub repo, LLM APIs, Router, and Deploy. A saved secret is not shown again. Leave the field empty to keep the stored value. The first admin is created with `orchestrator users bootstrap` once that command exists.

## 4. Start the API by itself

The service data directory holds `secrets.json` (`databaseUrl` and `tokenKey`). Each project directory holds that project's `config.yaml`, project secrets, worker lock, and worktrees. A project directory must sit outside that project's repo. If you omit `--data` on a current build, the default is `~/.orchestrator/<repo-name>-<hash8>`. If you pass `--data`, that directory must be mode `0700`.

`--addr` must be loopback unless `ORCHESTRATOR_CLOUD=1`. The default is `127.0.0.1:8787`.

```text
cd backend
go run ./cmd/orchestrator serve --repo /path/to/target-repo
```

The process keeps listening and runs one worker loop per project. A second `serve` on the same data directory exits while the first holds its lock. Log lines such as `worker: project proj_... created 1 promoted 1` show the worker making cards after an accept, and `worker: card card_... -> pr_open` shows a queued card finishing its implement job. `orchestrator run-next --project ID` runs the same passes, and the jobs of queued cards, once when no server holds that project's lock.

Before a card can run on a real repository, set in the project's Deploy settings a local check (for example `npm test`), its first word in the allowed executables, the sandbox engine (`podman` or `docker`), and an image pinned as `name@sha256:<64 hex>` that already holds the toolchain and dependencies. The check runs with no network, so it cannot install packages. Pull the image once with the engine before the first run.

## 5. Start the client by itself

In another terminal, with the API already listening:

```text
cd frontend
npm ci
npm run dev
```

Open [http://127.0.0.1:4173/](http://127.0.0.1:4173/) and sign in. Until Sprint 11, paste the UI token from the API log.

A static build still calls the Go API:

```text
npm run build
npm run preview
```

`preview` stays on `127.0.0.1:4173`.

## Checks

From `backend/`:

```text
go test ./...
go run ./cmd/fixture
```

`go run ./cmd/fixture` needs `ORCHESTRATOR_TEST_DATABASE_URL` and a clean `git status` in `backend/testdata/fixture-repo`.

From `frontend/`:

```text
npm run check
npm run test
npm run build
```
