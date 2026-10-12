# RFC-0020: The harness writes the pull request text

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-066`. It supersedes in part the pull request body in [tech/0003](../tech/0003-backend.md) ("two regions", with a summary written by the implementer below the metadata), and the "one-line" commit message of the implementer's `commit` tool. The PRD rule stands: the harness writes the metadata section and rewrites it on each push.

Sprint 29 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-066 | The harness builds the pull request title and description from the requirements, the commit, the changed files, and the check, and the review job posts its verdict as a comment | tech/0003 pull request body regions; the one-line commit message; `gh pr edit` and `gh pr comment` in tech/0003 and RFC-065 |

## RFC-066 The harness writes the pull request text

### Problem

The first real pull request (`regol-app/scbd-solid-web#34`) was titled `REQ-001`, the card title copied from a manual requirement with no heading. Its body started with raw `card:`, `attempt:`, and `model:` lines, because only the markers around them were HTML comments, then repeated `REQ-001`. The useful text existed: the commit subject, the requirement statement, and its acceptance line. The review verdict was on the board only.

### Title

The title is the card's requirement ids, a colon, and the card title. When the card title is empty or names only the requirement ids, the first line of the attempt's commit message is used instead. A title that already starts with the ids is not prefixed again. The title is one line, cut at 100 characters with `...`. `gh pr create` passes it on the first push.

### Writes go through REST

`gh pr edit` asks GraphQL for fields that need the `read:org` scope, which a token limited to Contents and Pull requests does not have. A later push therefore reads the body with `gh api repos/OWNER/REPO/pulls/NUMBER` and writes the title and body with `--method PATCH --input FILE`. Comments, both the verdict and the reviewer's `comment_pr`, use `POST repos/OWNER/REPO/issues/NUMBER/comments`. The input file is JSON under the project's `logs/` and is deleted after the call, so no title or text reaches argv.

### Body

No model call writes the body. The harness owns the text between `<!-- orchestrator:body:start -->` and `<!-- orchestrator:body:end -->` and rewrites it on each push:

1. **Summary**: the commit message, with the `Orchestrator-*` and `Model:` trailers removed. The implementer is asked for a subject of at most 72 characters in the imperative mood, a blank line, and two or three sentences on what changed and why. An empty message falls back to the card title.
2. **Requirement**: each requirement id in bold with its statement, read in the card's project, then its acceptance lines as an unticked checklist.
3. **Changes**: each changed file from the merge base of the local base branch to the commit, with added and deleted lines, or `binary`. At most 50 files, then `and N more files`.
4. **Verification**: the local check command and the short SHA it exited 0 on.
5. **Review**: a line that the AI review is a comment and a person approves the card before merge.
6. **Metadata**, inside `<details>`, between the existing metadata markers: requirement ids, local check, card, attempt, model, and the full commit SHA. Cards carry no decision ids yet, so there is no `decisions` line.

Text a person adds above or below the region is kept. A body with only the metadata markers, from before this RFC, is replaced whole. A body with no marker at all follows the region. The commit message and requirement text cannot close the region: `<!-- orchestrator:` in them is escaped. The title and body pass through the job's redaction list, which holds the model keys, the company token, and the owner's token. A git read that fails leaves its section out and does not stop the publish.

### Review comment

After the review job stores the verdict, it posts one comment with the company token:

```text
**AI review: Recommend approve**

<the verdict body>

Reviewed `ae5d441` with `deepseek-flash`, a model of the same family as the author, so this review is not independent. A person approves the card in the orchestrator before merge.
```

`request_changes` uses `**AI review: Request changes**` and says the card went back to the implementer. The comment is redacted. A failed comment appends `job_error` with `review comment:` and does not change the card. Comments the reviewer posts with `comment_pr` are separate.

### Tests

`TestPRTitleFromCommit` covers an ids-only, empty, descriptive, and already prefixed card title, and the 100-character cut. `TestPRBodySections` checks each section, the trailers removed, the binary file, and that requirement text cannot close the region. `TestPRBodyKeepsHumanText` keeps text a person wrote around the region, replaces a legacy body, and keeps an unmarked body. `TestImplementReachesPROpen` checks the `--title` argument, the body sections, and a planted owner token shown as `[redacted]`. `TestPublishEditsExistingPR` edits an existing pull request with a REST `PATCH` and never `gh pr edit`. `TestReviewPostsComment` and `TestReviewRequestChanges` check the verdict comment. `TestReviewCommentFailureKeepsStatus` moves the card to `human_review` after a failed comment and records the `job_error`.
