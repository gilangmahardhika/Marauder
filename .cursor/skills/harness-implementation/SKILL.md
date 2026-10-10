---
name: harness-implementation
description: Implements an orchestrator behavior in the right package, with the same CLI and HTTP service and the named tests. Use when adding a card transition, job, route, setting, tool, or sprint task.
---

# Implementation

Read the sprint task in `docs/plan/sprints.md` and the matching section of `docs/tech/0003-backend.md` before writing code.

## Steps

1. Name the requirement (`REQ-`) and the package that owns the behavior. If no package may do it, stop.
2. If the change adds a JSON field, status, or column, update `docs/tech/0002-data-and-api.md` in the same change. `frontend/src/lib/types.ts` changes with the JSON.
3. Put the use case on `internal/app`. CLI and HTTP both call that function. They do not call each other.
4. Status changes go through `machine.Transition` inside one transaction. `RowsAffected` must be 1 or the caller returns `board.ErrConflict` and does no side effect.
5. A model tool may only be one of `read_doc`, `propose_requirements`, `create_card`, `read_file`, `edit_file`, `run_local_check`, `commit`, `read_diff`, `comment_pr`, `verdict`, `read_log`, `write_failure_note`. `run_local_check` and `verdict` do not move the card.
6. Add or extend the test named in `docs/tech/0005-verification.md` or `docs/tech/0006-security-and-harness.md`. A missing `ORCHESTRATOR_TEST_DATABASE_URL` fails before any card write.

## Do not

- Deploy, merge, or push to the default branch from the implementer.
- Grade a check from model prose. Exit code 0 is pass.
- Count a review or triage call toward `attempts_used`.
- Apply a settings save to an attempt already running. The project's snapshot on the attempt stands.
- Treat a job role as a model profile. Model profiles stay the tool lists. Human gates use the session user and RFC-046.
