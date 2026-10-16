# RFC-0061: AI review note always in the implement pack

Status: Accepted for implementation. Date: 2026-10-16.

This RFC adds decision `RFC-131` for REQ-075. It supersedes in part RFC-073: each implement attempt still reads the newest instructions and the newest other failure note, and additionally always reads the newest `source = review` note when one exists, so a later check or approver note cannot hide the AI reviewer's feedback.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-131 | Every implement attempt's pack includes the newest `failure_notes` row with `source = review` under a labeled block when one exists, next to the newest non-`instructions` / non-`review` failure note and the current instructions | RFC-073 "Each implement attempt reads the newest `instructions` note and, separately, the newest note of any other source" (review is no longer only that single other note) |

## RFC-131 AI review feedback in the pack

### Problem

RFC-073 loads one newest non-instructions failure note. After AI `request_changes`, a later `local_check`, `ci`, `no_commit`, or human `approver` note becomes newest and hides the review body the implementer must fix. Auto-retry (`continue`) and manual Run share that pack builder, so both lose the feedback.

### Decision

- The board exposes the newest `source = review` note for a card (empty when none).
- The newest failure note used as the general failure note excludes `instructions` and `review`.
- When a review note is non-empty, the implementer pack places it inside the markers as a block between `AI REVIEW FEEDBACK` and `END OF AI REVIEW FEEDBACK`, before the general failure note and before instructions. Builders strip forged copies of those two lines from untrusted text like the instructions lines.
- The implementer system string says to start from the AI review feedback when that block is present, and still from the failure note when there is one.
- Auto-queued retries and person-started Runs use the same pack builder; the run trigger does not change which notes are loaded.

### Why

The AI reviewer's `request_changes` body is the primary fix list for the next attempt. Keeping it visible next to check or approver notes matches how a person reads the card Notes panel, without changing when continue vs manual queues a run.

### Set aside

- Merging multiple historical review notes into one block.
- Writing the review body into an `instructions` note.

## Tests

- `TestPackReviewNote`: the review block sits inside the markers; a forged heading in other text is dropped; the system string names the block.
- `TestAttemptReadsInstructionsAndNote`: a `review` note plus a newer `local_check` note still puts the review body under the review heading, and keeps instructions when set.
- Continue-mode coverage: after AI `request_changes`, the next implement prompt still contains the review body (existing continue path plus pack assert).
