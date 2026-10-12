# RFC-0038: Token usage for a tribe and for each member

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-090` under the new REQ-085. It supersedes one statement of `RFC-089`: the project a call counts in.

Sprint 54 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-090 | The Token usage page also opens for one tribe, to a company admin or that tribe's admins, and for the signed-in member, over their own calls. A call on a card counts in the card's project | RFC-089 in part: "the row carries the project" as the project a call counts in |

## RFC-090 Token usage for a tribe and for each member

### Problem

RFC-089 shows token usage to company admins only. A tribe admin answers for the projects of their tribe but cannot see what those projects use, and a member cannot see what their own cards and refines use.

### Decision

- `GET /api/tribes/{id}/usage?from=&to=` returns the RFC-089 rows for calls in projects of that tribe. A company admin or a tribe admin of that tribe may call it, as with the other `/api/tribes/{id}/` routes. Anyone else gets `403`, and a missing tribe is `404`.
- `GET /api/me/usage?from=&to=` returns the RFC-089 rows of the caller's own calls: calls on cards they own and refines they ran, in any project, including a tribe they have left. Any signed-in user may call it.
- Both take the RFC-089 range rules and reply with the same shape. Neither returns reply text or a secret.
- A call on a card counts in the card's project. An event row's own project is used only when the call has no card, as for a refine. Some older card events carry `proj_default`, and a tribe view would otherwise miss them or show them in the wrong tribe.
- The page `/tribes/{id}/usage` shows one tribe with the five dimensions, and `/account/usage` shows "My token usage" with LLM, model, project, and task, since the user is always the caller. Both have the RFC-089 range, filters, Group by, sort, totals, and bars. The tribe page links from the tribe's member page; "My token usage" links from the account page and the user menu.
- `orchestrator usage` takes `--tribe ID` and `--user ID` to read the same scopes. The operator runs it, so it checks no access.

### Why

The rows, families, and page already exist, so a scope is one more condition on the same query and one more caller of the same component. Counting a card's call in the card's project makes the three scopes agree: a tribe's total plus the other tribes' totals is the company total.

### Set aside

- A tribe admin seeing a member's calls in projects of other tribes. The tribe view is about the tribe's projects.
- Usage for a single project page. A tribe view filtered by project covers it for tribe admins.
