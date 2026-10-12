# RFC-0019: A card reaches Approved

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-065`. It builds the review job and worker pass steps 1, 2, 6, and 7 from [tech/0003](../tech/0003-backend.md), and fixes what stopped the first real card (a label change in a Laravel app) between its commit and its pull request. It supersedes in part the "required checks" wording of tech/0003 pass step 7 and review job step 5. Deploy and verify are still not built.

Sprint 28 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-065 | A checked pull request is reviewed by a model and waits for its checks, and a person approves it. Every check on the pull request head counts as required | tech/0003 "required checks" in pass step 7 and review job step 5 |

## RFC-065 A checked pull request is reviewed, and a person approves it

### Problem

The first real card committed the right change, then stopped three times before it had a pull request:

1. The gating check failed with `git exit 125`. `git archive` of a whole Laravel repository is larger than the 1 MiB output cap of `execx`, so the archive was cut and the job saw a failed command.
2. The next attempt started a new model run, although the earlier commit was correct and had never been checked.
3. The Podman machine was stopped, so the check could not start, and the card was blocked with `check_error` after spending an attempt.

Past `pr_open`, nothing ran: no review job, no CI wait, no head refresh. A card with a pull request stayed in `pr_open`.

### The gating export goes through a file

`runtime.Git.Export` runs `git archive --output=<dest>.tar <sha>`, reads that file through the same tar reader that refuses absolute names, `..`, and links outside the export (0006), then deletes the file. No archive passes through a pipe with an output cap. A git error message is cut at 2 KiB and ends with `(truncated)`.

### A rerun checks and publishes an existing commit before it calls a model

When the implement job starts on a card whose latest attempt has a `commit_sha` and an `ended_at`, and either no outcome or `checks_passed` while the card has no pull request, and the worktree HEAD is that commit, the job does not call a model. It moves the card to `self_check`, runs the gating check on that commit, and continues as RFC-063 step 10 or 11 with that attempt. An attempt that already passed is not counted again. This is Reconcile step 3 done inside the job, so a check that could not start, or a push that failed, no longer costs a model run.

### The sandbox engine is probed first

Before an attempt on a real repository, the job runs `<engine> version` with the engine environment and a 10-second limit. A non-zero exit writes a `job_error` naming the engine and its first output line, adds that the Podman machine or the Docker daemon must be started, and blocks the card with `sandbox_config`. No attempt row is inserted and no attempt is spent.

### `gh` can find git

`gh pr create` runs git even with `--repo` and `--head`, and the `gh` child environment had no `PATH`, so it failed with `unable to find git executable in PATH`. A job's `gh` child now gets `PATH` set to the directory of the git binary resolved at startup, with `GIT_CONFIG_NOSYSTEM=1` and `GIT_TERMINAL_PROMPT=0`. The token stays in `GH_TOKEN` only.

### Leaving blocked clears the reason

`machine.Transition` clears `blocked_reason` when the card leaves `blocked`. Before this, a card unblocked to `ready` still showed the old reason.

### Child rows keep the card's project

Rows in `attempts`, `gates`, and `approvals` take `project_id` from their card. Before this, they took the column default `proj_default`, so a project-scoped query missed them.

### Card routes without a project check the role

`POST /api/cards/{id}/{action}` requires a session, the card's project must be visible to the user (`404` otherwise), and the action must be allowed for the user's job role (`403` otherwise), the same as the project-scoped route.

### The review job

The worker starts a review job for each `pr_open` card, at most two at a time per project. A review that fails before it moves the card, because `gh` or `git` failed, writes a `job_error`, leaves the card in `pr_open`, and is tried again after one minute.

1. The job takes the card lock, reads the pull request with the company token (`gh pr view --json headRefOid,body,comments,statusCheckRollup`), and diffs local `refs/heads/<baseBranch>` to the checked commit, up to 64 KiB, then `truncated: true`.
2. It moves the card to `agent_review`. With no company organization or token, it blocks with `github_config`. With no review model, it blocks with `router_config`.
3. The review route `different-family-from-author`, or an empty route, picks the first model of another family than the attempt's model (`router.ReviewModel`). With one family, the reviewer is a model of the same family and `review_independent` is false. An explicit route model is used as named.
4. The reviewer tools are `read_diff`, `comment_pr`, and `verdict`. `comment_pr` posts with the company token through `gh pr comment --body-file`. The loop ends at the first valid `verdict` and runs at most 8 model turns. No verdict, or two invalid verdicts, is a parse failure, which is retried once with the route's fallback model and then blocks with `review_parse`. A provider failure blocks with `provider_error`.
5. The job stores `review_model_id`, `review_prompt_hash`, `review_independent`, `review_verdict`, and `reviewed_sha` on the reviewed attempt, and the pull request head as the card's `head_sha`. A review never changes `attempts_used`.
6. `request_changes` writes a failure note with source `review` and moves the card to `changes_requested`. `recommend_approve` applies the pull request checks.

### Every check on the head counts as required

`gh pr checks --required` reports its result through exit codes that `ghcli` treats as errors, so the job reads `statusCheckRollup` and counts every check run and status context on the head as required. Success, neutral, and skipped pass. Queued, in progress, pending, and expected are pending. Anything else fails.

- No checks: `human_review`, and the attempt stores `ci_required = false`.
- All pass: `human_review` with `ci_required = true`.
- Any failure: `changes_requested` with a `ci` note naming the failed checks.
- Pending: the card stays in `agent_review`.

A repository whose optional checks fail therefore needs those checks fixed or removed before a card reaches `human_review`.

### Refresh

Each tick, the worker refreshes cards in `agent_review` with a `recommend_approve` verdict, `human_review`, and `approved`, at most once every 30 seconds per card and two at a time. A refresh reads the pull request, stores a new head through `app.HeadMoved`, which sends an `approved` card back to `human_review` and clears its approval, and applies the checks to an `agent_review` card. A card in `agent_review` without a verdict is left alone.

The card JSON fills `reviewedSha` and `reviewIndependent` from the latest attempt with a review verdict.

### Approval

Approve stays a person's action on `human_review` with the head SHA (RFC-054). The card's owner cannot approve their own card. The approver needs a role that may approve and must be a member of the project's tribe. Reaching `approved` does not start deploy, and deploy is not built in this sprint.

### Tests

`TestExportLargeArchive` exports a 3 MiB file and leaves no `.tar` behind. `TestImplementGatesExistingCommit` gates the commit of an earlier attempt after an unblock with no model call. `TestImplementRepublishesPassedCommit` publishes a passed commit after a failed push with no model call and no second count. `TestImplementBlocksWhenEngineDown` blocks with `sandbox_config` and a `job_error` naming the engine. `TestRunnerPathHasOnlyGitDir` gives `gh` the git directory as its only `PATH`. `TestLeavingBlockedClearsReason`, `TestCardRowsKeepCardProject`, and `TestLegacyCardRoutesCheckRole` cover the three fixes. `TestReviewerTools` lists the three reviewer tools. `TestReviewApproveNoCI`, `TestReviewRequestChanges`, `TestReviewPendingCIWaits`, `TestPassFinishesCIWait`, `TestPassFailedCIRequestsChanges`, `TestReviewParseFallback`, and `TestReviewWithoutVerdictBlocks` cover the verdicts and checks. `TestRefreshStoresHeadSHA` and `TestApprovedHeadMovedReturnsToReview` cover the refresh. `TestAdvancerSchedulesReviewAndRefresh` covers the schedule. `TestCardShowsLatestReview` covers the card JSON. `TestCardReachesApproved` runs one card from `ready` to `approved`.
