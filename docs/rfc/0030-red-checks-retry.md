# RFC-0030: Red checks retry, and a sent-back card out of attempts blocks

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-077` under REQ-074. It extends RFC-071 ("A card out of attempts blocks with `attempt_cap`, and Run is not offered for it") to the two ways a card reaches `changes_requested` that RFC-071 and RFC-072 did not cover: a red pull request check, and a person's Request changes. RFC-063 stands for a person's Request changes: the card waits for Run while attempts remain.

Sprint 42 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-077 | A red pull request check queues the next attempt as an AI request for changes does, or blocks with `attempt_cap`. A person's Request changes on a card out of attempts blocks with `attempt_cap`. The worker blocks any `changes_requested` card out of attempts | None. Extends RFC-071 |

## RFC-077 Red checks retry, and the cap applies to every send-back

### Problem

`review.ApplyCI` moves a card with a red check to `changes_requested` and stops. No attempt is queued, so a person must press Run, unlike an AI request for changes, which queues the next attempt (RFC-071, RFC-072). When the card has used all its attempts, Run is not offered, the card is not blocked, and `changes_requested` has no other action: nobody can move it. A person's Request changes on a card out of attempts ends in the same place.

### Decision

- A red check moves the card to `changes_requested` with its `ci` note, and in the same transaction queues a `retry` run while attempts remain, or moves it on to `blocked` with `attempt_cap`. On a `manual` card (RFC-072) a person decides, so the red check queues nothing and the card waits for Run, but a card out of attempts still blocks with `attempt_cap`.
- `app.RequestChanges` on a card out of attempts moves it on to `blocked` with `attempt_cap` in the same transaction, after storing the approver note. With attempts left it still waits for Run.
- The worker's refresh pass also visits `changes_requested` cards, at most once every 30 seconds per card, and calls `app.AtCap`, which blocks a card out of attempts with `attempt_cap`. That releases cards stuck before this RFC.
- Unblock with "Reset attempt counter" returns such a card to `ready`, as for any `attempt_cap` block.

### Why

Every path into `changes_requested` now ends the same way RFC-071 set for attempts: the next attempt is queued, the card waits for a person's Run, or it is blocked with a reason that Unblock understands. The approver note and the `ci` note stay, so the attempt after Unblock reads them.

### Set aside

- Queueing an attempt after a person's Request changes. RFC-063 keeps Run with the person, who may add instructions first.
- Raising `max_attempts` from the card page. Reset attempt counter on Unblock covers it.

## Tests

- `TestCIRedQueuesRetry`: a red check with attempts left reaches `changes_requested` with one `ci` note and one open `retry` run.
- `TestReviewManualReachesHumanReview` "red checks" and "red checks out of attempts": a `manual` card with a red check queues no `retry` run, and out of attempts it blocks with `attempt_cap`.
- `TestCIRedAtCapBlocks`: a red check on refresh for a card out of attempts blocks it with `attempt_cap` and queues nothing.
- `TestRefreshCapsChangesRequested`: refresh blocks a `changes_requested` card out of attempts and leaves one with attempts left.
- `TestRequestChangesAtCapBlocks`: Request changes on a card out of attempts blocks it with `attempt_cap` and keeps the approver note.
- `TestAdvancerRefreshesChangesRequested`: the worker's tick refreshes a `changes_requested` card.
