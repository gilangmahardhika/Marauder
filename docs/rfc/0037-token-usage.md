# RFC-0037: Token usage across projects

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-089` under the new REQ-084. It supersedes no earlier statement.

Sprint 53 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-089 | An admin-only Token usage page and `GET /api/usage` total the tokens of stored `model_call` and `requirement_refined` events across every project, by day, LLM family, model, project, user, and task type. The page filters, groups, and sorts them | None |

## RFC-089 Token usage across projects

### Problem

Token counts are stored per model call, but nobody can see them across cards. The attempt row counts only the implementer, so reviewer and refine calls are invisible outside each card's Trace page. An admin who pays for the models cannot tell which project, model, or person uses them.

### Decision

- The source is the event log. Every `model_call` (implementer and reviewer) and every `requirement_refined` event carries `inputTokens`, `outputTokens`, and `cachedInputTokens`, and the row carries the project. The attempt columns are not used for totals, because they miss the reviewer and refine.
- Task type: `model_call` events now store `taskClass` (`implement`, `implement_hard`, or `review`). An older event with none reads `review` for role `reviewer`, and otherwise its attempt's `task_class`. A `requirement_refined` event is `refine_requirement`.
- User: a call on a card counts against the card's owner, and a refine against the member who ran it (`actor_id`). A card with no owner counts under "No owner". Nothing records who pressed Run, so the owner is the attribution that also covers past events.
- LLM: the event stores the model id. The family comes from the current `config.yaml`, which lists each model id in one family. A model no longer in the config reads "Unknown family".
- `GET /api/usage?from=YYYY-MM-DD&to=YYYY-MM-DD` returns one row per day, project, model, task type, and user, with calls and token sums, plus the family and names. `to` is inclusive. The default is the last 30 days, and a range over 366 days is `422`. Only an admin may call it; anyone else gets `403`. It returns no reply text and no secret.
- `orchestrator usage --from --to --by llm|model|project|user|task` prints the same totals grouped one way, largest first.
- The page `/admin/usage` loads 7, 30, or 90 days. It filters by LLM, model, project, user, and task type, groups by one of them, and sorts the groups by name, calls, input, output, cached, or total. It shows the totals, a bar per day, and each group's share. The filters, grouping, and sort live in the URL. Total tokens are input plus output; cached input is shown on its own.
- An index on `events (kind, ts)` keeps the read off a full scan. The events table stays insert-only.

### Why

The event log already has every counted call with its project, and adding `taskClass` to new events removes the one join that old events need. Grouping on the server by day keeps the reply small, and filtering and sorting in the browser follow the other admin pages.

### Set aside

- Cost in money. Prices differ per model and change, and no price list is kept.
- Tokens of a call that failed before a reply, and of a retry the router replaced. They are never stored.
- The LLM settings test button, which stores no event.
- Who pressed Run or Retry. A later run actor column could add it.
