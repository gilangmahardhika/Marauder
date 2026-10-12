# RFC-0029: Retry an AI review that gave no verdict

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-076` under REQ-072. It adds the edge `agent_review -> pr_open` to the PRD state table and the card action `retry_review`. RFC-069 stands: a review that gives no verdict still lets the pull request checks decide, unless a person asks for the review again.

Sprint 41 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-076 | A person may send an `agent_review` card whose AI review is `failed` back to `pr_open`, which clears that review so the worker runs the review job again on the same attempt | None. Adds an edge and an action |

## RFC-076 Retry AI review

### Problem

A review job runs once, when the card is in `pr_open`. When it gives no verdict (RFC-069), the attempt stores `failed` and the card waits in `agent_review` for the pull request checks. Nothing runs the review again, and an `agent_review` card offers no action. A review that failed for a passing reason, such as a provider error or a reply cut at the token limit before RFC-075, leaves the card with no AI verdict for good. When the checks stay pending, the card has no way forward at all.

### Decision

- New edge `agent_review -> pr_open`. Its evidence is the attempt id and the actor id. It writes no `local_checks` gate: the attempt's local check already passed, and `board.PublishedAt` keeps reading the first gate for the no-checks wait.
- `app.RetryReview(projectID, cardID, actorID)` takes the card lock, requires status `agent_review` and the latest review verdict `failed`, and in one transaction moves the card to `pr_open` and clears the attempt's review columns. Any other status or verdict is `ErrReviewNotFailed` (409). A held lock is `board.ErrLocked` (409).
- The worker's existing pass starts `ReviewCard` for the `pr_open` card. The review job, its fallback, RFC-069, and RFC-075 apply unchanged. `attempts_used` does not change.
- The card JSON lists `retry_review` when the card is in `agent_review`, its latest review is `failed`, and the role may run cards: Lead Developer, Senior Developer, Developer, QA.
- `POST /api/projects/{projectId}/cards/{id}/retry-review` calls `app.RetryReview` and returns the card. Another method is `405`. The CLI has `orchestrator retry-review CARD`.
- The card page shows a "Retry AI review" button when the action is listed.

### Why

Going back to `pr_open` reuses the one review job, its lock, and its worker slot, instead of a second path that runs a review from `agent_review`. Clearing the review keeps RefreshCard's rule for a review job that stopped (an empty verdict) correct for the new run. The run roles match: retrying a review spends model tokens like a run does, and it decides nothing a human gate decides.

### Set aside

- Retrying a review that gave a verdict. A person who disagrees with a verdict uses the human gates.
- Moving a card on without its pending checks. Checks that never finish are a CI problem to fix in CI.
- A new attempt for the retry. The commit already passed its local check.

## Tests

- `TestAgentReviewToPROpen`: the edge needs the attempt and the actor and writes no gate.
- `TestRetryReviewQueuesReview`: a failed review moves the card to `pr_open`, clears the review, keeps one `local_checks` gate, records the actor, and the next review job records its verdict without counting an attempt.
- `TestRetryReviewNeedsFailedVerdict`: `pr_open`, a `recommend_approve` verdict, and another project are refused, and the card stays.
- `TestRetryReviewLocked`: a held lock is `ErrLocked` and keeps the `failed` review.
- `TestRetryReviewAction`: a Developer sees `retry_review`, `GET` is `405`, `POST` returns `pr_open` without the action, and a second `POST` is `409`.
- `TestRetryReviewRole`: a Product Owner does not see `retry_review`, and `POST` is `403`.
- `CardActions.test.ts` "retry AI review shows only when listed and posts to retry-review".
