# Sprint checklist

Status board for [sprints.md](sprints.md). This file tracks progress. It does not change the schedule, and it does not override a PRD acceptance check, an RFC statement, or a tech guide.

- Check a task only when the behavior is in the tree.
- Check a test only when that named test exists and passes.
- A sprint is done only when every box in that section is checked.
- Do not start the next sprint while the current section is open.

## Sprint 1: The board cannot skip a gate

- [x] Create `backend/` as module `orchestrator/backend` on Go 1.26, with goose migration `00001_init.sql` (postgres dialect) from the schema in the data contract. The board is PostgreSQL 16 or newer through pgx. There is no SQLite file.
- [x] `go test` needs `ORCHESTRATOR_TEST_DATABASE_URL`. A missing variable fails the test with that name before any card is written. Tests do not use testcontainers. Each test migrates that database and isolates its rows.
- [x] Implement `Transition`: the REQ-006 edge map, evidence checks, and compare-and-swap (RFC-040). `done` has no outbound edge.
- [x] Append events with redaction. Triggers reject `UPDATE` and `DELETE` on `events` (RFC-031).
- [x] Startup refuses `--data` inside the repo or the worktree (RFC-036) and refuses a non-loopback `--addr` (RFC-032).
- [x] `TestEdgesMatchPRD`
- [x] `TestIllegalDeploySkip`
- [x] `TestEvidenceRequired`
- [x] `TestTransitionCAS`
- [x] `TestRedact`
- [x] `TestEventsImmutable`
- [x] `TestDataDirInsideRepo`
- [x] `TestLoopbackOnly`

## Sprint 2: Nothing starts until a person accepts the ids

- [x] Parse markdown with goldmark. Keep existing `REQ-` and `RFC-` ids. A file with no ids sets `draftPending` and stores drafted rows as unaccepted.
- [x] Record operator acceptance. Re-parse clears acceptance only for changed ids (RFC-019).
- [x] CLI: `ingest`, `accept`, and `status` on an empty board (the empty-board and missing-`--sha` CLI checks).
- [x] `TestDraftIds`
- [x] `status` on an empty data dir prints `board is empty` and `orchestrator ingest`
- [x] No worktree package exists yet

## Sprint 3: Checks cannot rewrite the worktree

- [x] `execChild`: argv only, built env, timeout, output cap. Refuse `sh` and `bash` (RFC-027).
- [x] Sandbox argv: `--network=none` for local checks, digest pin, export mount, no Docker socket, no worktree path (RFC-028, RFC-037). Dev-check argv is specified here but executed in Sprint 7.
- [x] Worktree and commit identity without a model. Git runs with hooks off, `core.fsmonitor` off, and a private `GIT_CONFIG_GLOBAL` (RFC-033).
- [x] Path rule: reject `../`, absolute paths, `.git/config`, and symlinks that leave the worktree.
- [x] `TestWorktreeOnce`
- [x] `TestCheckExit`
- [x] `TestCommandAllowlist`
- [x] `TestEnvScrub`
- [x] `TestCheckOnExport`
- [x] `TestPathEscape`
- [x] `TestSandboxFlags`
- [x] `TestNoShell`
- [x] `TestHookNotRun`
- [x] `TestFsmonitorOff`
- [x] `TestTarEscape`
- [x] `TestCheckParse`

## Sprint 4: A scripted implementer can commit

- [x] Fake provider replays scripts by task class and attempt. It records the system and user strings.
- [x] Tool loop stops at `maxTurns`. Unknown tools and a scripted `deploy` call are rejected and do not exec.
- [x] `create_card` cannot set the check command. Checks come from settings.
- [x] No commit counts as an implementer attempt and writes a `no_commit` note. `run_local_check` inside the attempt does not move status.
- [x] Prompt packs wrap untrusted text in the markers from the security guide. Commit trailers are `Orchestrator-Card`, `Orchestrator-Attempt`, and `Model`.
- [x] `TestImplementerTools`
- [x] `TestToolNotInProfile`
- [x] `TestNoCommitAttempt`
- [x] `TestToolCheckDoesNotMove`
- [x] `TestCommitIdentity`
- [x] `TestPromptMarkers`
- [x] `TestCreateCardIgnoresCommand`

## Sprint 5: One pull request, then a cold review

- [x] Ensure-PR is idempotent on card id. Metadata block is rewritten on each push. The summary block stays. Push uses the settings URL and the card refspec only, with the token in askpass, not in argv (RFC-034).
- [x] Reviewer verdicts: `request_changes` to `changes_requested`, `recommend_approve` plus green required CI to `human_review`. Pending CI stays in `agent_review`. Red CI goes to `changes_requested` with a `ci` note.
- [x] Review and triage do not increment `attempts_used` (RFC-042). `review_independent` is stored and is false when only one family is configured (RFC-008).
- [x] `TestPROnce`
- [x] `TestMetadataRewrite`
- [x] `TestPushURL`
- [x] `TestAskpass`
- [x] `TestReviewerPack`
- [x] `TestReviewNotCounted`
- [x] `TestCIPending`
- [x] `TestCIRed`
- [x] `TestRouteClass`
- [x] `TestNoDeployClass`

## Sprint 6: Approval is a person and a SHA

- [x] `approve --sha`, `request-changes`, `reject`, `unblock`. Missing or wrong SHA exits 1 and writes no approval row.
- [x] Same SHA twice inserts one approval. Bot login cannot be an approver.
- [x] Unblock destinations follow RFC-041: budget and reject go to `ready`; `health`, `deploy_failed`, and `deploy_config` go to `human_review`. `source_changed` stays blocked while a linked id is unaccepted. Default release at the cap raises `max_attempts` by one. `--reset-attempts` zeros `attempts_used`.
- [x] `TestApprovedHeadMoved`
- [x] `TestRequestChangesFromApproved`
- [x] `TestRejectFromApproved`
- [x] `TestApproveSha`
- [x] `TestApproveIdempotent`
- [x] `TestStaleSHA`
- [x] `TestUnblockDestinations`
- [x] `TestUnblockAtCap`
- [x] `TestUnblockReset`
- [x] `TestUnblockSourceChanged`
- [x] `TestCapFromChangesRequested`
- [x] CLI approve and unblock checks

## Sprint 7: Deploy the approved SHA, then test it

- [x] Deploy and rollback commands are absolute paths outside the repo and the data directory, mode `0700` or `0500` (RFC-038). The SHA argument is 40 or 64 hex only.
- [x] Probe allows a listed dev host, refuses link-local, loopback outside fixture mode, and unlisted hosts. A redirect is a failure (RFC-039).
- [x] Empty `dev_check` plus a healthy probe is `health_only` and `done`. A failing dev check returns to `changes_requested` on the same PR and stores the triage note.
- [x] Reconcile: deploy event without health probes or rolls back before another deploy exec. Commit without a gating check reruns the check and does not call the model.
- [x] `TestDeployFailed`
- [x] `TestHealthFail`
- [x] `TestHealthOnly`
- [x] `TestDevFailThenPass`
- [x] `TestVerifyAtDeployedSHA`
- [x] `TestDeployCommandPath`
- [x] `TestSHAArg`
- [x] `TestHealthLinkLocal`
- [x] `TestHealthLoopback`
- [x] `TestHealthPrivateListed`
- [x] `TestHealthUnlisted`
- [x] `TestHealthRedirect`
- [x] `TestReconcileDeploy`
- [x] `TestReconcileCommit`
- [x] `TestDevCheckNetwork`

## Sprint 8: One worker runs the board

- [x] `config.yaml` and mode `0600` `secrets.json`. GET never returns a token. PUT without a secret field keeps the stored bytes. Short secrets and unknown model ids are `422`. A config change event names fields, not values (RFC-030).
- [x] Router: retry primary once, then one fallback, then `blocked` with `provider_error`. Empty route blocks with `router_config`. A save during a running attempt does not change that attempt's model.
- [x] Worker lock file. A second `serve` exits. Plan job creates cards only for accepted ids and refuses a dependency cycle. Implementation cap and the global deploy lock are enforced. A moved base ref blocks with `base_moved`.
- [x] `TestSecretOmitted`
- [x] `TestSecretKeep`
- [x] `TestShortSecret`
- [x] `TestUnknownModel`
- [x] `TestModelIdUnique`
- [x] `TestBotApprover`
- [x] `TestConfigEvent`
- [x] `TestSnapshot`
- [x] `TestRouterConfig`
- [x] `TestFallbackOnce`
- [x] `TestPlanCards`
- [x] `TestWorkerLock`
- [x] `TestDependency`
- [x] `TestDeployLock`
- [x] `TestCap`
- [x] `TestBaseRefUnchanged`
- [x] `TestLockHolder`

## Sprint 9: The fixture script and the JSON API

- [x] Fixture runner uses a temp data directory, copies the deploy stub outside the repo, replays the fake provider for two families, and requires a clean `git status` in the fixture repo (REQ-045).
- [x] HTTP: bearer session, loopback `Host` check, CORS only for `http://127.0.0.1:4173`, 1 MiB body cap, the error shape from the contract. Card JSON includes server-computed `group`, `actions`, and `unblockTo`.
- [x] Session lockout after 5 failures. Wrong `Origin` and evil `Host` are `403`.
- [x] `go run ./cmd/fixture` exits 0
- [x] `TestActionsEmpty`
- [x] `TestActionsApproved`
- [x] `TestCORS`
- [x] `TestHostHeader`
- [x] `TestSessionLockout`
- [x] `TestRedactPack`
- [x] `TestSandboxOffRejected`
- [x] `TestAskpass` still passes inside `go test ./...`

## Sprint 10: The operator UI and the release pass

- [x] App shell: client-only SvelteKit, `adapter-static`, CSP meta, session bearer in `sessionStorage`, `src/lib/api.ts` covering every route.
- [x] Work pages: `/login`, `/` (six groups, Run, 3s poll), `/intake`, `/cards/[id]`, `/cards/[id]/trace`. `CardActions` renders only names in `actions`.
- [x] Settings pages: GitHub, LLM APIs, router, deploy. `SecretField` omits an empty secret and never prints a token.
- [x] Styles: system font, one `.primary` button, `.danger` only for blocked, 44px controls, visible focus, one column under 720px.
- [x] `npm run check`, `npm run test`, `npm run build`, and `npm audit --omit=dev` from `frontend/`
- [x] Vitest: `CardActions` empty, approved, and `unblockTo: null`
- [x] Vitest: `SecretField` omits the secret
- [x] Vitest: `api` clears the bearer on 401
- [x] Built `index.html` has the CSP meta and no `unsafe-inline`
- [x] Manual browser pass on fixture cards, from the table in the verification guide
- [ ] Optional, after that pass: one real dev host using the same deploy code, with secrets only in `secrets.json`. That run is recorded by hand and is not part of `go test`

## Sprint 11: A person is an account

- [x] Store passwords as argon2id hashes. Reject a password shorter than 12 characters.
- [x] `POST /api/session` takes email and password and returns a bearer bound to that user. The row stores the bearer hash. Expiry is 12 hours and is checked on every request. `DELETE /api/session` drops that bearer.
- [x] Five bad passwords for one email pause that email for one minute. A different email is not paused.
- [x] `orchestrator users bootstrap` creates the first admin from `ORCHESTRATOR_ADMIN_EMAIL` and `ORCHESTRATOR_ADMIN_PASSWORD` when `users` is empty. The last admin cannot be removed or disabled.
- [x] Admin `POST /api/users` and `PUT /api/users/{id}`. A non-admin receives `403`.
- [x] `GET` and `PUT /api/me/github` and `POST /api/me/github/test`. The token is ciphertext under `tokenKey`. GET returns `tokenSet` only.
- [x] Login page: email and password. Account page: password change and GitHub settings.
- [x] `TestPasswordLogin`
- [x] `TestWrongPassword`
- [x] `TestSessionLockout`
- [x] `TestBearerExpiry`
- [x] `TestLastAdmin`
- [x] `TestUserGitHubOmitted`
- [x] `TestNonAdminForbidden`

## Sprint 12: A tribe hides its projects

- [x] Migration for tribes, members, projects, and `project_id` on board rows. Per-project intake state and deploy lock. Unique requirement ids, branch names, and pull request URLs are per project.
- [x] Admin can create a tribe, add or remove a member, and create a project. A non-admin is `403`.
- [x] A member lists the project and its cards. A non-member, and an unknown project id, get `404`.
- [x] Board, intake, run, and card routes live under `/api/projects/{projectId}/`.
- [x] Project switcher stores the project id and no secret.
- [x] `TestMemberSeesProject`
- [x] `TestOutsiderMissesProject`
- [x] `TestReqIdsPerProject`
- [x] `TestBranchPerProject`
- [x] `TestAdminManagesMembership`

## Sprint 13: Work follows the project

- [x] Each project has a data directory outside its repo, with settings, project secrets, a worker lock, and worktrees.
- [x] Project GitHub settings hold the repo and the bot token. Push uses that token, not the signed-in user's token.
- [x] `actions` follows the job role. Accept is Product Owner only. Deploy is Lead Developer only.
- [x] A card in `approved` stays there until a Lead Developer member starts deploy.
- [x] Two projects may implement and deploy at the same time. One project still deploys one card at a time.
- [x] New transition events include `actorId`. Old events are not updated.
- [x] `ORCHESTRATOR_CLOUD=1` may bind off loopback. The default bind stays loopback.
- [x] `TestParallelProjects`
- [x] `TestLeadDeploy`
- [x] `TestOwnerApproveStops`
- [x] `TestAcceptByOwner` (replaced by `TestAcceptSeniorRoles` in Sprint 16)
- [x] `TestDeveloperRunOnly`
- [x] `TestActorOnTransition`
- [x] `TestCloudBind`
- [x] `TestLoopbackOnly` still passes without `ORCHESTRATOR_CLOUD`

## Sprint 14: One company account, one repo per project

- [x] An admin saves, tests, and clears the company GitHub account. The token is ciphertext and never returned.
- [x] A Lead Developer lists the organization's repositories.
- [x] A project names one repository and a base branch. Two projects cannot name the same repository.
- [x] Sync clones or fetches the project repository with the company token, never in argv.
- [x] Worktrees branch from the project base branch. Intake and accept are per project.
- [x] Deploy settings and the deploy token are saved per project. The service-wide deploy route is gone.
- [x] `TestCompanyGitHubTokenNeverReturned`
- [x] `TestCompanyGitHubAdminOnly`
- [x] `TestRepoListUsesCompanyToken`
- [x] `TestProjectRepoUnique`
- [x] `TestCloneTokenNotInArgv`
- [x] `TestCheckoutUsesBaseBranch`
- [x] `TestIntakePerProject`
- [x] `TestProjectDeploySettingsIsolated`
- [x] `TestProjectDeployTokenNeverReturned`
- [x] `TestDeployLeadOnly`

## Sprint 15: A project reads its neighbours

- [x] A Lead Developer links projects in the same tribe, with shared paths per link.
- [x] Packs add linked requirements, decisions, and shared files inside the untrusted markers.
- [x] `read_linked_file` reads a linked export and cannot write.
- [x] An event records the linked project ids and SHAs for each attempt.
- [x] LLM APIs and Router are company settings that only an admin saves.
- [x] `TestProjectLinkSameTribeOnly`
- [x] `TestProjectLinkNoSelf`
- [x] `TestProjectLinkOutsiderMisses`
- [x] `TestPackIncludesLinkedRequirements`
- [x] `TestSharedPathConfined`
- [x] `TestReadLinkedFileReadOnly`
- [x] `TestLinkedContextIsUntrusted`
- [x] `TestModelSettingsAdminOnly`

## Sprint 16: Requirements by hand

- [x] A member adds, edits, and deletes a manual requirement. A blank id takes the next free `REQ-` number.
- [x] Ingest keeps manual rows and reports the skipped ids.
- [x] Lead Developer, Senior Developer, QA Lead, and Product Owner members accept. Developer and QA do not.
- [x] UI: Add requirement, edit and delete pages, Manual badge, Accept only when `canAccept`.
- [x] `TestManualRequirementAdd`
- [x] `TestManualRequirementEdit`
- [x] `TestIngestKeepsManual`
- [x] `TestAcceptSeniorRoles`

## Sprint 17: Card owner

- [x] Web ingest records the creator. `create_card` sets `owner_id` and rejects mixed creators.
- [x] The account token test verifies the login with GitHub.
- [x] Commits, push, and the pull request use the owner's identity and token, or the card blocks with `owner_github`.
- [x] Reviewers named on Intake are requested on the pull request.
- [x] The owner cannot approve their own card.
- [x] UI: owner on the board and card page, reviewer picker on Intake, Verified on account GitHub.
- [x] `TestIngestRecordsCreator`
- [x] `TestCardOwnerFromRequirement`
- [x] `TestCreateCardMixedOwners`
- [x] `TestGitHubLoginVerified`
- [x] `TestCommitIdentityOwner`
- [x] `TestPushUsesOwnerToken`
- [x] `TestOwnerGitHubBlocks`
- [x] `TestRequestReviewers`
- [x] `TestReviewersSetByCreator`
- [x] `TestOwnerCannotApprove`
- [x] `MultiSearchSelect.test.ts`

## Sprint 18: Conventions and skills

- [x] An admin saves rules, stacks, and skills with versions and revisions.
- [x] Sync detects a project's stacks and keeps stacks added by hand.
- [x] A Lead Developer adds stacks and enables skills on the project Conventions tab.
- [x] `create_card` attaches enabled skills to a card.
- [x] The implementer and reviewer packs carry the card's rules and skills, and the trace names their versions.
- [x] UI: admin Conventions and Skills pages, project Conventions tab, skills on the card page.
- [x] `TestConventionCatalogAdminOnly`
- [x] `TestConventionRevisions`
- [x] `TestDetectStacks`
- [x] `TestSyncKeepsManualStacks`
- [x] `TestProjectConventionsLeadOnly`
- [x] `TestCreateCardSkills`
- [x] `TestPackConventions`
- [x] `TestConventionsCannotAddTool`
- [x] `TestConventionsContextEvent`
- [x] `TestReviewerPackConventions`
- [x] `TestPromptMarkers` with the new system hash

## Sprint 19: Tribe admins and people on the board

- [x] A tribe admin adds and removes members and grants the tribe admin flag.
- [x] A tribe admin or an admin creates a project with links in one transaction.
- [x] A tribe admin saves a project's links.
- [x] Cards created before migration `00009` get their owner back.
- [x] The card JSON lists reviewers. The board and card page show owner and reviewers.
- [x] UI: Tribes and New project for tribe admins, linked projects on New project, LLM APIs search and filters, Linked projects empty-list hint.
- [x] `TestTribeAdminManagesMembers`
- [x] `TestTribeAdminGrantsTribeAdmin`
- [x] `TestNonTribeAdminCannotManage`
- [x] `TestTribeAdminCreatesProjectWithLinks`
- [x] `TestCreateProjectLinkOtherTribeRollsBack`
- [x] `TestTribeAdminSavesLinks`
- [x] `TestCardReviewers`
- [x] `llm.test.ts`

## Sprint 20: User display names

- [x] An admin names a user on create and edit, and each user renames themselves on Account.
- [x] The user, member, candidate, owner, and reviewer objects carry `name`.
- [x] UI: the board, card page, user menu, Users, tribe members, and reviewer pickers show the name first.
- [x] `TestUserNameRequired`
- [x] `TestAdminRenamesUser`
- [x] `TestOwnName`
- [x] `TestCardPeopleNames`
- [x] `people.test.ts` display name fallback

## Sprint 21: Card context

- [x] The single-card response carries `requirements`, `rules`, and `skills[].useWhen`.
- [x] UI: status and reasons read as words. Board tiles show the owner's avatar and name.
- [x] UI: the card page shows People, Intake, Rules, Skills, and Details.
- [x] `TestCardDetail`
- [x] `status.test.ts`
- [x] Board owner filter by avatar, several owners at a time, replacing Everyone and Mine.
- [x] `board.test.ts` owner cases and `OwnerFilter.test.ts`

## Sprint 22: Settings for admins

- [x] A company admin reads and saves every project's settings and runs Sync, without card access.
- [x] A tribe admin saves every setting of a project in their tribe.
- [x] Settings bodies carry `canEdit`, and the pages lock their fields when it is false.
- [x] `TestAdminEditsProjectSettings`
- [x] `TestTribeAdminEditsProjectSettings`

## Sprint 23: Repository check

- [x] Project create and the repository save ask GitHub with the company token, and store the name GitHub reports.
- [x] UI: the repository is picked from the organization list only.
- [x] `TestProjectRepoCheckedOnGitHub`
- [x] `TestProjectRepoNeedsCompanyToken`
- [x] `TestProjectRepoGitHubDown`
- [x] The repository list adds collaborator repositories of the organization, and Test connection reports what the token sees.
- [x] `TestRepoListIncludesCollaboratorRepos`
- [x] `TestCompanyTestReportsAccess`

## Sprint 24: One card at a time

- [x] A run request names one ready card, and a card has at most one open request.
- [x] The top button queues every ready card whose dependencies are done.
- [x] UI: Run, Queued, and Waiting on N cards on the board tile, and Run on the card page.
- [x] `TestCardRunQueues`
- [x] `TestCardRunNeedsDependencies`
- [x] `TestCardRunNotReady`
- [x] `TestCardRunRole`
- [x] `TestRunReadyCards`
- [x] `BoardCard.test.ts`

## Sprint 25: The worker makes cards

- [x] One card per accepted requirement, made without a model, and backlog cards promoted to Ready.
- [x] `serve` runs one worker loop per project with that project's lock. `run-next` runs the same passes.
- [x] The board polls while a project run row is open.
- [x] `TestPassCreatesCardPerRequirement`
- [x] `TestPassPromotesReady`
- [x] `TestPassMarksRunsProcessed`
- [x] `TestWorkerCreatesCardsAfterAccept`
- [x] `TestProjectLockSkips`
- [x] `TestRunNextPlans`
- [x] `TestBoardRunningWhileProjectRunOpen`

## Sprint 26: Implement job with real models

- [x] `openai` and `anthropic` protocol clients with tool schemas, chosen by the model's family, behind the router's retry and fallback.
- [x] Test connection sends one real request.
- [x] Non-fixture checks run in the pinned image with `podman` or `docker`.
- [x] The implement job runs queued cards from Ready to a pull request, or back to In progress, or blocked with the reason.
- [x] The worker starts queued cards under the cap. `serve` and `run-next` run the job.
- [x] `TestOpenAIToolCall`
- [x] `TestOpenAIError`
- [x] `TestAnthropicToolCall`
- [x] `TestAnthropicBaseURL`
- [x] `TestForModelPicksProtocol`
- [x] `TestFallbackAfterRetry`
- [x] `TestLLMTestCallsProvider`
- [x] `TestLLMTestBadKey`
- [x] `TestLLMTestNoKey`
- [x] `TestExecuteCheckBuildsContainerArgv`
- [x] `TestImplementReachesPROpen`
- [x] `TestImplementCheckFailsReturnsInProgress`
- [x] `TestImplementAttemptCap`
- [x] `TestImplementBlocksSandboxConfig`
- [x] `TestImplementBlocksUnpinnedImage`
- [x] `TestImplementBlocksRouterConfig`
- [x] `TestImplementRefusesOpenAttempt`
- [x] `TestPassStartsQueuedCardsUnderCap`
- [x] `TestPassSkipsUnqueuedReady`
- [x] `TestCardRunNotReady`
- [x] `TestCardRunResumesInProgress`

## Sprint 27: The implementer finds files

- [x] `list_files` and `search_files` read the worktree and nothing outside it.
- [x] A family `baseUrl` pasted with its endpoint works.
- [x] A rate-limited call waits for `retry-after` and is sent again. A provider error keeps its message in `job_error`.
- [x] `TestImplementerTools`
- [x] `TestListAndSearchFiles`
- [x] `TestOpenAIEndpointBaseURL`
- [x] `TestAnthropicBaseURL`
- [x] `TestRateLimitWaitsThenRetries`
- [x] `TestImplementProviderErrorRecordsMessage`
- [x] Each attempt starts from the worktree's last commit.
- [x] `TestDiscardChangesKeepsCommits`
- [x] The model sees its earlier turns, and the history stays under a free-tier request limit.
- [x] `TestHistoryKeepsNewestResults`
- [x] `TestOpenAISendsTurns`
- [x] `TestAnthropicSendsTurns`

## Sprint 28: A card reaches Approved

- [x] The gating export goes through a file, so a large repository exports whole.
- [x] `TestExportLargeArchive`
- [x] A rerun gates an existing commit without a model call, and a stopped sandbox engine blocks with `sandbox_config` before an attempt.
- [x] `TestImplementGatesExistingCommit`
- [x] `TestImplementRepublishesPassedCommit`
- [x] `TestRunnerPathHasOnlyGitDir`
- [x] `TestImplementBlocksWhenEngineDown`
- [x] Leaving `blocked` clears the reason, child rows keep the card's project, and card routes without a project check the role.
- [x] `TestLeavingBlockedClearsReason`
- [x] `TestCardRowsKeepCardProject`
- [x] `TestLegacyCardRoutesCheckRole`
- [x] The review job moves `pr_open` cards to `agent_review` and on, and the worker refreshes cards past review.
- [x] `TestReviewerTools`
- [x] `TestReviewApproveNoCI`
- [x] `TestReviewRequestChanges`
- [x] `TestReviewPendingCIWaits`
- [x] `TestPassFinishesCIWait`
- [x] `TestPassFailedCIRequestsChanges`
- [x] `TestReviewParseFallback`
- [x] `TestReviewWithoutVerdictBlocks` (superseded in Sprint 33 by RFC-069)
- [x] `TestRefreshStoresHeadSHA`
- [x] `TestApprovedHeadMovedReturnsToReview`
- [x] `TestAdvancerSchedulesReviewAndRefresh`
- [x] `TestCardShowsLatestReview`
- [x] `TestCardReachesApproved`
- [x] A real card on `scbd-solid-web` reaches `human_review` with a model review.

## Sprint 29: Pull request text

- [x] The title uses the commit subject when the card title names only requirement ids.
- [x] `TestPRTitleFromCommit`
- [x] The body has summary, requirement, acceptance, changes, verification, and metadata sections, and keeps text a person wrote.
- [x] `TestPRBodySections`
- [x] `TestPRBodyKeepsHumanText`
- [x] `TestImplementReachesPROpen`
- [x] `TestPublishEditsExistingPR`
- [x] The review verdict is a pull request comment, and a failed comment does not change the card.
- [x] `TestReviewPostsComment`
- [x] `TestReviewRequestChanges`
- [x] `TestReviewCommentFailureKeepsStatus`
- [x] Pull request #34 on `scbd-solid-web` shows the new title, body, and review comment.

## Sprint 30: Solo approval

- [x] The Deploy settings save and show `soloApproval`, stored as `approval.solo`.
- [x] With Solo approval on, the owner and the bot login may approve, and the approval stores `independent = false`.
- [x] `TestSoloOwnerApproves`
- [x] `TestOwnerCannotApprove`
- [x] `TestBotApprover`
- [x] `scbd-solid-web` has Solo approval on, and `gilangmahardhika@gmail.com` can approve card REQ-001.

## Sprint 31: Approval on GitHub and the Changes panel

- [x] A new approval posts a pull request comment and a `success` `orchestrator/approval` status. Request changes and Reject on an approved card post `failure`. A failed post leaves the card `approved` with a `job_error`.
- [x] The review job does not count `orchestrator/approval` as a check.
- [x] `GET /api/projects/{projectId}/cards/{id}/diff` returns the card's diff to members, and the card page shows it in the Changes panel.
- [x] `TestApprovalPostsCommentAndStatus`
- [x] `TestApprovalAnnounceFailureKeepsApproved`
- [x] `TestRequestChangesWithdrawsApprovalStatus`
- [x] `TestReviewIgnoresApprovalStatus`
- [x] `TestCardDiff`
- [x] `TestCardDiffRoute`
- [x] `CardChanges` Vitest cases
- [x] Card REQ-001 on `scbd-solid-web` shows its Changes, `resources/js/Pages/Auth/Login.tsx` with `+1 -1`.
- [x] After approving REQ-001, pull request #34 has the approval comment and a green `orchestrator/approval` status on `ae5d441`.

## Sprint 32: Readable card trace

- [x] Migration `00016` adds `events.seq`, and the trace orders by `ts` then `seq`.
- [x] The trace route returns `attempts` with model, tokens, times, outcome, commit, and review.
- [x] `job_error` messages over 8 KiB keep their head and tail, without NUL bytes.
- [x] The Trace page shows milestones, attempt blocks, one-line rows, and clamped long text.
- [x] `TestTraceOrderAndAttempts`
- [x] `TestJobErrorCapped`
- [x] `trace` Vitest cases
- [x] `TraceRow` Vitest cases
- [x] `TraceBlock` Vitest cases
- [x] The REQ-001 trace on `scbd-solid-web` shows Ready -> In progress before -> Blocked, 8 attempt blocks and a review block, and the 1.2 MB error hidden as binary output.

## Sprint 33: Checks decide when the AI review gives no verdict

- [x] Migration `00017` lets `review_verdict` be `failed`.
- [x] A review with no verdict posts "AI review: Not done" and applies the checks instead of blocking.
- [x] An empty check list waits 2 minutes after `pr_open` before it counts as no checks.
- [x] A refresh finishes an `agent_review` card left without a verdict once the card lock is free.
- [x] The card JSON has `reviewVerdict`, and the card page and Trace show a failed review.
- [x] `TestReviewWithoutVerdictUsesChecks`
- [x] `TestReviewWithoutVerdictRedChecks`
- [x] `TestReviewWithoutVerdictWaitsForChecks`
- [x] `TestNoChecksWaitsToRegister`
- [x] `TestChecksInsideWaitDecide`
- [x] `TestRefreshFinishesStoppedReview`
- [x] `TestCardShowsLatestReview` with `reviewVerdict`
- [x] `TraceBlock` Vitest case for a failed review
- [x] Migration `00017` applies on the live board at startup.

## Sprint 34: Self review on a requirement

- [x] Migration `00018` adds `requirements.self_review`.
- [x] Only a Lead Developer or an admin may set Self review, on add or with the self-review route.
- [x] The intake JSON has `selfReview` and `canSetSelfReview`, and the card JSON has `selfReview`.
- [x] A card whose requirements all have Self review lets its owner and the bot login approve, stored as not independent.
- [x] The New requirement page, the Reviewers panel, and the card page show Self review.
- [x] `TestSelfReviewNeedsLead`
- [x] `TestSelfReviewOwnerApproves`
- [x] `TestCardSelfReview`
- [x] `ReviewersPanel` Vitest cases
- [x] Migration `00018` applies on the live board at startup.

## Sprint 35: Markdown statements and a growing requirement form

- [x] `$lib/markdown` and `Markdown.svelte` render a statement with text nodes only, and links only to GitHub or the local harness.
- [x] The requirement form has Write/Preview, and the statement and acceptance boxes start at 8 and 6 rows and grow.
- [x] The intake row renders markdown, and the table cell, card page, and search use `plainText`.
- [x] The statement limit is 10000 characters, stored with LF line endings.
- [x] The pull request body puts the statement under its bold id and closes an open code fence.
- [x] `TestStatementLimitAndLineEndings`
- [x] `TestPRBodyMarkdownStatement`
- [x] `markdown`, `Markdown`, and `RequirementForm` Vitest cases
- [x] On the live server, `/intake/new` grows with a markdown statement and Preview formats it.
- [x] Acceptance lines take inline markdown, and Preview shows them as a checklist beside the statement.
- [x] An acceptance line drops one leading list or checkbox marker in `toLines` and in the API.
- [x] A local link with userinfo after the port, like `http://127.0.0.1:1@host`, stays text.
- [x] `TestAcceptanceLineMarkers`
- [x] On the live server, `/intake/new` previews markdown acceptance lines as a checklist.
- [x] A pasted document with a list saves only its list items, Preview counts the lines left out, and the edit page loads checks as a `- ` list.
- [x] On the live server, a pasted definition of done previews only its list items.

## Sprint 36: Card notes and a reason for Request changes

- [x] The single-card JSON returns the newest 10 failure notes of its project, newest first, and the board list does not.
- [x] The review note is redacted before it is stored.
- [x] Request changes takes an optional reason, stored as the approver note in the same transaction, capped at 4000 characters.
- [x] The card page shows a Notes panel above Next step, with review and approver notes as markdown.
- [x] Request changes opens a reason form.
- [x] `TestCardDetailNotes`
- [x] `TestRequestChangesReason`
- [x] `TestReviewNoteRedacted`
- [x] `CardNotes` and `CardActions` Vitest cases
- [x] On the live server, `card_a91765e0d872` shows the reviewer's note.

## Sprint 37: Retry after a failed attempt, and the AI review mode

- [x] Migration `00019` adds `requirements.ai_review_mode` and the `retry` run trigger.
- [x] `no_commit` and `checks_failed` queue a `retry` row while attempts remain, and block with `attempt_cap` at the cap.
- [x] A card at the cap does not list `run` and is not started.
- [x] An AI `request_changes` on a `continue` card queues the next attempt. On a `manual` card the checks decide and green checks reach Human review.
- [x] Only a Lead Developer or an admin sets the mode, on add or through `PUT .../ai-review-mode`. The intake and card JSON return `aiReviewMode`.
- [x] The Reviewers panel and the New requirement page show the mode, and the card Details show "After AI review: A person reviews".
- [x] `TestRetryAfterNoCommit`
- [x] `TestRetryAfterChecksFailed`
- [x] `TestRunRefusedAtCap`
- [x] `TestReviewContinueQueuesRetry`
- [x] `TestReviewManualReachesHumanReview`
- [x] `TestRefreshSendsBackWhenNoLongerManual`
- [x] `TestCardAIReviewMode`
- [x] `TestAIReviewModeNeedsLead`
- [x] `ReviewersPanel` and `api` Vitest cases
- [x] On the live server, the REQ-002 Reviewers panel shows the mode and the card JSON has `aiReviewMode`.

## Sprint 38: Instructions for the next attempt

- [x] Migration `00020` adds the `instructions` failure note source.
- [x] The implementer pack writes the current instructions in their own block after the newest other note, and builders drop forged block lines.
- [x] `PUT .../cards/{id}/instructions` and Run with `instructions` save them with an `instructions_saved` event, for the roles that may run, on any card that is not `done`.
- [x] The card JSON lists the `instructions` action and returns `instructions` on the single card.
- [x] Run on the card page opens the instructions box. The Instructions panel shows and edits them, and Notes label them.
- [x] `TestPackInstructions`
- [x] `TestAttemptReadsInstructionsAndNote`
- [x] `TestSetInstructions`
- [x] `TestRunWithInstructions`
- [x] `TestInstructionsRoute`
- [x] `TestInstructionsRole`
- [x] `CardActions`, `CardInstructions`, `CardNotes`, `trace`, and `api` Vitest cases
- [x] On the live server, the card page shows the Instructions panel and the Run box.

## Sprint 39: A live, verbose trace

- [x] `trace.Clip` redacts before it cuts, and `job_error` uses it.
- [x] `model_call` stores the reply `text` and `tool_call` stores `args`, both capped, with no file text.
- [x] `JobDeps.Secrets` adds the `databaseUrl` and `tokenKey` text to both jobs' redaction.
- [x] The trace route takes `since` and returns `seq`, `maxTurns`, and `live`.
- [x] The Trace page polls while live, merges rows, shows Live and Now running, and follows the newest row.
- [x] `TestTraceClip`
- [x] `TestModelCallText`
- [x] `TestToolCallArgs`
- [x] `TestTraceSince`
- [x] `TestTraceLive`
- [x] `TestReviewerPackHasNoAuthorText`
- [x] `TestTraceOrderAndAttempts` and `TestJobErrorCapped` still pass
- [x] `trace`, `pager`, `TraceBlock`, and Trace page Vitest cases
- [x] On the live server, the Trace page shows the new fields and does not poll a card that is idle.

## Sprint 40: The reviewer's reply limit

- [x] Reviewer requests set `MaxTokens` to 32,768.
- [x] A reply cut at the limit with no tool call is asked again once, and a second one is `ErrReviewCut`.
- [x] The `job_error` and the "AI review: Not done" comment name the token limit.
- [x] The Anthropic client sends a 32,768-token request with a timeout sized to it, instead of the SDK refusing it as streaming-only.
- [x] `TestReviewMaxTokens`
- [x] `TestAnthropicReviewMaxTokens`
- [x] `TestReviewRetriesCutReply`
- [x] `TestReviewCutTwiceFails`
- [x] The rest of the review suite still passes
- [x] The live server runs the new reviewer limit.

## Sprint 41: Retry an AI review that gave no verdict

- [x] Edge `agent_review -> pr_open` with the attempt and the actor, and no `local_checks` gate.
- [x] `app.RetryReview` moves a card with a `failed` review to `pr_open` and clears the review, without counting an attempt.
- [x] `retry_review` in the card actions for the run roles, the `retry-review` route, and the CLI command.
- [x] "Retry AI review" on the card page.
- [x] `TestEdgesMatchPRD`
- [x] `TestAgentReviewToPROpen`
- [x] `TestRetryReviewQueuesReview`
- [x] `TestRetryReviewNeedsFailedVerdict`
- [x] `TestRetryReviewLocked`
- [x] `TestRetryReviewAction`
- [x] `TestRetryReviewRole`
- [x] `CardActions` retry test
- [x] The live server runs the Sprint 41 build.

## Sprint 42: Red checks retry, and the attempt cap on every send-back

- [x] A red check queues a `retry` run, or blocks with `attempt_cap` when no attempts remain.
- [x] Request changes on a card out of attempts blocks it with `attempt_cap`.
- [x] The worker blocks a `changes_requested` card out of attempts on refresh.
- [x] `TestCIRedQueuesRetry`
- [x] `TestCIRedAtCapBlocks`
- [x] `TestRefreshCapsChangesRequested`
- [x] `TestRequestChangesAtCapBlocks`
- [x] `TestAdvancerRefreshesChangesRequested`
- [x] The rest of the review, gate, and worker suites still pass
- [x] The live server blocks the stuck card with `attempt_cap`, and Unblock is offered.

## Sprint 43: Terminal view on the Trace page

- [x] `terminal.terminalLines` prints each event kind with its full stored text, and marks each attempt.
- [x] `TerminalView` follows new output, folds past 200 lines, renders 300 events at a time, and offers "Jump to latest".
- [x] The Trace page's Structured and Terminal switch in `?view=terminal`, polling every second in the terminal view while live.
- [x] `terminal` Vitest
- [x] `TerminalView` Vitest
- [x] Trace page terminal view Vitest
- [x] `npm run check` and the full Vitest suite pass, and the backend suite still passes
- [x] The live server runs the Sprint 43 build, and a real card's trace from its API renders in `TerminalView`, one line per event plus one per attempt.

## Sprint 44: Thinking and effort per model

- [x] `settings.Model` holds `thinking`, `thinkingBudget`, and `effort`, checked by protocol on save.
- [x] `modelOptions` on `GET` and `PUT /api/settings/llm`.
- [x] `app.RoutedProvider` and "Test connection" send each model's settings through both clients.
- [x] Reasoning goes back to the same model between tool turns, and is not stored.
- [x] Thinking, Budget, and Effort in `FamilyForm`.
- [x] `TestNoModelOptionsSendsNothing`
- [x] `TestSaveLLMModelOptions`
- [x] `TestLLMSettingsModelOptions`
- [x] `TestAnthropicThinking`
- [x] `TestAnthropicReplaysThinking`
- [x] `TestOpenAIEffortAndThinking`
- [x] `TestOpenAIReplaysReasoningContent`
- [x] `TestRoutedProviderModelOptions`
- [x] `llm` model options Vitest
- [x] `FamilyForm` model options Vitest
- [x] `npm run check` and the full Vitest suite pass, and the backend suite still passes

## Sprint 45: Tools that do not destroy files

- [x] The fake provider returns several tool calls per turn and records turns.
- [x] Ranged, numbered `read_file` and `read_linked_file`.
- [x] `replace_in_file`, `delete_file`, `move_file`, and a safe, atomic `edit_file`.
- [x] `search_files` regex, path, glob, ignoreCase, context. `list_files` glob and depth.
- [x] Tool errors flagged, results redacted, `edit_file` text dropped from history.
- [x] Trace and Terminal labels for the new tools.
- [x] `TestReadFileRange`
- [x] `TestReplaceInFileUnique`
- [x] `TestReplaceInFileNotFound`
- [x] `TestEditRefusesPlaceholder`
- [x] `TestHistoryDropsEditText`
- [x] `TestSearchRegexGlobContext`
- [x] `TestListGlobDepth`
- [x] `TestDeleteMoveSafePath`
- [x] `TestToolErrorsFlagged`
- [x] `TestToolResultsRedacted`
- [x] `TestImplementerTools`, `TestToolCallArgs`, `TestHistoryKeepsNewestResults` updated
- [x] Trace and terminal Vitest cases

## Sprint 46: A loop that finishes

- [x] `historyBudgetKiB` in `modelOptions` and `FamilyForm`.
- [x] Stable eviction, re-read dedupe, check output tail.
- [x] Continue nudge, turns-left note in `Turn.Note`, 40-turn default, 200-turn limit.
- [x] Auto-commit after a passing check on the same tree.
- [x] `turn_cap` and `token_cap` with `budget_stop`, in the fixture.
- [x] Infrastructure failures leave `attempts_used`, bad tool JSON returned, `no_change` to the gate.
- [x] `TestHistoryBudgetPerModel`
- [x] `TestHistoryStableEviction`
- [x] `TestRereadUnchanged`
- [x] `TestCheckOutputKeepsTail`
- [x] `TestTextReplyNudged`
- [x] `TestTruncatedReplyNudged`
- [x] `TestAutoCommitAfterPassingCheck`
- [x] `TestNoAutoCommitOnDirtyTree`
- [x] `TestTurnCapStops`
- [x] `TestTokenCapStops`
- [x] `TestInfraErrorKeepsAttempt`
- [x] `TestProviderErrorKeepsAttempt`
- [x] `TestBadToolJSONReturnedToModel`
- [x] `TestNoChangeGoesToGate`
- [x] `FamilyForm` history budget Vitest

## Sprint 47: Pack, prompt, and todo list

- [x] Path map and `AGENTS.md`/`CLAUDE.md` inside the markers, with `repo_context`.
- [x] Requirement headings, acceptance lines, and decisions.
- [x] New `ImplementerSystem`.
- [x] `todo_write` echoed every request.
- [x] Concurrent read-only calls.
- [x] `TestPackPathMap`
- [x] `TestPackRepoMemory`
- [x] `TestPackAcceptanceLines`
- [x] `TestPromptMarkers`
- [x] `TestTodoWriteEchoed`
- [x] `TestTodoWriteCap`
- [x] `TestReadOnlyCallsConcurrent`

## Sprint 48: Prompt caching

- [x] Anthropic cache breakpoints.
- [x] `CachedInputTokens` from both clients, `attempts.tokens_cached`.
- [x] Token cap counts cached reads at a tenth.
- [x] `TestAnthropicCacheBreakpoints`
- [x] `TestCacheUsageRecorded`
- [x] `TestTokenCapCountsCachedAtTenth`
- [x] `TestHistoryPrefixStable`

## Sprint 49: Sandboxed run_command

- [x] `checks.commandExecutables` with shells and wrappers refused.
- [x] `sandbox.ExecuteArgv`, throwaway copy with the export assertion, 30-call budget.
- [x] Refused in fixture mode, never check evidence.
- [x] "Allowed commands" settings field.
- [x] `TestRunCommandParse`
- [x] `TestRunCommandRefusesShell`
- [x] `TestRunCommandFlags`
- [x] `TestRunCommandOnCopy`
- [x] `TestRunCommandDisabled`
- [x] `TestRunCommandBudget`
- [x] `TestRunCommandCannotPassGate`
- [x] `TestRunCommandRedactsOutput`
- [x] `TestRunCommandExportAssertion`
- [x] `TestSaveDeployCommandExecutables`
- [x] Project settings Vitest
- [x] `npm run check` and the full Vitest suite pass, and the backend suite still passes

## Sprint 50: Refine a requirement with AI

- [x] Refine route, `propose_refinement` tool, proposal checks, `requirement_refined` event.
- [x] Refine button, comparison, Apply and Discard on the new and edit pages.
- [x] `TestRefineRequirementProposal`
- [x] `TestRefineDoesNotSave`
- [x] `TestRefineNoImplementRoute`
- [x] `TestRefineInvalidProposal`
- [x] `TestRefineRequiresMember`
- [x] `TestRefinePromptMarkers`
- [x] `TestRefineRedactsKeys`
- [x] `textdiff`, `RefineCompare`, `RequirementForm`, and edit page Vitest
- [x] `npm run check` and the full Vitest suite pass, and the backend suite still passes

## Sprint 51: Refine one field at a time, streamed

- [x] `providers.Streamer` in anthropic, openai, and fake.
- [x] Routed stream falls back only before the first piece of text.
- [x] `app.RefineField`, redact buffer, save rules on the finished text, streamed refine route.
- [x] Spark button and refine box per field, streamed text, Stop, comparison, Apply, Discard.
- [x] `TestAnthropicStream`
- [x] `TestOpenAIStream`
- [x] `TestRoutedStreamFallsBackBeforeFirstToken`
- [x] `TestRoutedStreamNoRetryAfterToken`
- [x] `TestRefineFieldStreams`
- [x] `TestRefineFieldAcceptanceLines`
- [x] `TestRefineFieldRedactsSplitKey`
- [x] `TestRefineFieldErrorsBeforeStream`
- [x] `TestRefineFieldCancel`
- [x] `TestRefinePromptMarkers`
- [x] `streamRefine`, `RefinePanel`, `RequirementForm`, and edit page Vitest
- [x] `npm run check` and the full Vitest suite pass, and the backend suite still passes

## Sprint 52: Stream the card trace

- [x] `live.Hub`, and the stream redactor in `trace`.
- [x] `providers.RestartStreamer` in the routed provider.
- [x] Implementer and reviewer loops stream each turn.
- [x] Trace stream route with the stored trace, live frames, the database read, heartbeat, `done`, and `resync`.
- [x] Trace page reads the stream, falls back to polling, and the terminal view shows the reply being written.
- [x] `TestHubSnapshotAndSlowSubscriber`
- [x] `TestStreamRedactorEverySplit`
- [x] `TestRoutedStreamRestartAfterToken`
- [x] `TestLoopStreamsTurnText`
- [x] `TestReviewStreamsText`
- [x] `TestTraceStreamFrames`
- [x] `TestTraceStreamRedactsSplitSecret`
- [x] `TestTraceStreamOtherProcessEvents`
- [x] `TestTraceStreamRequiresMember`
- [x] `TestTraceStreamCap`
- [x] `streamTrace`, Trace page, and `TerminalView` Vitest
- [x] `npm run check`, the fixture, and the full Vitest and backend suites pass

## Sprint 53: Token usage across projects

- [x] `events (kind, ts)` index and `taskClass` on `model_call`.
- [x] `board.UsageRows` and `app.Usage` with the family from `config.yaml`.
- [x] `GET /api/usage` (admin only) and `orchestrator usage`.
- [x] `/admin/usage` with range, filters, Group by, sort, totals, and daily bars, and the Admin nav item.
- [x] `TestUsageAggregates`
- [x] `TestUsageTaskClass`
- [x] `TestUsageUser`
- [x] `TestUsageFamily`
- [x] `TestUsageAdminOnly`
- [x] `TestUsageRange`
- [x] `TestModelCallTaskClass`
- [x] `TestUsageCLI`
- [x] `usage` lib, Token usage page, and nav Vitest
- [x] `npm run check`, the fixture, and the full Vitest and backend suites pass

## Sprint 54: Token usage for a tribe and for each member

- [x] `board.UsageScope` for a tribe or a user, and a card's call counted in the card's project.
- [x] `app.TribeUsage` and `app.MyUsage`, `GET /api/tribes/{id}/usage`, `GET /api/me/usage`, and `orchestrator usage --tribe --user`.
- [x] `UsageDashboard` on `/admin/usage`, `/tribes/{id}/usage`, and `/account/usage`, with the links.
- [x] `TestUsageTribeScope`
- [x] `TestUsageSelfScope`
- [x] `TestTribeUsageAccess`
- [x] `TestMyUsage`
- [x] `TestUsageCLIScope`
- [x] `usage` lib, tribe and account usage page, admin usage page, and menu Vitest
- [x] `npm run check`, the fixture, and the full Vitest and backend suites pass

## Sprint 55: Token usage by source and card

- [x] Usage rows carry `source`, `cardId`, and `cardTitle`, grouped by card.
- [x] `GroupUsage` and `orchestrator usage --by` take `source` and `card`.
- [x] Source and Card filters and Group by on every usage page.
- [x] `TestUsageSourceAndCard`
- [x] `TestUsageCLICard`
- [x] `usage` lib, admin usage page, and account usage page Vitest
- [x] `npm run check`, the fixture, and the full Vitest and backend suites pass

## Sprint 56: Tribe MCP servers, settings

- [x] `tribe_mcp_servers` and `tribe_mcp_tools` store servers, sealed secrets, and the allowlist.
- [x] `netguard` and `mcp.DialHTTP` refuse internal addresses and cap responses.
- [x] Tribe MCP routes and pages, checked like the other tribe routes.
- [x] `TestMCPServerSecretsNeverReturned`
- [x] `TestMCPServerAccess`
- [x] `TestMCPSlugAndToolNames`
- [x] `TestMCPNameCollision`
- [x] `TestNetguardRefusesPrivate`
- [x] `TestNetguardRedirectAndProxy`
- [x] `TestMCPHTTPListAndCall`
- [x] `TestMCPResultCap`
- [x] `TestMCPDiscoverNoBody`
- [x] Tribe MCP pages Vitest
- [x] `npm run check`, the fixture, and the full Vitest and backend suites pass

## Sprint 57: MCP tools in the implementer loop

- [x] Frozen `mcp_` tools after the constant list, with caps and wrapped, redacted results.
- [x] `ImplementCard` loads the tribe's servers once per attempt and records `mcp_context`.
- [x] `TestImplementerMCPTools`
- [x] `TestMCPResultWrappedAndStripped`
- [x] `TestMCPHeaderRedacted`
- [x] `TestMCPCallCap`
- [x] `TestMCPToolListChanged`
- [x] `TestMCPCannotMoveCard`
- [x] `TestMCPNoLiveFrame`
- [x] `TestMCPOtherTribe`
- [x] `TestAnthropicSchemaExtraFields`
- [x] Updated `TestImplementerTools` and `TestPromptMarkers`
- [x] Trace Vitest
- [x] `npm run check`, the fixture, and the full Vitest and backend suites pass

## Sprint 58: Stdio MCP servers in the sandbox

- [x] `execx.Start` and `sandbox.StartArgv` run a stdio server with no network and remove it on close.
- [x] `TestStdioNoNetworkNoArgvSecret`
- [x] `TestStdioRefusesShell`
- [x] `TestStdioContainerRemoved`
- [x] `TestStdioFixtureRefused`
- [x] `TestStdioListAndCall`
- [x] The fixture and the full backend suite pass

## Sprint 59: Refine with MCP tools

- [x] Refine runs at most 3 turns and 3 calls with the tribe's tools, and the last turn has none.
- [x] `TestRefineNoServersNoTools`
- [x] `TestRefineMCPLoop`
- [x] `TestRefineMCPResetEvent`
- [x] `TestRefineLastTurnNoTools`
- [x] `TestRefineMCPCancel`
- [x] `RefinePanel` reset Vitest
- [x] `npm run check`, the fixture, and the full Vitest and backend suites pass

## Sprint 60: Database connectors

- [x] `postgres` and `sqlserver` transports with `config_json` and a write-only password.
- [x] `dbtools` read-only tools with a row cap that refuse the board database.
- [x] `TestDBConfigValidation`
- [x] `TestPostgresToolReadOnly`
- [x] `TestPostgresSingleStatement`
- [x] `TestPostgresRowCap`
- [x] `TestDBConnectorRefusesBoard`
- [x] `TestDBPasswordRedacted`
- [x] `TestImplementerDBTools`
- [x] `TestSQLServerStatementCheck`
- [x] `TestSQLServerLive` (runs when `ORCH_TEST_MSSQL` is set)
- [x] The fixture and the full backend suite pass

## Sprint 61: MCP catalog API and image pull

- [x] The catalog lists built-in entries and the MCP Registry, cached, with a notice when the registry is down.
- [x] An image pull returns the image pinned by digest.
- [x] `TestMCPCatalogBuiltins`
- [x] `TestMCPCatalogRegistry`
- [x] `TestMCPCatalogRegistryDown`
- [x] `TestMCPCatalogAccess`
- [x] `TestMCPImagePull`
- [x] The fixture and the full backend suite pass

## Sprint 62: MCP catalog page

- [x] `/tribes/{id}/mcp/catalog` searches the catalog and opens a filled form.
- [x] The form has database fields, image pull, and discovery after a create.
- [x] Catalog page, database form, registry prefill, image pull, and auto-discover Vitest
- [x] `npm run check`, the fixture, and the full Vitest and backend suites pass

## Sprint 63: Featured MCP servers and a catalog that answers at once

- [x] The catalog route takes `source`, and featured entries resolve by id.
- [x] The registry fetch is apart, bounded, and remembers failures; Set up reads a registry entry by exact name.
- [x] `TestMCPCatalogBuiltins`
- [x] `TestMCPCatalogFeatured`
- [x] `TestMCPCatalogRegistrySlow`
- [x] `TestMCPCatalogEntryByName`
- [x] `TestMCPCatalogRegistryDown`
- [x] `TestMCPCatalogAccess`
- [x] Catalog page Vitest: featured and built-in before the registry, abort on typing, registry notice
- [x] `npm run check`, the fixture, and the full Vitest and backend suites pass

## Sprint 64: MCP mentions in requirements, harness

- [x] Mentions are read from the markdown tree and stored with ingested and manual requirements.
- [x] The intake response lists each requirement's mentions and the tribe's slugs.
- [x] Jobs and refine offer only mentioned servers, fail closed, and record the scope; the pull request body escapes mentions.
- [x] `TestMentionGrammar`
- [x] `TestIngestMentions`
- [x] `TestHashUnchangedWithoutMentions`
- [x] `TestManualRequirementMentions`
- [x] `TestIntakeMCPServers`
- [x] `TestImplementerMCPMentions`
- [x] `TestImplementerMCPUnion`
- [x] `TestImplementerMCPMentionMissing`
- [x] `TestImplementerMCPUnaccepted`
- [x] `TestRefineMCPMentions`
- [x] `TestPRBodyMentionsEscaped`
- [x] `TestImplementerMCPTools` and `TestMCPOtherTribe` pass unchanged
- [x] The fixture and the full backend suite pass

## Sprint 65: MCP mentions in requirements, pages

- [x] Typing `@` in a requirement field lists the tribe's slugs and inserts the one picked.
- [x] Mentions show under each field, on the intake row, and on the detail page; the trace shows the scope.
- [x] Mention helper Vitest
- [x] `RequirementForm` autocomplete and chips Vitest
- [x] Intake pages Vitest: mentions on the detail page, servers passed to the new requirement form
- [x] Trace Vitest: scope `none` and missing mentions
- [x] `npm run check`, the fixture, and the full Vitest and backend suites pass

## Sprint 66: Tribe selector after sign-in

- [x] Sign-in goes to `/choose-tribe`, and picking a tribe stores it with its first project and opens the board.
- [x] A project route with no stored tribe redirects to `/choose-tribe`.
- [x] The switcher lists only the selected tribe's projects, with "Switch tribe" and `Tribe / Project` in the top bar.
- [x] Tribe selector Vitest
- [x] Layout tribe scope Vitest
- [x] Layout load and sign-in destination Vitest
- [x] The Projects page lists only the active tribe's projects, for an admin too, and New project starts on the active tribe.
- [x] Projects by tribe Vitest
- [x] `npm run check`, the fixture, and the full Vitest and backend suites pass

## Sprint 67: Reviewers before GitHub verification

- [x] `SetReviewers` accepts an enabled tribe member without a verified login, and `reviewerOptions` lists them.
- [x] The reviewer request asks GitHub only for verified logins, and `reviewers_requested` lists the others as `waiting`.
- [x] The picker labels an unverified member and explains when GitHub asks them; the trace shows who is waiting.
- [x] `TestReviewersSetByCreator`, `TestReviewerOptionsUnverified`, `TestRequestReviewers`, `TestRequestReviewersOnlyWaiting`
- [x] Reviewer picker, `ReviewersPanel`, and trace Vitest
- [x] `npm run check`, the fixture, and the full Vitest and backend suites pass

## Sprint 68: Tribe and Project sections

- [x] The sidebar shows "Tribe · {name}" and "Project · {name}" groups with their links.
- [x] MCP servers and Members show only to a tribe admin or an admin, and the Project group hides with no project.
- [x] The settings sub-nav is removed, and the mobile menu shows the headings.
- [x] Tribe and Project sections Vitest
- [x] `npm run check` and the full Vitest suite pass

## Sprint 69: Layered conventions

- [x] Migration `00027` adds the tribe, project, and card layers and keeps existing skills and manual stacks.
- [x] `conventions.Effective` resolves the layers with origins; the pack and `create_card` use it.
- [x] Tribe conventions route and page; project conventions take on/off choices.
- [x] `TestTribeConventionsSave`, `TestEffectiveConventions`, `TestProjectConventionsMigration`, `TestPackUsesEffective`
- [x] Conventions pages Vitest
- [x] `npm run check`, the fixture, and the full Vitest and backend suites pass

## Sprint 70: Card context

- [x] `PUT .../cards/{id}/context` with the `context` and `attach-mcp` actions and role checks.
- [x] The pack and the MCP scope apply the card's context; `mcp_context` records `card`, `attached`, and `detached`.
- [x] `context_changed` is appended and the card status never changes.
- [x] `TestCardContextRoles`, `TestCardContextDone409`, `TestCardContextScoped`, `TestCardContextPack`, `TestMCPScopeCardAttach`, `TestCardContextDisabledServer`
- [x] The fixture and the full backend suite pass

## Sprint 71: Card context panel

- [x] The card page's Context panel lists rules, skills, and MCP servers with origins and controls from the actions.
- [x] The trace shows `context_changed` and the `card` MCP scope.
- [x] Card context Vitest
- [x] `npm run check` and the full Vitest suite pass

