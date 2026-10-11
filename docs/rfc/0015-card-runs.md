# RFC-0015: Run one ready card

Status: Accepted for implementation. Date: 2026-10-11.

This RFC adds decision `RFC-061`. It does not supersede an earlier statement. The worker still owns which job runs and when (tech/0003). This file says how a person asks for one card, and how the board shows that the request is waiting.

Sprint 24 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-061 | A run request names one ready card, and the board shows it as queued | None |

## RFC-061 A run request names one ready card, and the board shows it as queued

A `runs` row may name one card in `card_id`. A card has at most one open row, which is a row with `processed_at` null. A partial unique index enforces it, and a second request for the same card adds nothing.

A card offers the `run` action when its status is `ready`, every card in `dependsOn` is `done`, it has no open row, and the caller's role is Lead Developer, Senior Developer, Developer, or QA. `POST /api/projects/{id}/cards/{card}/run` adds the row and returns the card. A card that is not ready, or whose dependencies are not done, is `409`. Neither the request nor the queued state changes the card's status, so `machine.Transition` is not called.

The board's top button queues every ready card of the project that offers `run`, and is labelled with how many there are. `changes_requested` and `approved` cards are not queued by it. Their own gates already add rows (tech/0003).

Each card carries `queued` and `waitingOn`, the dependencies that are not done yet. The board shows Run, Queued, or Waiting on N cards in the card's footer. It reads the `run` action from `actions` and decides nothing itself.

The board had one Run button that added a project row and gave no sign of which cards it would start. A person who wanted one card started had no way to say so.

The worker loop in tech/0003 is not built yet. Sprint 8 delivered the lock, cap, and dependency helpers, but `serve` starts no per-project loop, and no code sets `processed_at`. Until the loop exists, a queued card stays queued. The board says "Queued" and never "Running".

Set aside: Run on `blocked`, `changes_requested`, or `backlog` cards. Unblock and Request changes already move those cards. Also set aside: cancelling a queued request, and building the worker loop here.

Consequences: `POST /api/projects/{id}/runs` returns `{queued, readyCount}` instead of `{runId, canRun, reason}`. The board response adds `readyCount`, and `canRun` is true only when `readyCount` is above zero and the caller's role may run.

Requirements: REQ-067.
