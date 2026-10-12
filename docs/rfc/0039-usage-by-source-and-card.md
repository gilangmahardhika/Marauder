# RFC-0039: Token usage by source and card

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-091` under the new REQ-086. It supersedes no earlier statement. It adds fields to the usage rows of RFC-089 and RFC-090.

Sprint 55 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-091 | Token usage filters and groups by source, Intake or Card, and by card. Each usage row names its source and card | None |

## RFC-091 Token usage by source and card

### Problem

The Token usage pages split calls by model, project, user, and task type, but not by where the work came from. An admin cannot tell how much intake work costs next to card jobs, or which card used the most tokens.

### Decision

- Each usage row has `source`: `intake` for a `requirement_refined` event and `card` for a `model_call` event, the implement and review jobs on a card.
- Each usage row has `cardId` and `cardTitle`, the event's card and its current title. A refine has neither.
- The usage query groups by card as well, so a row is one day, project, model, task type, user, source, and card.
- Every Token usage page, for the company, a tribe, and the member, filters and groups by Source and by Card. A refine shows as "Intake" under Card, and its filter value is `-`. Cards are told apart by id, so two cards with one title stay apart, and the card id shows as a tooltip.
- `orchestrator usage --by` takes `source` and `card`. A card prints as its title with its id in parentheses, and a refine as `Intake`.

### Why

The event already names its card and its kind, so the split needs no new data. Keying cards by id keeps a renamed or repeated title from merging two cards' totals.

### Set aside

- A link from a card group to the card page. Card pages are opened within the selected project, and a usage page lists cards from many projects.
- Grouping by requirement. A card can link several requirements, so its calls have no single requirement to count under.
