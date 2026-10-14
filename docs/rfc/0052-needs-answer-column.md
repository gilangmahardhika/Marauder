# RFC-0052: Needs answer column

Status: Accepted for implementation. Date: 2026-10-14.

This RFC adds decision `RFC-116` for REQ-097. It supersedes one statement of RFC-114: the board group of `waiting_answer`.

Sprint 90 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-116 | `waiting_answer` is in a new board group `question`. The board shows it as the lane "Needs answer", between Waiting and Running | RFC-114 "`waiting_answer` is in the board group `waiting`" |

## RFC-116 Needs answer lane

### Problem

A card waiting for an answer sat in the Waiting lane among backlog and ready cards. Only its amber label set it apart, so a person scanning the board for work that needs them missed it.

### Decision

- `Actions` returns group `question` for `waiting_answer`, with no status actions. The card's `answer` and `question_mode` actions are unchanged (RFC-114, RFC-115).
- The board renders seven lanes in this order: Waiting, Needs answer, Running, Review, Shipping, Finished, Stopped. "Won't fix" still follows when shown (RFC-112).
- A tile in the Needs answer lane has the amber stripe, like the "Needs answer" status label.
- A requirement change still moves `waiting_answer` to `blocked` with `source_changed`, as RFC-114 says.

### Why

The lane is where a person looks for what waits on them. A separate group keeps the client placing cards by `group` alone, so it still does not decide anything from the status.

### Set aside

- Keeping the group `waiting` and sorting answer cards first. The lane count and the empty state would still mix the two kinds of waiting.

### Tests

- `TestActionsWaitingAnswer`: `waiting_answer` is in group `question` with no status actions.
- Vitest: the board shows a `question` card in the "Needs answer" lane, between Waiting and Running.
