# RFC-0016: The worker makes cards without a model

Status: Accepted for implementation. Date: 2026-10-11.

This RFC adds decision `RFC-062`. It supersedes in part the plan job in [tech/0003](../tech/0003-backend.md), which stops with `router_config` when no plan model is configured. Until a model provider exists, the worker plans without one.

Sprint 25 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-062 | `serve` runs a worker per project that makes one card per accepted requirement without a model | The plan job's `router_config` stop in tech/0003, until a provider exists |

## RFC-062 `serve` runs a worker per project that makes one card per accepted requirement without a model

`serve` starts one loop per project, and picks up projects created while it runs. Each loop takes `<data>/projects/<id>/worker.lock`. A project whose lock another process holds is logged and retried on the next tick. Every 2 seconds, while the project has an open `runs` row with no `card_id`, the loop runs passes until one changes nothing.

A pass, with no model call:

1. Promotes each `backlog` card to `ready` through `machine.Transition` when every requirement it names is accepted in its project, every card in `dependsOn` is `done`, and the card is manual or one of its requirements has acceptance lines.
2. Makes one `backlog` card for each accepted requirement that no card of the project names. The title is the requirement heading, or its id when the heading is empty. Risk is `low`. The owner is the requirement's `created_by` (RFC-054). The checks come from the project settings, and `maxAttempts` from the router budget.
3. When it changed nothing, sets `processed_at` on the project's open rows that have no `card_id`. Card-scoped rows (RFC-061) stay open for the implement job.

`orchestrator run-next --project ID` inserts a `cli` row, then runs the same passes when the project's lock is free, and prints `created N` and `promoted M`. When the lock is held it prints `queued for the running server`.

Accept recorded a run request that nothing read, so accepted requirements never became cards and the board stayed empty. No model provider exists yet, and the planner cannot run without one.

Set aside: splitting one requirement into several cards, dependencies between generated cards, and attaching skills. The model planner does those later. Also set aside: implement, review, deploy, and verify jobs. A queued card stays queued.

Consequences: a requirement without acceptance lines makes a card that stays in `backlog`. The board's `running` flag is true while a project row is open, so the board polls until the worker is idle.

Requirements: REQ-068.
