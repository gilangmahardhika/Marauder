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
- [x] `TestAcceptByOwner`
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
