# RFC-0049: Won't fix for blocked cards

Status: Accepted for implementation. Date: 2026-10-14.

This RFC adds decision `RFC-112` under REQ-006. A blocked card can be closed as won't fix, so it leaves the board, and reopened later. It supersedes no statement. NG-002 still holds: the harness never closes a pull request on its own; the close is one person's action.

Sprint 81 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-112 | A `wont_fix` status. A Lead Developer, a Product Owner, or the card owner moves a `blocked` card there with a reason, after its open pull request is commented on and closed with their own GitHub token. The board hides these cards unless asked, and Reopen returns the card to `blocked` with its old reason | none |

## RFC-112 Won't fix for blocked cards

### Problem

A blocked card had two ways out: release it to `ready` or `human_review`. Work the team decided not to do stayed in the Stopped column forever, and its pull request stayed open on GitHub.

### Decision

The status:

- `cards.status` gains `wont_fix` (migration 30). Its board group is `closed`, and it offers no action other than `reopen`.
- `blocked -> wont_fix` needs a reason and an actor. The card keeps its `blocked_reason`. No gate row is written.
- `wont_fix -> blocked` restores the kept reason, or `reopened` when there was none, so a `source_changed` card still waits for Intake and the usual unblock rules apply.
- Only `done` satisfies a dependency, so a card that depends on a `wont_fix` card stays in `backlog`.

Who:

- A Lead Developer, a Product Owner, or the card owner sees `wont_fix` on a `blocked` card, including one blocked for `source_changed` that cannot be released yet, and `reopen` on a `wont_fix` card. Other roles see neither, and the routes answer `403`.

The routes:

- `POST /api/projects/{projectId}/cards/{id}/wont-fix` with `{ "reason": "..." }`. The reason is trimmed and 1 to 500 characters, otherwise `422` on `reason`. A card that is not `blocked` is `409`.
- When the card has a pull request, the server reads its state with `gh api repos/{owner}/{repo}/pulls/{n} --jq .state` using the caller's own verified GitHub token on the company host. A caller with no verified token gets `422` "connect your GitHub account in Account to close the pull request", and nothing changes.
- Only a pull request in state `open` gets the comment "Closed as won't fix by @login: {reason}" and then `PATCH pulls/{n}` with `state` `closed`, each with the JSON in a file. A closed or merged pull request is left alone.
- The GitHub calls run before the status change. A refusal is `422` with GitHub's first line, redacted, and the card stays `blocked`.
- On success the card moves `blocked -> wont_fix` and a `wont_fix` event records `reason`, `prUrl`, `prClosed`, and `login`.
- `POST /api/projects/{projectId}/cards/{id}/reopen` moves a `wont_fix` card back to `blocked` (`409` otherwise). It does not touch GitHub. The next publish edits the pull request with `state` `open` as well as its title and body, so a pull request closed as won't fix reopens with the new work.

The board:

- The board hides `wont_fix` cards and leaves them out of the card count and owner filter. "Show won't fix" (`?wontfix=1`) shows them in a "Won't fix" column after Stopped.

### Why

Closing is a decision about the work, and it belongs to the people who own scope (Lead Developer, Product Owner) and to the person who owns the card. A reason makes the decision readable in the trace and on GitHub. The person's own token names them on GitHub, as for Squash and merge (RFC-111). Keeping the blocked reason means a reopen cannot skip the gate that blocked the card.

### Set aside

- Deleting cards. The trace and attempts stay readable, and a reopen is possible.
- Won't fix from any status. A running or reviewed card has a job or a person acting on it; it is rejected or blocked first.
- Closing the pull request with the company token. The close would not name the person.
- A CLI command. Closing the pull request needs a person's GitHub token.

### Tests

- `TestBlockedToWontFix`: the edge needs a reason and an actor, keeps `blocked_reason`, writes no gate, and `wont_fix -> blocked` restores the reason.
- `TestActionsWontFix`: a Lead Developer, a Product Owner, and the owner see `wont_fix`; a QA Lead does not and gets `403`; a `wont_fix` card is in group `closed` and offers `reopen`.
- `TestWontFixClosesPR`: a bad reason is `422` with no GitHub call; then one state read, one comment, and one `PATCH` with `state: closed`, the user token in the environment only, the card `wont_fix`, and a `wont_fix` event.
- `TestWontFixNoPR`: no token needed and no GitHub call.
- `TestWontFixNoToken`: `422`, no GitHub call, and the card stays `blocked`.
- `TestWontFixClosedPR`: a closed or merged pull request gets the state read only.
- `TestWontFixRefused`: a refused `PATCH` is `422`, and the card stays `blocked`.
- `TestReopen`: `403` for a QA Lead, then `blocked` with the old reason, then `409` on a second reopen.
- `TestEnsurePRReopens`: the publish edit sends `state: open`.
- `board.test.ts`, `status.test.ts`, and `CardActions.test.ts`: won't fix cards are hidden unless asked, the status reads "Won't fix", the form needs a reason and posts it beside a primary Unblock, and Reopen posts to `reopen`.
