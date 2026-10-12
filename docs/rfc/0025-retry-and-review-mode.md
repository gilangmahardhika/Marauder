# RFC-0025: Retry after a failed attempt, and the AI review mode of a requirement

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decisions `RFC-071` and `RFC-072` for REQ-074. RFC-071 supersedes in part RFC-063 ("The worker starts an implement job only for a card with an open card-scoped `runs` row... so no model tokens are spent until a person presses Run"), only for a card a person already ran. RFC-072 supersedes in part RFC-069 ("`request_changes` still writes a `review` note and moves the card to `changes_requested`", "`request_changes` keeps its force, because that is a verdict", and the set-aside item "Letting green checks override `request_changes`"), only for a card with a requirement in `manual` mode.

Sprint 37 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-071 | After an attempt ends with `no_commit` or `checks_failed`, the harness queues the next attempt while attempts remain. A card out of attempts blocks with `attempt_cap`, and Run is not offered for it | RFC-063 "only ... until a person presses Run", for a card a person already ran |
| RFC-072 | Each requirement has an AI review mode, `continue` or `manual`. In `continue`, a `request_changes` verdict sends the card back and queues the next attempt. In `manual`, the card goes on to the pull request checks and a person decides | RFC-069 "`request_changes` ... moves the card to `changes_requested`" and its set-aside item, for `manual` cards |

## RFC-071 Retry after a failed attempt

### Problem

A card a person ran stops after an attempt that made no commit, or whose local check failed. The card stays `in_progress` with a failure note and the attempts it has left, but nothing starts the next attempt, so the person has to notice and press Run again. The worker pass step 10 in tech/0003 already says the next pass picks it up; RFC-063 took that away for every card nobody queued.

### Decision

When an implement attempt ends with `no_commit`, or with `checks_failed` and the card back in `in_progress`, the job checks the attempt cap in the same place it does today (`app.AtCap`):

- When `attempts_used >= max_attempts`, the card blocks with `attempt_cap`, as before.
- Otherwise the job inserts one card-scoped `runs` row with trigger `retry` (migration `00019` allows the value). The worker starts it like a Run, oldest first and under the project's cap. The board shows the card as queued.

`app.CanStartImplement` refuses a card with `attempts_used >= max_attempts`, so Run is neither offered nor accepted for it, and a queued row for it does not start an attempt. Such a row is closed and the card blocks with `attempt_cap`.

A card nobody ran still does not start. A crash that cancels an attempt does not queue a retry. Unblock and its Reset attempt counter do not change.

### Why

The person already chose to spend tokens on the card when they pressed Run. The attempt budget, not a second click, is what bounds the spend. Writing a row keeps one path into the implement job, so the cap, the card lock, and the queued badge work as they do for Run.

### Set aside

- Retrying after `blocked`. A block names something a person fixes.
- A separate retry budget. `max_attempts` already bounds it.

## RFC-072 The AI review mode of a requirement

### Problem

The AI review's `request_changes` always sends the card back (RFC-069). A person cannot accept the head as it is, because Approve needs `human_review` and `changes_requested` only leads to `in_progress` or `blocked`. A team that wants a person to judge the review has no way to.

### Decision

`requirements.ai_review_mode` is `continue` or `manual`, default `continue` (migration `00019`). Repository ingest does not write it, so a re-ingest keeps the value.

Only a Lead Developer or an admin may set it (`intake.CanSetSelfReview`). They may set it when they add a manual requirement, with `aiReviewMode` in the `POST /api/projects/{projectId}/requirements` body, or later with `PUT /api/projects/{projectId}/requirements/{id}/ai-review-mode` and the body `{"aiReviewMode": "manual"}`. Anyone else asking for `manual` gets `403`. Another value is `422`. The update is scoped by `project_id`, and a missing requirement is `404`. Each requirement in the intake JSON has `aiReviewMode`.

A card is `manual` when any requirement it lists has `ai_review_mode = 'manual'` in the card's project (`app.CardAIReviewMode`). The card JSON adds `aiReviewMode`.

When the AI review's verdict is `request_changes`, the review job still records the verdict, posts the verdict comment, and writes a `review` failure note from the body. Then:

- `continue`: the card moves to `changes_requested` and the job queues the next attempt as in RFC-071, in the same transaction. A card out of attempts blocks with `attempt_cap`.
- `manual`: the card stays in `agent_review` and the pull request checks decide, as for `recommend_approve`. Green checks, or none after the wait, move it to `human_review`. A red check moves it to `changes_requested` with a `ci` note. The comment says a person decides in the orchestrator.

In `human_review`, the card page shows "AI review: Request changes" and the review note. An approver may Approve the head, Request changes, or Reject, as for any card. `machine.Transition` and its evidence do not change.

A refresh treats an `agent_review` card with a `request_changes` verdict by the card's current mode: `manual` applies the checks, and `continue` sends it back with a retry.

### Why

The mode sits on the requirement for the same reason Self review does (RFC-070): that is where the owner and the reviewers come from, and cards are re-planned. One `manual` requirement is enough, because the setting asks for a person, which is the cautious side. The default keeps the review's force and adds only the retry.

### Set aside

- A per-project switch. A project that wants every card reviewed by a person sets each requirement.
- Approving straight from `changes_requested`. The checks still have to run first.

## Tests

- `TestRetryAfterNoCommit`: a `no_commit` attempt with attempts left leaves one open `retry` row. At the cap the card is `blocked` with `attempt_cap` and has no row.
- `TestRetryAfterChecksFailed`: a failed check with attempts left leaves the card `in_progress` with one open `retry` row.
- `TestRunRefusedAtCap`: a card at the cap is not startable and has no `run` action.
- `TestReviewContinueQueuesRetry`: `request_changes` on a `continue` card reaches `changes_requested` with one open `retry` row; at the cap it is `blocked` with `attempt_cap`.
- `TestReviewManualReachesHumanReview`: `request_changes` on a `manual` card keeps the `review` note and reaches `human_review` with green checks; pending checks wait and a refresh finishes; a red check reaches `changes_requested`.
- `TestRefreshSendsBackWhenNoLongerManual`: a `manual` card waiting with `request_changes` whose requirement turns `continue` is sent to `changes_requested` with one `retry` row on refresh.
- `TestCardAIReviewMode`: one `manual` requirement makes the card `manual`.
- `TestAIReviewModeNeedsLead`: a Developer asking for `manual` gets `403`, an unknown value `422`, and a Lead Developer's change shows in the intake and card JSON.
- Vitest: the Reviewers panel shows the mode control to a lead and a read-only line to others.
