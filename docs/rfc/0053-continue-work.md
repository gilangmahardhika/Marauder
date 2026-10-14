# RFC-0053: Continue unfinished work

Status: Accepted for implementation. Date: 2026-10-14.

This RFC adds decisions `RFC-117`, `RFC-118`, and `RFC-119` for REQ-098. A long attempt compacts its own context instead of overflowing it, an attempt that ends without a commit hands its progress to the next one, and a runner that never ran the card starts from the branch the control plane holds. RFC-118 supersedes one statement of RFC-114, that a parked attempt's uncommitted edits are discarded. NG-008 still holds: no attempt sends an earlier attempt's turns; the next attempt reads a short handoff in its pack, as it reads a failure note.

Sprints 91 to 93 in [../plan/sprints.md](../plan/sprints.md) implement this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-117 | When a reply's input tokens reach 75% of the routed model's `contextTokens`, or the provider reports a full context, the model writes a handoff summary in one call without tools. The attempt then goes on with that summary in place of its earlier turns. An attempt compacts at most three times | none |
| RFC-118 | An attempt that ends without a commit saves a handoff: a summary, a patch of its uncommitted changes, and its todo list. The card's next attempt applies the patch, starts with the todo list, and reads the summary and the branch's commits in a `WORK SO FAR` block. A handoff is applied once | RFC-114: "a parked attempt's uncommitted edits are discarded when the next attempt starts". The `ask_user` description now says the next attempt starts with the answers and the uncommitted edits |
| RFC-119 | A runner starting a card that has no pull request seeds `orch/<card>` from the control plane's copy of the branch when that copy is ahead of the base | none |

## RFC-117 Compaction inside an attempt

### Problem

An attempt sends every earlier reply and tool call back on each request. The history budget drops old tool results, but replies and calls stay, so a long attempt reached the model's context window and ended with a provider error, losing its work.

### Decision

- Each model may set `contextTokens` in its `modelOptions`, from 8,000 to 2,000,000. Zero means 128,000. The compaction point is 75% of it.
- After a model call whose `inputTokens` plus `cachedInputTokens` reach the compaction point, the loop compacts before the next request. A provider reply that the context is full (anthropic "prompt is too long", openai `context_length_exceeded`) compacts at once and sends the turn again. A full context is not retried on the same model or the fallback.
- Compacting is one request with the attempt's system prompt, user message, and history, no tools, a reply cap of 2,048 tokens, and a note asking for a handoff of at most 400 words under: Goal, Done, In progress, Next steps, Files touched, Decisions and assumptions, Open problems. Its tokens count toward the attempt and the card cap.
- The summary is redacted and cut at 8 KiB. The attempt's earlier turns are dropped, and its user message becomes the pack plus `WORK SO FAR`, the summary, and `END OF WORK SO FAR`. The system prompt and the todo list stay.
- A `context_compacted` event records `inputTokens`, `turnsDropped`, and `summaryBytes`.
- A fourth compaction, or a compaction request that is itself too large, ends the attempt with outcome `budget` and reason `context_full`. It saves a handoff (RFC-118).

### Why

The model that did the work knows what it did; a short summary from it costs one call and keeps the attempt going. Dropping the turns keeps later requests small, which saves tokens on every following call.

### Set aside

- Summarizing with another model. It does not see the reasoning, and a second route adds settings.
- Raising the history budget. It moves the wall, it does not remove it.

## RFC-118 Handoff to the next attempt

### Problem

A new attempt reset the worktree to its last commit and deleted untracked files. Edits an attempt made before a cap, a wall-clock limit, or a provider error were lost, and the next attempt explored and wrote them again.

### Decision

Saving:

- An attempt that ends with outcome `budget`, `no_commit`, `question`, or `error` saves one handoff in `attempt_handoffs`, keyed by the attempt.
- The patch is the worktree against `HEAD`, built through a temporary index (`read-tree HEAD`, `add -A`, `diff --binary --cached HEAD`), so the worktree's own index does not change. A file over 1 MiB is left out of it and the patch is dropped when it is over 2 MiB; the summary says what was left out.
- The summary is the model's (as in RFC-117, without dropping turns) after a turn cap, a reply without a commit, or a question. After a token cap, a full context, a wall-clock limit, a provider error, or a failed summary call, the harness writes it from the todo list, the patch's changed files, the commits since the base, the last local check exit and its last 2 KiB, and the last compaction summary.
- The save runs after the attempt's context is cancelled, with its own 30-second limit, so a wall-clock stop still saves.
- A `handoff_saved` event records `files`, `patchBytes`, and `source`.

Applying:

- The next attempt of the card, after it resets the worktree, reads the handoff of the card's latest attempt when that handoff is not yet applied.
- When `HEAD` is the patch's base commit, it applies the patch as is; otherwise with a three-way merge. A patch that does not apply is skipped.
- The handoff is marked applied either way, so it is used once. A `handoff_applied` event records `ok`, `files`, and the reason when it failed.
- The attempt starts with the handoff's todo list.
- The pack has a `WORK SO FAR` block after the answers: the summary, "Commits on this branch since the base:" with up to 40 one-line subjects, and whether the uncommitted work is in the worktree or could not be re-applied. The block is untrusted data between markers.

Storage and exposure:

- The patch and the summary are never returned by the API. The trace shows counts and the source only.

### Why

The worktree is the card's real state; keeping the edits and the plan lets the next attempt continue at the next step instead of the first. Unblocking a card at its attempt cap now continues the work instead of repeating it.

### Set aside

- Committing work in progress on the card branch. A commit starts the check and publish path, and a broken commit would reach a pull request.
- Sending the previous attempt's turns. NG-008.

## RFC-119 A runner without a pull request

### Decision

- `start` sets `cardBundle: true` when the card has no pull request and the control plane's `orch/<card>` is ahead of the base.
- The runner then reads `GET /api/runner/jobs/{lease}/bundle`, a git bundle of that branch, and seeds its own `orch/<card>` from it unless the local branch already contains that commit. It checks the bundle as the control plane checks a runner's bundle: one head, the card branch.

### Why

The control plane already holds every commit a runner made (RFC-108). Without this, a card that moved to another runner before it had a pull request started again from the base.

## Tests

- `TestCompactOnContextLimit`, `TestCompactOnOverflowError`, `TestCompactCap`.
- `TestHandoffSavedOnBudget`, `TestHandoffHarnessFallback`, `TestHandoffAppliedNextAttempt`, `TestHandoffConflictSkipped`, `TestHandoffTodosSeeded`, `TestHandoffAppliedOnce`.
- `TestPackIncludesWorkSoFar`, `TestUnblockContinuesWork`, `TestContextFullClassified`, `TestContextTokensRange`.
- `TestRunnerHandoffRoundTrip`, `TestRunnerSeedsFromControlPlaneBundle`.
- Vitest: the `FamilyForm` context window field and the `trace` words for the three new kinds.
- `go run ./cmd/fixture` runs the `continueWork` scenario.
