# RFC-0047: Card page for review

Status: Accepted for implementation. Date: 2026-10-14.

This RFC adds decision `RFC-110` under REQ-071. The card page leads with the next step, shows the pull request title and description the harness published, and makes the diff readable. It supersedes no statement. RFC-068's Changes panel stays and gains line numbers, a file list, and controls.

Sprint 78 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-110 | Publish stores the pull request title and harness body on the attempt, and the card page shows them with the next step first and a numbered, navigable diff | none |

## RFC-110 Card page for review

### Problem

The card page listed its panels in the order they were added. The buttons that move a card sat under the notes. Facts such as SHAs and reasons took a full-width panel above the code. The pull request title and description that the harness writes (RFC-066) went to GitHub and were not stored, so a person had to open GitHub to read what the change says it does. The Changes panel opened one file at a time, had no line numbers, and showed added and deleted lines by color alone.

### Decision

Pull request text:

- `attempts` gains `pr_title` and `pr_body`. After `Publish` creates or edits the pull request, it stores the title it sent and the harness part of the body on the publishing attempt, redacted with the publish secrets. A failed store does not fail the publish, because the pull request is already out.
- The single-card JSON adds `pullRequest`: the title, the body, the attempt number, and the commit SHA of the newest attempt with a stored title, or `null`. The body leaves out the "Orchestrator metadata" block, because the card page renders markdown without HTML. The board list leaves it out.
- The text is the harness's. Edits a person makes on GitHub outside the body markers are not read back.

Card page:

- The header shows the title, the status badge, the owner, "Attempt x of y", and the branch. Its buttons are "Open pull request" when `prUrl` is a GitHub URL, and "View trace".
- From 1024px wide the page has two columns. The main column holds, in order: Next step, Pull request, Changes, Notes, Instructions, and Context. The sidebar holds Details, People, and Intake. Below 1024px the sidebar follows the main column.
- Next step is always shown. When `actions` has `approve`, the primary button names the short head SHA, the AI review verdict and reviewed SHA sit beside it, Request changes is a visible button, and Reject is under "More actions". "Approve a different SHA" opens the SHA field. The request body is unchanged. With no listed control, Next step says no action is open.
- Every control still comes from `actions`. The page still has one primary button.

Changes panel:

- Each line shows its old and new line number from the hunk header, and the hunk shows "Lines a to b" with the function context.
- Added and removed lines carry screen-reader text, so color is not the only signal. Line numbers and signs cannot be selected, so a copy takes only code.
- A file list grouped by folder jumps to and opens a file. Expand all and Collapse all open or close every file. Wrap lines wraps long lines instead of scrolling sideways.
- File headers stay visible while their file scrolls. A diff of 10 files or fewer and 400 changed lines or fewer opens every file. A larger one starts closed.

### Why

A person on the card page is there to decide. The decision, what the change says it does, and the code should come before the history. Storing the text at publish shows exactly what went to GitHub without a GitHub call per view, and a card in `self_check` or a blocked publish shows nothing rather than a guess.

### Set aside

- Reading the live pull request body from GitHub. It needs a token call per view and returns text the harness did not write.
- Building the text again on read with `BuildPRText`. It can differ from what GitHub received once the requirements or commit change.
- A side-by-side diff view. The unified view with line numbers covers review on the narrow main column.

### Tests

- `TestLatestPRText`: the newest attempt with a stored title wins, and another project reads nothing.
- `TestPublishStoresPRText`: a publish stores the title and the redacted body on its attempt.
- `TestPRBodyForDisplay`: the display body has no metadata block and keeps the escaped marker as text.
- `TestCardDetailPullRequest`: `pullRequest` is `null` before publish, carries the stored text after, and the board list leaves it out.
- `diff.test.ts`: hunk headers parse, lines are numbered, hunks are labelled, and files group by folder.
- `CardChanges.test.ts`: numbered and labelled lines, Expand all and Collapse all, Wrap lines, a large diff starting closed, and the file list opening a file.
- `CardPullRequest.test.ts`: title, branch, commit, rendered description, the long-description toggle, the empty states, and no link outside GitHub.
- `CardActions.test.ts`: Approve names the head SHA beside the AI review, the different-SHA field, the visible Request changes, and Reject under More actions.
- `routes/cards/[id]/page.test.ts`: the main column starts with Next step, Pull request, and Changes, and the sidebar holds Details, People, and Intake.
