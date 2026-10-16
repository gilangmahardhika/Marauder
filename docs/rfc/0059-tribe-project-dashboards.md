# RFC-0059: Tribe and project dashboards

Status: Accepted for implementation. Date: 2026-10-16.

This RFC adds decision `RFC-127` under the new REQ-100. It supersedes no statement. Token usage pages (RFC-089 to RFC-091) stay as they are; a project usage route stays set aside (RFC-090).

Sprint 99 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-127 | A project member reads a project summary of board WIP and range insights; a company admin or tribe admin reads a tribe summary that rolls up every project of that tribe | none |

## RFC-127 Tribe and project dashboards

### Problem

The board lists every card of one project. Token usage totals model calls. Neither gives a tribe admin or a member a short view of where work is stuck: Needs answer, blocked reasons, rework, budget stops, or how the tribe's projects compare. Fetching every board to count statuses does not scale, and there is no tribe-scoped card list.

### Decision

- `GET /api/projects/{projectId}/summary?from=&to=` returns a snapshot of that project's cards by board group, attention counts (ready to run, waiting answer, human review, blocked by reason, open runs, locks, implementation cap), the oldest open cards, and range totals from events and answers in `[from, to]`. A project member may call it (`SeeProject`); anyone else gets `404`, including a company admin who is not a member. The range follows the RFC-089 day rules (default last 30 UTC days, max 366).
- `GET /api/tribes/{id}/summary?from=&to=` returns the same snapshot fields rolled up across the tribe's projects, a per-project list, and MCP health counts for that tribe in the range. A company admin or a tribe admin of `{id}` may call it (`ManageTribe`); anyone else gets `403`, and a missing tribe is `404`, both before the dates.
- Neither response returns reply text, prompts, secrets, handoff bodies, or full card lists. Aging rows carry `cardId`, `title`, `status`, and `updatedAt` only.
- The UI adds `/overview` under the Project sidebar group for the selected project, and `/tribes/{id}/overview` for tribe admins (and company admins), linked from the tribe members page and the Tribe sidebar. Token spend links to the existing usage pages; there is no `GET /api/projects/{id}/usage`.

### Why

Summary SQL over `cards`, `runs`, `locks`, `events`, and `card_questions` answers the ops questions without new instrumentation. Membership for the project route matches the board. ManageTribe for the tribe route matches tribe usage and MCP admin routes. Keeping usage on its own pages avoids undoing RFC-090's set-aside of a project usage page.

### Set aside

- Dollar cost of tokens (RFC-089).
- Who pressed Run (RFC-089).
- Convention override debt and linked-project heat (later).
- Stage cycle-time columns; the range block uses transition and answer timestamps only.

### Tests

- `TestProjectSummaryMember`: a member gets group counts that match seeded cards, and `readyCount` matches the board.
- `TestProjectSummaryOutsider404`: a non-member and an unknown project are `404`.
- `TestTribeSummaryManageOnly`: a tribe admin gets the rollup; a plain member is `403`; a missing tribe is `404`.
- `TestSummaryRange`: default range is 30 days; a bad `from` is `422`; range totals count `done` transitions and `budget_stop` events in the window.
- Overview page and tribe overview Vitest cases; layout links Overview in Project and Tribe groups.
