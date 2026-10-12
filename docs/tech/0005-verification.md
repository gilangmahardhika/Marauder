# Verification

Status: Accepted for implementation. Date: 2026-10-10. Revised after review.

This is how you know the backend in [0003-backend.md](0003-backend.md) and the frontend in [0004-frontend.md](0004-frontend.md) match the contract in [0002-data-and-api.md](0002-data-and-api.md). The release bar is PRD REQ-043. Live model keys are not required for that bar. A fake provider replays scripts. A real dev server is a later run of the same health and deploy steps.

## Layout

```text
backend/
  internal/machine/transition_test.go
  internal/contextpack/testdata/reviewer.json
  internal/providers/fake/fake.go
  testdata/fixture-repo/          small git repo, enables fixture mode
  testdata/scripts/               fake provider transcripts
  testdata/bin/deploy-stub        copied to a temp dir outside the repo by the runner
  testdata/prd/fixture.md
  testdata/rfc/fixture.md
  cmd/fixture/main.go             REQ-043 runner
frontend/
  src/lib/**/*.test.ts            Vitest with @testing-library/svelte
```

Run backend tests from `backend/`:

```text
go test ./...
```

The fixture command:

```text
go run ./cmd/fixture
```

It exits `0` only when all nine REQ-043 steps pass. It does not call Anthropic or OpenAI. It creates a temporary data directory outside the repo and copies the deploy stub to a temporary directory with mode `0700`, so RFC-036 and RFC-038 hold during the run.

`go test` and the fixture need `ORCHESTRATOR_TEST_DATABASE_URL`, a PostgreSQL 16 or newer database (RFC-044). Each test migrates that database and isolates its rows. A missing variable fails the test with that name before any card is written. Tests do not use testcontainers and do not create a SQLite file.

## Fake provider

`internal/providers/fake` implements the provider interface. A script is a list of responses keyed by task class and attempt number. Each response is text, a tool call, or an error.

```json
{
  "plan": [
    { "tool": "create_card", "ref": "a", "title": "Hello", "requirementIds": ["REQ-001"], "risk": "low" },
    { "tool": "create_card", "ref": "b", "title": "World", "requirementIds": ["REQ-002"], "dependsOn": ["a"], "risk": "low" }
  ],
  "implement": [
    { "tool": "edit_file", "path": "hello.txt", "text": "ok\n" },
    { "tool": "commit", "message": "Make the check pass" }
  ],
  "review": [
    { "tool": "verdict", "value": "recommend_approve" }
  ],
  "triage": [
    { "text": "hello.txt did not contain ok" }
  ]
}
```

The fake records the system and user strings it received. Tests read that record. The reviewer record must contain the diff and must not contain `author-transcript`.

Transport failure is a script entry `{ "error": "transport" }`. The router test uses two entries: primary fails twice, fallback succeeds.

## Unit tests

| Test | Asserts |
| --- | --- |
| `TestEdgesMatchPRD` | The edge map equals the REQ-006 table, and `done` has no outbound edge |
| `TestIllegalDeploySkip` | `in_progress` to `deploying_dev` returns `ErrIllegal` and writes `transition_rejected` |
| `TestEvidenceRequired` | `approved` without a SHA returns `ErrEvidence` |
| `TestTransitionCAS` | Two transitions from the same read status: one wins, the other returns `ErrConflict` and writes no event or gate |
| `TestApprovedHeadMoved` | `approved` with a new head goes to `human_review` and clears the approval row |
| `TestRequestChangesFromApproved` | Request-changes on `approved` clears approval and writes an `approver` note |
| `TestRejectFromApproved` | Reject on `approved` blocks with `rejected` and runs no deploy |
| `TestCapFromChangesRequested` | `changes_requested` at the cap goes to `blocked` with `attempt_cap` |
| `TestUnblockDestinations` | `health`, `deploy_failed`, and `deploy_config` go to `human_review`. `turn_cap` goes to `ready` |
| `TestUnblockAtCap` | Release at the cap with the default policy raises `max_attempts` by one. One attempt runs, then the card blocks again |
| `TestUnblockReset` | `resetAttempts` sets `attempts_used` to 0. `attempts.n` keeps increasing |
| `TestUnblockSourceChanged` | `unblockTo` is null and unblock is `422` while a linked id is unaccepted |
| `TestReviewNotCounted` | A review and a triage call leave `attempts_used` unchanged |
| `TestRedact` | A payload containing the fixture deploy token is stored as `[redacted]` |
| `TestReviewerPack` | Fixture JSON has the diff and lacks `author-transcript` |
| `TestImplementerTools` | Tool list has `read_linked_file` and no `deploy`, no `verdict`, and no linked write tool |
| `TestFallbackOnce` | Primary transport error, one retry, one fallback, then `blocked` with `provider_error` if the fallback also fails |
| `TestRouteClass` | Low risk and zero attempts select `implement`. High risk or any used attempt selects `implement_hard` |
| `TestNoDeployClass` | `Decide` rejects task class `deploy` |
| `TestRouterConfig` | An empty `implement` model blocks the card with `router_config` |
| `TestPlanCards` | Planner cards link only accepted ids, get checks from settings, and a dependency cycle is refused |
| `TestDraftIds` | A PRD without `REQ-` headings sets `draftPending`, and drafted rows are unaccepted |
| `TestManualRequirementAdd` | A Developer member adds with a blank id after `REQ-001` and gets `REQ-002`, unaccepted. A typed `REQ-010` is stored. A duplicate id is `409`. A bad id or empty statement is `422`. A non-member is `404` |
| `TestStatementLimitAndLineEndings` | A 10000-character statement is `200` and 10001 is `422` on `statement`. CRLF and CR in a statement are stored as LF on add and on edit |
| `TestAcceptanceLineMarkers` | Acceptance lines lose one leading `- `, `* `, `+ `, `1. `, `2) `, `- [ ] `, or `- [x] ` marker, keep inline markdown, and turn an inner line break into a space. A marker-only line is `422`, and a 1001-character line is `422` with "Keep each acceptance line to 1000 characters or fewer." |
| `TestPRBodyMarkdownStatement` | In the pull request body each requirement id is bold on its own line with the markdown statement under it. A statement that leaves a code fence open gets its closing line before "Acceptance criteria", so `## Changes` is not inside code; a closed `~~~` fence is left alone |
| `TestManualRequirementEdit` | An edit of an accepted manual row clears acceptance. Editing or deleting a parsed row is `409`. Deleting a manual id a card lists is `409`. Deleting an unused manual row removes it |
| `TestIntakeLinkedCards` | A card naming `REQ-001` is listed in that requirement's `linkedCards`. A card in another project naming `REQ-001` and `REQ-002` is not listed, and `REQ-002` has `[]` |
| `TestIngestKeepsManual` | A manual `REQ-001`, then ingest of a PRD that defines `REQ-001`. The row stays manual with its text, and `Skipped` is `[REQ-001]` |
| `TestSecretOmitted` | GET of the company GitHub, LLM, and project deploy settings has no token or key substring from the secrets file |
| `TestModelSettingsAdminOnly` | A Lead Developer who is not an admin reads LLM and router settings, and gets `403` on PUT and on a family test. An admin saves them, and no response has the saved key |
| `TestSecretKeep` | A project deploy PUT without the token leaves that project's `secrets.json` bytes unchanged |
| `TestUnknownModel` | Router PUT with model id `missing` returns `422` and the yaml is unchanged |
| `TestModelIdUnique` | The same model id in two families is `422` |
| `TestBotApprover` | An approval by a user whose GitHub login is the company bot login returns `422` |
| `TestOwnerCannotApprove` | The card owner's `actions` has no `approve`, and an approval POST by the owner is `403` with no approval row (RFC-054) |
| `TestSoloOwnerApproves` | With Solo approval off the owner is `403`. After `soloApproval: true` is saved, the owner, whose login is the bot login, sees `approve` and approves, with `independent = false` (RFC-067) |
| `TestSelfReviewNeedsLead` | A Developer's add with `selfReview: true` is `403`. The creator, as a Developer, is `403` on the self-review route. A Lead Developer's add stores the flag, and an admin turns it on and off. An unknown id is `404` (RFC-070) |
| `TestSelfReviewOwnerApproves` | With Solo approval off, a card that lists a flagged and an unflagged requirement gives the owner no `approve`, and the owner's POST is `403`. With only the flagged requirement the owner sees `approve`, approves to `approved`, and the row has `independent = false` |
| `TestCardSelfReview` | `app.CardSelfReview` is true only when every listed requirement is flagged. Mixed, none flagged, no requirements, or a missing id is false |
| `TestApprovalPostsCommentAndStatus` | An approval posts one comment on the pull request and one `success` status with context `orchestrator/approval` on the SHA, with the company token in `GH_TOKEN` and not in argv. A repeat approval of the same SHA posts nothing (RFC-068) |
| `TestApprovalAnnounceFailureKeepsApproved` | When `gh` fails, the approval is still `200` and `approved`, with one `job_error` for the comment and one for the status |
| `TestRequestChangesWithdrawsApprovalStatus` | Request changes on an approved card posts `failure` on the approved SHA |
| `TestRequestChangesReason` | A 4001-character reason is `422` with `fields.reason`, and the card stays `human_review` with no note. A reason is stored trimmed with LF line ends as the newest `approver` note with `attemptN` 0. An empty body stores `changes requested` |
| `TestReviewIgnoresApprovalStatus` | A pending `orchestrator/approval` status on the head still moves the card to `human_review`, with `ci_required = false` |
| `TestCardDiff` | On a real repository the diff lists each file with counts and hunks, redacts a planted model key, cuts a file past 1,500 lines, and returns not found for another project |
| `TestTraceOrderAndAttempts` | Three events appended in the same second, with ids that sort the other way, come back in insert order. `attemptId` is left out when empty. `attempts` lists the attempt's `n`, model, tokens, outcome, commit, and end, with `reviewIndependent` null. The card under another project is `404` |
| `TestJobErrorCapped` | A 1.5 MiB message with NUL bytes and invalid UTF-8 is stored under 10 KiB as valid UTF-8 without NUL, with its head, its tail (redacted), and the cut marker |
| `TestCardDiffRoute` | A member gets `200` with the head SHA, and `missing: true` without a clone. A non-member gets `404` |
| `TestReviewWithoutVerdictUsesChecks` | A reviewer with no verdict and no checks reaches `human_review` with `review_verdict = 'failed'`, `ci_required = false`, one "AI review: Not done" comment, and one `job_error` (RFC-069) |
| `TestReviewWithoutVerdictRedChecks` | No verdict and a red check: `changes_requested` with one `ci` note |
| `TestReviewNoteRedacted` | A `request_changes` body that holds the company token is stored as a `review` note with `[redacted]` in its place |
| `TestReviewWithoutVerdictWaitsForChecks` | No verdict and pending checks stay in `agent_review`. Green checks on refresh: `human_review` |
| `TestNoChecksWaitsToRegister` | No checks right after `pr_open` keeps `agent_review` with `ci_required` unset, through a refresh. After `NoChecksWait`: `human_review` with `ci_required = false` |
| `TestChecksInsideWaitDecide` | Green checks inside the wait move the card to `human_review` |
| `TestRefreshFinishesStoppedReview` | An `agent_review` card with no verdict stays while another holder has the lock. Once the lock is free, refresh records `failed` and reaches `human_review` |
| `TestIngestRecordsCreator` | A web ingest stores the caller as `created_by` on new parsed rows. A second ingest by another user keeps the first. A CLI ingest stores null |
| `TestCardOwnerFromRequirement` | `create_card` stores the requirements' creator as `owner_id`, and an `ownerId` argument is ignored |
| `TestCreateCardMixedOwners` | `create_card` naming requirements from two creators is rejected and inserts no card |
| `TestCardReviewers` | The board and the card route list the requirement reviewers sorted by email, without the owner, with `githubLogin` `""` when unverified. A card in another project naming the same requirement id has `"reviewers": []` |
| `TestCardDetail` | The single card returns `requirements` from its own project only, `rules` labelled `global` and by stack, and `skills[].useWhen`. The board list has no `requirements` or `rules`. Unblock returns the same detail fields |
| `TestCardDetailNotes` | The single card returns `notes` newest first with source, text, and `attemptN`, `0` for an approver note. A note stored under another project is left out. The board list has no `notes` |
| `TestCardPeopleNames` | The card route returns `owner.name` and `reviewers[].name`. A reviewer without a name has `"name": ""` (RFC-058) |
| `TestReviewersSetByCreator` | The creator and a Lead Developer set reviewers on a parsed row. Another Developer is `403`. A non-member reviewer, an unverified reviewer, or the creator is `422` |
| `TestConventionCatalogAdminOnly` | Every signed-in user reads `/api/conventions`. A Lead Developer who is not an admin gets `403` on create, update, and delete. An admin save appends `config_changed` with section `conventions` (RFC-055) |
| `TestConventionRevisions` | Editing a rule increments its version, changes its hash, and keeps the old text in `convention_revisions`. A save with the same text keeps the version |
| `TestDetectStacks` | A repo with `go.mod`, and a `web/package.json` that depends on `@sveltejs/kit` and `typescript`, is detected as Go, TypeScript, and SvelteKit. `node_modules` and a symlinked directory are not walked |
| `TestSyncKeepsManualStacks` | A Sync replaces detected stacks, keeps a stack added by hand, and appends `stacks_detected` |
| `TestProjectConventionsLeadOnly` | A Lead Developer member saves manual stacks and enabled skills. A Developer member is `403`. An unknown or disabled id is `422` |
| `TestCreateCardSkills` | `create_card` with an enabled skill stores it in `card_skills` and the card JSON lists it. A skill not enabled for the project stores no card (RFC-056) |
| `TestConfigEvent` | `config_changed` payload has section and field names and not the token |
| `TestSnapshot` | A router save during a running attempt does not change that attempt's model |
| `TestActionsEmpty` | Card JSON for `in_progress` has `"actions": []` |
| `TestActionsApproved` | Card JSON for `approved` has `request_changes` and `reject` only |
| `TestApproveSha` | Wrong SHA returns `422` and `approved_sha` stays null |
| `TestApproveIdempotent` | The same SHA twice inserts one `approvals` row |
| `TestCORS` | Origin `http://127.0.0.1:9999` is `403`. `http://127.0.0.1:4173` is allowed |
| `TestWorkerLock` | A second `serve` exits. `run-next` while `serve` runs inserts a `runs` row and exits `0` without running a job |

## Runtime tests

These use `testdata/fixture-repo`, a git repository with one file and a check script.

| Test | Asserts |
| --- | --- |
| `TestWorktreeOnce` | Two implement jobs for one card create one branch and one worktree |
| `TestCheckExit` | Gating exit `0` can open a pull request. Exit `1` cannot. A model message that says the tests passed does not override exit `1` |
| `TestToolCheckDoesNotMove` | `run_local_check` exit `0` inside an attempt does not change status |
| `TestNoCommitAttempt` | An attempt that ends without a commit counts, writes a `no_commit` note, and the next pass starts a new attempt |
| `TestCommandAllowlist` | A tool call that names a command does not exec it |
| `TestEnvScrub` | The sandbox env has no `GH_TOKEN`, model key, or `DEPLOY_TOKEN` |
| `TestVerifyAtDeployedSHA` | An uncommitted edit left in the worktree is absent from the dev check export |
| `TestPROnce` | Calling ensure-PR twice leaves one `pr_url` |
| `TestMetadataRewrite` | The second push updates the metadata block and keeps the summary block |
| `TestCIPending` | A pending required check keeps the card in `agent_review`. A later pass moves it when the check passes |
| `TestCIRed` | A failed required check moves the card to `changes_requested` with a `ci` note |
| `TestStaleSHA` | A new commit after approval clears `approved_sha` and returns the card to `human_review` |
| `TestDeployFailed` | Deploy stub exits `1`. Rollback runs. Card is `blocked` with `deploy_failed` |
| `TestHealthFail` | Stub health returns 500. Rollback runs once. Card is `blocked` with `health`. No `verify` event |
| `TestHealthOnly` | Empty `dev_check` and a healthy probe mark the card `done` with verify payload `mode: health_only` |
| `TestDevFailThenPass` | First dev exit `1` goes to `changes_requested` on the same `pr_url`, and the next implementer pack has the failure note. The second cycle exits `0` and marks `done` |
| `TestDependency` | Card B stays out of `in_progress` until card A is `done` |
| `TestDeployLock` | Two approved cards produce non-overlapping `deploy` events |
| `TestCap` | The third counted attempt failing on a card with `max_attempts` 3 ends `blocked` and opens no other attempt |
| `TestCommitIdentity` | Commits use the `runtime.Git.Author` they are given and carry the three trailers |
| `TestCommitIdentityOwner` | A commit on an owned card has the owner's verified login and `<id>+<login>@users.noreply.github.com` as author and committer (RFC-054) |
| `TestRequestReviewers` | The reviewer request names the union of the card's requirement reviewers without the owner, the bot, or an unverified user. A failed request writes `reviewers_requested` with `ok: false` and the card status is unchanged |
| `TestReconcileDeploy` | Kill after the `deploy` event and before `health`. Restart probes or rolls back and does not exec deploy again first |
| `TestReconcileCommit` | Kill after a commit and before the gating check. Restart runs the check without a model call and makes no second commit |

The health stub is an `httptest.Server` on `127.0.0.1`, allowed by fixture mode. It is not a public URL.

## REQ-043 runner

`go run ./cmd/fixture` performs the nine steps against a temporary data directory and `testdata/fixture-repo`.

| Step | What the runner checks |
| --- | --- |
| 1 | Ingest of a two-requirement PRD. `run-next` before accept creates no card and no worktree |
| 2 | After accept, the plan job creates two cards. The second `depends_on` the first |
| 3 | Card 1 has a worktree, a gating exit `0`, and one pull request URL |
| 4 | The reviewed attempt has `review_independent` true with two families in the fixture config. The recorded reviewer prompt has no `author-transcript` |
| 5 | Approve the head SHA. A new commit before deploy clears the SHA and sets `human_review`. Approve the new head |
| 6 | Health stub fails. Rollback ran. Card is `blocked` with `health`. No verify event. `unblockTo` is `human_review` |
| 7 | Unblock. The card is in `human_review` and no implementer attempt started. Re-approve the same head. The stub now passes health and the dev check fails. New attempt, same pull request, failure note stored |
| 8 | Health pass and dev exit `0` mark card 1 `done`. Only then does card 2 enter `in_progress` |
| 9 | Process stopped after `deploy` and before `health`. New process reconciles before a second deploy exec |

Step 6 uses the stub. The runner does not need a real dev server. After this command exits `0`, a person may point the same deploy and health code at one real dev target. That run is recorded by hand and is not part of `go test`.

The fixture config uses two families and two model ids so step 4 can require `review_independent` true. The fake provider serves both ids. No network.

After step 9, the runner runs `git status --porcelain` in the fixture repo and requires empty output (REQ-045).

## CLI checks

| Command | Expected |
| --- | --- |
| `orchestrator status` on an empty data dir | Exit `0`, stdout contains `board is empty` and `orchestrator ingest` |
| `orchestrator approve card_x` without `--sha` | Exit `1`, no `approvals` row |
| `orchestrator approve card_x --sha wrong` | Exit `1`, stderr has the validation message |
| `orchestrator run-next` when idle | Exit `0`, stdout contains `no card is ready` |
| `orchestrator unblock card_x --reset-attempts` on a `turn_cap` card | Exit `0`, card `ready`, `attemptsUsed` 0 |
| `orchestrator serve --data <repo>/.orchestrator` | Exit `1` before listen |

## Frontend checks

From `frontend/`:

```text
npm ci
npm run check
npm run test
npm run build
npm audit --omit=dev
```

The Vitest cases are listed in [0004-frontend.md](0004-frontend.md). `npm audit` with no runtime dependencies should report nothing. Any finding blocks the release until it is read and recorded.

## Browser pass

Run this after `orchestrator serve` and `npm run dev` are up. Use the fixture data dir so the board has a known `in_progress` card, a known `human_review` card, a known `approved` card, and a known `blocked` card.

| Check | Pass |
| --- | --- |
| Open the UI without a bearer | Login form is the only page |
| Wrong password | Live region shows the error, no board |
| Six wrong passwords for one email | Live region says to wait one minute |
| Empty data dir | "No cards yet" and a link to Intake |
| `in_progress` card | No action controls |
| `human_review` card | Approve form shows `headSha`. Submitting another SHA shows the field error and the card stays in `human_review` |
| `approved` card for a Lead Developer | Request changes, Reject, and Deploy |
| `approved` card for a Product Owner | Request changes and Reject, and no Deploy |
| `blocked` card with `health` | Unblock text says the card returns to human review |
| Approve in the UI while the CLI approves the same card | One approval row. The second action gets a reload or the same card back |
| Settings GET after a token was saved | Password field is empty. Placeholder says a value is saved. The network response has no token |
| Deploy settings with a repo-relative command | Field error on the command |
| Router with one family | The independence sentence from the API is visible |
| Router save of an unknown model | Field error, yaml unchanged |
| Leave the board while a poll is active | No further card requests after navigation |
| Built page | Response `index.html` has the CSP meta tag. The console shows no CSP violation |
| Keyboard | Tab reaches nav, Run, and the settings submit. Focus outline is visible |

This pass is manual. The automated locks for buttons are `TestActionsEmpty`, `TestActionsApproved`, and the `CardActions` Vitest cases.

## Hardening tests

The cases in [0006-security-and-harness.md](0006-security-and-harness.md) are part of `go test ./...`. A build that skips `TestCheckOnExport`, `TestHookNotRun`, `TestFsmonitorOff`, `TestHealthLinkLocal`, `TestEventsImmutable`, `TestPushURL`, or `TestDataDirInsideRepo` is not a release candidate.

Sprints 11–13 add `TestPasswordLogin`, `TestWrongPassword`, `TestSessionLockout`, `TestBearerExpiry`, `TestLastAdmin`, `TestUserGitHubOmitted`, `TestNonAdminForbidden`, `TestMemberSeesProject`, `TestOutsiderMissesProject`, `TestReqIdsPerProject`, `TestBranchPerProject`, `TestAdminManagesMembership`, `TestParallelProjects`, `TestLeadDeploy`, `TestOwnerApproveStops`, `TestAcceptByOwner` (Sprint 16 replaces it with `TestAcceptSeniorRoles`), `TestDeveloperRunOnly`, `TestActorOnTransition`, and `TestCloudBind`. `TestLoopbackOnly` still passes when `ORCHESTRATOR_CLOUD` is unset. The REQ-043 fixture stays one project. Those new tests add a second project beside it. They are not part of the fixture's nine steps.

## Release order

1. `go test ./...` in `backend/`.
2. `go run ./cmd/fixture` exits `0`.
3. Frontend checks.
4. Browser pass on the fixture cards.
5. Optional: one real dev target for health failure and health success, using the same deploy code paths, with secrets only in the data directory's `secrets.json`.

Stop at the first failure. A green browser pass does not substitute for the fixture command.
