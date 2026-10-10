---
name: go-conventions
description: Go coding conventions for the orchestrator harness. Use when writing or reviewing backend Go, package boundaries, errors, subprocesses, or HTTP handlers in backend/.
---

# Go conventions

Module path `orchestrator/backend`. Guides: `docs/tech/0003-backend.md` and `docs/tech/0006-security-and-harness.md`. The contract is `docs/tech/0002-data-and-api.md`.

## Packages

Put code in the package whose "Must not" column allows it. In particular:

- `internal/machine` does not call git, models, or HTTP.
- `internal/cli` and `internal/httpapi` do not run SQL. They call `internal/app`.
- `internal/router` does not call a provider. `internal/providers` does not choose a fallback.
- `internal/app` does not encode HTML.

`cmd/orchestrator/main.go` wires the database, settings, and logger. Tests build `app` with fakes for the provider, git, `gh`, the sandbox engine, and the clock.

## Errors

Return the sentinel from the package that owns the failure. `internal/httpapi` maps them. Do not invent a second JSON error shape.

| Sentinel | HTTP |
| --- | --- |
| `machine.ErrIllegal`, `machine.ErrEvidence`, `app.ErrActionNotListed` | 422 `transition_rejected` |
| `settings.ErrValidation` | 422 `validation` |
| `board.ErrNotFound` | 404 `not_found` |
| `board.ErrLocked`, `board.ErrConflict` | 409 `conflict` |

Middleware order: `Host`, CORS and `Origin`, body cap, bearer, handler. A successful body has no `error` field.

## Processes

`execChild` takes an argv slice, a built environment, a timeout, and an output cap. It refuses a basename of `sh` or `bash`. Do not pass a model string to a shell.

```go
// BAD
exec.Command("bash", "-lc", modelText)

// GOOD
execx.Command(ctx, []string{name, arg}, env)
```

Do not log request bodies, `databaseUrl`, tokens, or model keys.
