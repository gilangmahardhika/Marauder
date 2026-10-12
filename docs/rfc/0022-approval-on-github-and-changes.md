# RFC-0022: Approval on GitHub and the Changes panel

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-068` for REQ-071. It supersedes in part RFC-065 ("counts every check run and status context on the head as required"), only for the status context `orchestrator/approval` that this RFC writes.

Sprint 31 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-068 | A new approval posts a pull request comment and a green `orchestrator/approval` commit status on the approved SHA, and the card page shows the head's diff | RFC-065 required-checks sentence, for `orchestrator/approval` only |

## RFC-068 Approval on GitHub and the Changes panel

### Problem

An approval in the orchestrator moved the card to `approved` and stopped there. The pull request on GitHub showed nothing, so a reader of the pull request could not tell it was approved, and branch protection could not require it. The card page also had no view of the code, so the approver had to open GitHub to see what they were approving.

A GitHub review with `APPROVE` does not fit. GitHub refuses it from the pull request author, the owner's token opens the pull request, and on a solo install (RFC-067) the approver, the owner, and the bot are one account.

### Decision

On approval:

- When `Approve` inserts a new approval row, the harness posts one comment on the card's pull request and one commit status on the approved SHA, both with the company token. A repeat approval of the same SHA posts nothing.
- The comment says who approved and the short SHA, says when the approval is not independent (RFC-067), and says that merging stays a separate step on GitHub (NG-002).
- The status has context `orchestrator/approval`, state `success`, and the description `Approved by @<login> in the orchestrator`.
- Request changes or Reject on a card that was `approved` posts `orchestrator/approval` with state `failure` on the approved SHA.
- The calls run after the transition commits, with a 30 second limit that the request's cancel does not cut. A failure writes a `job_error` event that starts with `approval comment:` or `approval status:` and does not change the card. The card stays `approved`.
- A missing pull request URL, SHA, or data directory posts nothing.
- The CLI `approve` does the same as the route.

The review job's check reading skips the `orchestrator/approval` context, so the harness's own pending or failed status never holds a card in `agent_review` or fails it. Every other check run and status context still counts as required.

The company token needs permission to write commit statuses. A classic token with `repo` has it. A fine-grained token needs "Commit statuses: write".

On the card page:

- `GET /api/projects/{projectId}/cards/{id}/diff` returns the diff from `refs/heads/<baseBranch>` to the card's head SHA, or the last attempt's commit when the card has no head yet. Members of the project's tribe may read it. Other callers get `404`.
- When the SHA is not in the project clone, the server fetches `refs/heads/orch/<cardId>` with the company token. If it is still missing, the response says `missing: true` and lists no files.
- The diff is capped at 256 KiB, and each file at 1,500 lines. A cut is marked `truncated`. Renames show as a delete and an add. Binary files are listed without lines. Model keys and the company token are redacted before parsing.
- The Changes panel lists each file with its added and deleted counts, opens a file to its hunks with added lines in green and deleted lines in red, and links to the GitHub compare view of the base and the SHA.

### Why

A comment is what a person reading the pull request sees, and a commit status is what branch protection can require. Neither needs a second GitHub account, and both name the SHA the approval is bound to (RFC-006). Failing to post must not undo an approval that the board already holds, so the post is a side effect with a recorded error.

Reading the diff from the project clone uses the same git and the same redaction as the reviewer pack, and needs no new token scope.

### Set aside

- A GitHub `APPROVE` review. GitHub refuses it from the author, and on a solo install every login is the author.
- Merging on approval. NG-002 keeps merging a human step on GitHub.
- Rendering the diff as HTML. The panel renders text nodes, and `{@html}` stays out of the client.
- Reading the diff from the GitHub API. It caps large files without saying so, and it needs a network call per view.

### Tests

- `TestApprovalPostsCommentAndStatus`: an approval posts one comment and one `success` status on the SHA, and a repeat approval posts nothing.
- `TestApprovalAnnounceFailureKeepsApproved`: a failing `gh` leaves the card `approved` and writes a `job_error`.
- `TestRequestChangesWithdrawsApprovalStatus`: request changes on an approved card posts `failure` on the approved SHA.
- `TestReviewIgnoresApprovalStatus`: a pending `orchestrator/approval` context still moves the card to `human_review` with `ci_required = false`.
- `TestCardDiff`: a real repository gives per-file counts and hunks, redacts a planted key, truncates a large file, and refuses another project.
- `TestCardDiffRoute`: a member gets `200`, and a non-member gets `404`.
- `CardChanges.test.ts`: the panel lists files with counts, opens a file to colored lines with the compare link, warns when the diff is cut, and shows the missing-commit message.
