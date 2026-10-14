# Tech doc: implementation stack

Status: Accepted for implementation. Date: 2026-10-10. Revised after review.

This document chooses the stack and the open source tools, and sets the process boundary. Requirements are in [../prd/coding-orchestrator.md](../prd/coding-orchestrator.md). Decisions are in `../rfc/`. Document precedence is in [../README.md](../README.md). The contract, the build steps, and the security rules are in the guides below. This file does not repeat them.

The backend is a Go process that speaks JSON. Node.js is not the server, the worker, or the deploy path. The frontend is a separate SvelteKit app. It does not use React, it has no server routes, and it does not store the board.

| Guide | What it locks |
| --- | --- |
| [0002 Data and API](0002-data-and-api.md) | PostgreSQL schema, error shape, every JSON route, settings fields, files on disk |
| [0003 Backend](0003-backend.md) | Packages, startup, worker, jobs, router, providers, git, CLI |
| [0004 Frontend](0004-frontend.md) | SvelteKit client, work pages, settings pages, CSP, client tests |
| [0005 Verification](0005-verification.md) | Fixture repo, automated checks, REQ-043 runner, release order |
| [0006 Security and harness](0006-security-and-harness.md) | Data directory, path rule, checks, sandbox, probe, secrets, sessions, git, assertions |

## Stack

| Layer | Choice |
| --- | --- |
| Backend language | Go 1.26 |
| Backend process | One `orchestrator` binary: CLI, JSON API, worker |
| HTTP | Standard library `net/http` |
| CLI | Cobra, calling the same service functions as the API |
| Board store | PostgreSQL 16 or newer through `database/sql` and `github.com/jackc/pgx/v5/stdlib` |
| Migrations | goose, dialect `postgres` |
| PRD and RFC parse | goldmark, then a small heading walker |
| Config on disk | `yaml.v3` for settings, a mode `0600` JSON file for tokens, both in the data directory |
| Models | Official Anthropic and OpenAI Go SDKs, behind an internal provider interface |
| Git | The `git` binary, resolved once at startup |
| GitHub | The `gh` binary, resolved once at startup |
| Check sandbox | Podman, or Docker when Podman is absent. The fixture suite needs neither |
| Frontend | SvelteKit 2 with Svelte 5, client render only, `adapter-static` |
| Frontend tooling | Vite on `127.0.0.1:4173`, `svelte-check`, Vitest with `@testing-library/svelte` |
| Backend tests | Go `testing` and `net/http/httptest` |

Go 1.26 is the target because the current official OpenAI Go SDK (module `github.com/openai/openai-go/v3`) requires it. The official Anthropic SDK (`github.com/anthropics/anthropic-sdk-go`) requires Go 1.24 or newer.

The harness stays in Go because that process owns the PostgreSQL board, git worktrees, built child environments, and the deploy command. The browser is a client of the JSON API. A Python API could replace the Go server later and keep this frontend. It is a fallback, not a second runtime beside Go.

## Processes

| Process | Address | Owns |
| --- | --- | --- |
| `orchestrator serve` | `127.0.0.1:8787` | JSON API, the worker lock, PostgreSQL, secrets, git, `gh`, sandbox, deploy |
| `orchestrator <command>` | None | The same `internal/app` functions. `run-next` executes jobs only when `serve` is not running (RFC-043) |
| `orchestrator runner run` | None; calls `serve` over HTTPS | Leased implement jobs on its own machine: clone, worktrees, the loop, the gating check in Podman. No database, no model key, no GitHub write token (RFC-106 to RFC-108) |
| SvelteKit dev or preview | `127.0.0.1:4173` | Static pages that call the API with `fetch` |

Rules that hold across processes:

- One process holds the worker at a time (RFC-043). Human gates from either client are safe through compare-and-swap transitions (RFC-040).
- Settings, secrets, worktrees, exports, and logs live in the data directory, outside the target repo (RFC-036). The board is PostgreSQL, named by `databaseUrl` in that directory (RFC-044).
- `internal/app` is the only caller of `machine.Transition`. The frontend never writes the board, chooses a status, or keeps the board as a source of truth.
- The API binds loopback only, checks `Host`, and allows the exact UI origin.
- Child environments are built, never inherited (0006).

## Repository layout

```text
backend/
  cmd/orchestrator/main.go
  cmd/fixture/main.go
  internal/machine/
  internal/trace/
  internal/board/
  internal/intake/
  internal/settings/
  internal/router/
  internal/contextpack/
  internal/providers/
  internal/execx/
  internal/sandbox/
  internal/runtime/
  internal/vcs/
  internal/deploy/
  internal/verify/
  internal/worker/
  internal/runner/
  internal/app/
  internal/cli/
  internal/httpapi/
  migrations/00001_init.sql
  testdata/
scripts/
  install-runner.sh
  release.sh
deploy/systemd/
  orchestrator-runner.service
frontend/
  package.json
  package-lock.json
  svelte.config.js
  vite.config.ts
  src/routes/
  src/lib/
```

The full frontend tree is in 0004. `backend/internal/web` does not exist. Go templates are not the UI.

## Open source tools

Pin versions in `backend/go.mod` and `frontend/package-lock.json` when each project is created.

| Tool | Use here | Required in v1 | Kept out of |
| --- | --- | --- | --- |
| [Go](https://go.dev/) | API, CLI, worker | Yes | The frontend |
| [PostgreSQL](https://www.postgresql.org/) | Board, attempts, events. Version 16 or newer | Yes | The target repo |
| [pgx](https://github.com/jackc/pgx) | Go driver, `github.com/jackc/pgx/v5/stdlib` | Yes | |
| [goose](https://github.com/pressly/goose) | SQL migrations, postgres dialect | Yes | |
| [Cobra](https://github.com/spf13/cobra) | CLI verbs from REQ-042 | Yes | Card status |
| [goldmark](https://github.com/yuin/goldmark) | Headings, lists, existing ids | Yes | Card creation before accept |
| [yaml.v3](https://github.com/go-yaml/yaml) | `config.yaml` | Yes | Tokens |
| [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) | Anthropic-protocol calls | When that family is configured | Routing policy |
| [openai-go](https://github.com/openai/openai-go) | OpenAI-protocol calls, including local compatible servers | When that family is configured | Routing policy |
| [Git](https://git-scm.com/) | Worktrees, commits, archive, push | Yes | Model-invented shell |
| [GitHub CLI](https://github.com/cli/cli) | Pull requests and CI status, binary `gh` | Yes | Deploy |
| [SvelteKit](https://github.com/sveltejs/kit) | Client routes, `adapter-static` build | Yes, in `frontend/` | Server routes, form actions, the harness |
| [Vite](https://vite.dev/) | Frontend dev server and build | Yes, in `frontend/` | |
| [Vitest](https://vitest.dev/) and [Testing Library](https://testing-library.com/docs/svelte-testing-library/intro) | Component tests for action buttons and secret fields | Yes, in `frontend/` | |
| [Podman](https://github.com/containers/podman) | Check sandbox | For a real repo target | Fixture state-machine tests |
| [Docker Engine / Moby](https://github.com/moby/moby) | Same sandbox role | Fallback only | |

Licenses: Go is BSD-3-Clause. pgx, goose, goldmark, the GitHub CLI, `anthropic-sdk-go`, Svelte, SvelteKit, Vite, Vitest, and Testing Library are MIT. Cobra, `openai-go`, and Podman are Apache-2.0. PostgreSQL is the PostgreSQL License. Git is GPL-2.0 and runs as a separate program, not linked into the binary.

## Not in this stack

| Tool | Why it is absent |
| --- | --- |
| Node.js as the harness | The API, worker, and deploy stay in Go. Node runs the frontend toolchain only |
| React | The client is SvelteKit. Card state stays on the server |
| SvelteKit server routes, form actions, `hooks.server.ts` | A second backend beside the Go API |
| LiteLLM | Routing stays in `internal/router` |
| LangChain, langchaingo | Tool allowlists are snapshot-tested in our own loop |
| Temporal, Redis, NATS, Kafka | One Postgres database. One worker and one deploy lock per project (RFC-048). Remote runners claim leases with `SKIP LOCKED` in the same database (RFC-107) |
| HTMX, Go templates | They tie the HTML to the API process |
| A cloud provider SDK | Deploy is an operator script (RFC-038) |

## Build and run

Machine prerequisites:

```text
go 1.26
git
gh
node               # frontend toolchain only
podman or docker   # real repo checks only
```

```text
# backend
cd backend
go run ./cmd/orchestrator serve --repo /path/to/target-repo
# loopback by default. ORCHESTRATOR_CLOUD=1 is the only off-loopback bind

# frontend, separate terminal
cd frontend
npm ci
npm run dev
```

Open `http://127.0.0.1:4173/` and sign in with email and password (REQ-046). Fill account GitHub. As an admin, fill Company GitHub, LLM APIs, and Router. Then, as a Lead Developer member, fill the project's Repository, Linked projects, and Deploy. A target repo may use Node for its own `localCheck`. That command runs in the sandbox and is not the harness.

## Implementation order

1. `machine`, `trace`, `board`, the migration, startup checks.
2. `intake` with goldmark, the accept gate.
3. `execx`, `sandbox`, and `runtime` with `git worktree` on the fixture repo.
4. The fake provider and the implementer pack.
5. `vcs` through `git` and `gh`.
6. Reviewer pack and the second provider family.
7. Human gates bound to a SHA, with the CLI.
8. Deploy stub, health, verify.
9. Settings files and snapshots.
10. Worker: lock, passes, plan job, dependencies, deploy lock.
11. REQ-043 fixture.
12. JSON API for every route in 0002.
13. SvelteKit app: layout and session, work pages, then the admin settings (Company GitHub, LLM APIs, Router) and the project settings (Repository, Linked projects, Deploy).
14. Accounts, tribes, and per-project workers (Sprints 11–13, RFC-045 through RFC-048).

Each step's tests are in 0005. Do not start a step until the previous step's tests pass.
