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
- [x] `TestReviewWithoutVerdictBlocks`
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
