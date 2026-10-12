# RFC-0023: Checks decide when the AI review gives no verdict

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-069` for REQ-072. It supersedes in part RFC-065: review job step 2 ("With no review model, it blocks with `router_config`"), step 4 ("then blocks with `review_parse`. A provider failure blocks with `provider_error`"), the outcome "No checks: `human_review`" when the list is read right after the pull request opens, and the refresh sentence "A card in `agent_review` without a verdict is left alone". It also supersedes `TestReviewWithoutVerdictBlocks`.

Sprint 33 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-069 | A review that gives no verdict no longer blocks the card: the pull request checks decide it. An empty check list counts as no checks only 2 minutes after the card entered `pr_open` | RFC-065 review job steps 2 and 4 (`router_config`, `review_parse`, `provider_error`), "No checks" on the first read, and the refresh rule for `agent_review` without a verdict |

## RFC-069 Checks decide when the AI review gives no verdict

### Problem

The AI review is an extra pair of eyes. The pull request checks are the evidence. RFC-065 makes the review a hard gate in both directions: a reviewer that cannot be reached, cannot be routed, or gives no valid verdict blocks the card, and unblocking a review reason sends the card back to `ready`, which spends a new model attempt on a commit whose checks may already be green. A review that stops for any other reason, such as a worker shutdown, leaves the card in `agent_review` with no verdict, and nothing moves it again.

The review job also reads the checks about two seconds after `gh pr create`, before GitHub Actions has registered its runs. An empty list is then taken as "no checks", so a repository with CI can reach `human_review` without them.

### Decision

The review job keeps its order. `request_changes` still writes a `review` note and moves the card to `changes_requested`. `recommend_approve` still applies the checks. A missing company organization or token still blocks with `github_config`, since the checks cannot be read.

When the review gives no verdict, the job does not block. That covers no routable review model, a parse failure after the fallback model, a provider failure, and any other error while the job's context is live. The job:

1. Appends a `job_error` with the cause.
2. Stores `review_verdict = 'failed'` and `reviewed_sha` on the attempt. The review model is stored when there was one, and `review_independent` is false.
3. Stores the pull request head as `head_sha`.
4. Posts one pull request comment with the company token: `**AI review: Not done**`, the cause in words, and the short SHA. The comment never quotes the error. A failed comment appends `job_error` and does not change the card.
5. Applies the checks as for `recommend_approve`.

A refresh treats an `agent_review` card with a `failed` verdict as it treats `recommend_approve`. It may also find an `agent_review` card with no verdict. When the card lock is free, no review job is running, so the refresh records the review as failed ("The review job stopped before a verdict") and goes through steps 1 to 5. A held lock means a review is still running, and the refresh leaves the card.

Before applying the checks, the review job reads the pull request again, so runs that registered while the model worked count. A failed read, or a head that moved, keeps the first read.

An empty check list counts as "no checks" only when `review.NoChecksWait` (2 minutes) has passed since the card entered `pr_open`, which is the time of its latest `local_checks` gate. Before that the card stays in `agent_review` with `ci_required` unset, and the 30-second refresh reads the checks again. A check that appears in the meantime decides the card as before.

The card JSON adds `reviewVerdict`: `recommend_approve`, `request_changes`, `failed`, or `null`. `reviewIndependent` is `null` for a failed review. The card page shows "AI review: Failed, no verdict" in red, and the trace shows the same badge on the review block.

`review_verdict` accepts `failed` (migration `00017`). `machine.Transition` and its edges do not change: `agent_review` to `human_review` was already legal without a verdict in the evidence.

### Why

A person approves every card before merge (RFC-054), so a missing AI verdict loses an opinion, not a gate. Green checks are evidence that the commit builds and passes. Blocking on a reviewer outage wastes an attempt on a commit that is already correct. The PR comment and the red line on the card make sure the person knows there was no AI review. `request_changes` keeps its force, because that is a verdict.

Two minutes covers the time GitHub Actions takes to register runs after a push, and the refresh already polls every 30 seconds. The time of the `local_checks` gate is also correct after a rework push, unlike the pull request's creation time.

### Set aside

- A per-project switch to skip the AI review. The review still runs whenever it can.
- Letting green checks override `request_changes`.
- Reading required checks from branch protection. Every check on the head still counts (RFC-065).
- Remapping `CANCELLED` or `STALE` conclusions, and a deadline for checks that stay pending.

### Tests

- `TestReviewWithoutVerdictUsesChecks`: a parse failure with no checks moves the card to `human_review`, stores `failed` with `ci_required = false`, posts "AI review: Not done", and appends one `job_error`.
- `TestReviewWithoutVerdictRedChecks`: a red check moves the card to `changes_requested` with one `ci` note.
- `TestReviewWithoutVerdictWaitsForChecks`: pending checks keep `agent_review`, and green checks on refresh move the card to `human_review`.
- `TestNoChecksWaitsToRegister`: no checks right after `pr_open` keeps `agent_review` with `ci_required` unset, through a refresh. After `NoChecksWait`, the card reaches `human_review` with `ci_required = false`.
- `TestChecksInsideWaitDecide`: green checks inside the wait move the card to `human_review`.
- `TestRefreshFinishesStoppedReview`: an `agent_review` card with no verdict stays while another holder has the lock. Once the lock is free, it is recorded as failed and reaches `human_review`.
- `TestCardShowsLatestReview` adds `reviewVerdict`, and checks that a failed review has `reviewIndependent: null`.
