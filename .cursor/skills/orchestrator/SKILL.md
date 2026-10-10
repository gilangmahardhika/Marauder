---
name: orchestrator
description: Implements the AI Coding Orchestrator from its PRD, RFCs, and tech guides. Use when editing this repo, adding backend or frontend behavior, changing docs/prd, docs/rfc, docs/tech, or docs/plan, or when the user mentions the harness, kanban cards, sprints, the PostgreSQL board, or the SvelteKit client.
---

# Orchestrator

More specific procedures live in `go-conventions`, `harness-implementation`, `board-database`, and `orchestrator-ui` under `.cursor/skills/`.

## Before editing

1. Read `docs/README.md` for which document wins.
2. Read the current sprint in `docs/plan/sprints.md`. Do not start the next sprint while the current sprint's named tests fail.
3. Read the contract section you will change in `docs/tech/0002-data-and-api.md` before writing SQL, JSON, or a route.

Precedence, highest first: PRD acceptance checks, RFC statements (a later RFC wins only where it names the superseded id), `docs/tech/0002-data-and-api.md`, then the other guides (`0006` over `0003`, `0004`, and `0005`, which win over `0001`), then `docs/ai-coding-orchestrator.md`. The sprint plan only schedules work.

## Where work goes

- Backend: module `orchestrator/backend` under `backend/`. Package duties are in `docs/tech/0003-backend.md`. Only `internal/app` calls `machine.Transition`.
- Frontend: `frontend/`, SvelteKit 2 and Svelte 5, client render only, `adapter-static`. Spec is `docs/tech/0004-frontend.md`. No `+server.ts`, no form actions, no React.
- Security controls in `docs/tech/0006-security-and-harness.md` override a looser sentence in another guide.

## Board

PostgreSQL 16 or newer through `github.com/jackc/pgx/v5/stdlib` and goose with the postgres dialect (RFC-044). `databaseUrl` lives in `secrets.json` in the data directory. The API does not return it. Tests and `go run ./cmd/fixture` need `ORCHESTRATOR_TEST_DATABASE_URL`. A missing variable fails before any card is written. Do not add SQLite or testcontainers.

## Changing a decision

Leave an accepted RFC statement in place. Add a new RFC that names the id it supersedes, and update the PRD acceptance check in the same change when behavior changes. Do not leave `docs/tech/0002-data-and-api.md` disagreeing with the PRD.

## Done

Match the sprint's "Done when" list. Backend release order is `go test ./...`, then `go run ./cmd/fixture` exiting 0, then the frontend checks in `docs/tech/0005-verification.md`. A green UI does not replace the fixture.
