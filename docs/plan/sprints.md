# Sprint plan

Status: Accepted for implementation. Date: 2026-10-10.

This is the build schedule. It follows the order in [../tech/0003-backend.md](../tech/0003-backend.md) (steps 1 to 12, then the frontend, then steps 13 to 15). Tests are the ones named in [../tech/0005-verification.md](../tech/0005-verification.md) and [../tech/0006-security-and-harness.md](../tech/0006-security-and-harness.md). Contract fields come from [../tech/0002-data-and-api.md](../tech/0002-data-and-api.md). The UI comes from [../tech/0004-frontend.md](../tech/0004-frontend.md).

If this file disagrees with a PRD acceptance check, an RFC statement, or a tech guide, the other document wins. See [../README.md](../README.md).

A sprint is done when its tests pass. The next sprint does not start on a failing suite. Live Anthropic or OpenAI keys are not required. The fake provider and the health stub cover the release bar.

Progress is tracked in [checklist.md](checklist.md).

```mermaid
flowchart LR
  s1[Sprint1_Board]
  s2[Sprint2_Intake]
  s3[Sprint3_Runtime]
  s4[Sprint4_Implementer]
  s5[Sprint5_Review]
  s6[Sprint6_Approval]
  s7[Sprint7_Deploy]
  s8[Sprint8_Worker]
  s9[Sprint9_API]
  s10[Sprint10_UI]
  s11[Sprint11_Accounts]
  s12[Sprint12_Tribes]
  s13[Sprint13_Projects]
  s14[Sprint14_CompanyRepos]
  s15[Sprint15_LinkedProjects]
  s16[Sprint16_ManualIntake]
  s17[Sprint17_CardOwner]
  s18[Sprint18_Conventions]
  s19[Sprint19_TribeAdmin]
  s20[Sprint20_UserNames]
  s21[Sprint21_CardContext]
  s22[Sprint22_SettingsForAdmins]
  s23[Sprint23_RepositoryCheck]
  s1 --> s2 --> s3 --> s4 --> s5 --> s6 --> s7 --> s8 --> s9 --> s10 --> s11 --> s12 --> s13 --> s14 --> s15 --> s16 --> s17 --> s18 --> s19 --> s20 --> s21 --> s22 --> s23
```

Sprint 21 implements display and read fields under RFC-054, RFC-055, and RFC-056. It adds no decision.

Sprint 22 implements [../rfc/0013-project-settings-admins.md](../rfc/0013-project-settings-admins.md).

Sprint 23 implements [../rfc/0014-repository-check.md](../rfc/0014-repository-check.md).

Sprint 20 implements [../rfc/0012-user-names.md](../rfc/0012-user-names.md).

Sprint 19 implements [../rfc/0011-tribe-admin.md](../rfc/0011-tribe-admin.md).

Sprint 16 implements [../rfc/0008-manual-requirements.md](../rfc/0008-manual-requirements.md).

Sprints 14 and 15 implement [../rfc/0006-company-github-and-linked-projects.md](../rfc/0006-company-github-and-linked-projects.md).

Sprints 11–13 implement [../rfc/0005-teams-and-cloud.md](../rfc/0005-teams-and-cloud.md). Do not start Sprint 11 while a required Sprint 10 test fails. The optional real-host box does not block Sprint 11. The REQ-043 fixture stays one project. New tests add a second project beside that run.

## Sprint 1: The board cannot skip a gate

Goal: Card status changes only through compare-and-swap, the event log is insert-only, and the data directory cannot sit inside the target repo.

Backend step 1. Packages: `internal/machine`, `internal/trace`, `internal/board`, `cmd/orchestrator` startup.

- Create `backend/` as module `orchestrator/backend` on Go 1.26, with goose migration `00001_init.sql` (postgres dialect) from the schema in the data contract. The board is PostgreSQL 16 or newer through pgx. There is no SQLite file.
- `go test` needs `ORCHESTRATOR_TEST_DATABASE_URL`. A missing variable fails the test with that name before any card is written. Tests do not use testcontainers. Each test migrates that database and isolates its rows.
- Implement `Transition`: the REQ-006 edge map, evidence checks, and compare-and-swap (RFC-040). `done` has no outbound edge.
- Append events with redaction. Triggers reject `UPDATE` and `DELETE` on `events` (RFC-031).
- Startup refuses `--data` inside the repo or the worktree (RFC-036) and refuses a non-loopback `--addr` (RFC-032).

Done when: `TestEdgesMatchPRD`, `TestIllegalDeploySkip`, `TestEvidenceRequired`, `TestTransitionCAS`, `TestRedact`, `TestEventsImmutable`, `TestDataDirInsideRepo`, `TestLoopbackOnly`.

## Sprint 2: Nothing starts until a person accepts the ids

Goal: A PRD and its RFCs become requirement and decision records, and `run-next` creates no worktree before acceptance.

Backend step 2. Package: `internal/intake`, accept in `internal/app`.

- Parse markdown with goldmark. Keep existing `REQ-` and `RFC-` ids. A file with no ids sets `draftPending` and stores drafted rows as unaccepted.
- Record operator acceptance. Re-parse clears acceptance only for changed ids (RFC-019).
- CLI: `ingest`, `accept`, and `status` on an empty board (the empty-board and missing-`--sha` CLI checks).

Done when: `TestDraftIds`, and `status` on an empty data dir prints `board is empty` and `orchestrator ingest`. No worktree package exists yet.

## Sprint 3: Checks cannot rewrite the worktree

Goal: Git can open one worktree per card, and a check runs on an exported copy with a scrubbed argv. No model is called.

Backend step 3. Packages: `internal/execx`, `internal/sandbox`, `internal/runtime`. Fixture repo under `backend/testdata/fixture-repo`.

- `execChild`: argv only, built env, timeout, output cap. Refuse `sh` and `bash` (RFC-027).
- Sandbox argv: `--network=none` for local checks, digest pin, export mount, no Docker socket, no worktree path (RFC-028, RFC-037). Dev-check argv is specified here but executed in Sprint 7.
- Worktree and commit identity without a model. Git runs with hooks off, `core.fsmonitor` off, and a private `GIT_CONFIG_GLOBAL` (RFC-033).
- Path rule: reject `../`, absolute paths, `.git/config`, and symlinks that leave the worktree.

Done when: `TestWorktreeOnce`, `TestCheckExit`, `TestCommandAllowlist`, `TestEnvScrub`, `TestCheckOnExport`, `TestPathEscape`, `TestSandboxFlags`, `TestNoShell`, `TestHookNotRun`, `TestFsmonitorOff`, `TestTarEscape`, `TestCheckParse`.

## Sprint 4: A scripted implementer can commit

Goal: One fake model edits the fixture repo and commits. Exit code 0 is the only way a check passes. The implementer has no deploy tool.

Backend step 4. Packages: `internal/providers/fake`, `internal/contextpack` implementer pack, tool loop.

- Fake provider replays scripts by task class and attempt. It records the system and user strings.
- Tool loop stops at `maxTurns`. Unknown tools and a scripted `deploy` call are rejected and do not exec.
- `create_card` cannot set the check command. Checks come from settings.
- No commit counts as an implementer attempt and writes a `no_commit` note. `run_local_check` inside the attempt does not move status.
- Prompt packs wrap untrusted text in the markers from the security guide. Commit trailers are `Orchestrator-Card`, `Orchestrator-Attempt`, and `Model`.

Done when: `TestImplementerTools`, `TestToolNotInProfile`, `TestNoCommitAttempt`, `TestToolCheckDoesNotMove`, `TestCommitIdentity`, `TestPromptMarkers`, `TestCreateCardIgnoresCommand`.

## Sprint 5: One pull request, then a cold review

Goal: A card has one pull request on `orch/<card-id>`. The reviewer is a different provider family when two are configured, and its prompt has the diff and not the author transcript.

Backend steps 5 and 6. Packages: `internal/vcs`, reviewer pack, router family choice.

- Ensure-PR is idempotent on card id. Metadata block is rewritten on each push. The summary block stays. Push uses the settings URL and the card refspec only, with the token in askpass, not in argv (RFC-034).
- Reviewer verdicts: `request_changes` to `changes_requested`, `recommend_approve` plus green required CI to `human_review`. Pending CI stays in `agent_review`. Red CI goes to `changes_requested` with a `ci` note.
- Review and triage do not increment `attempts_used` (RFC-042). `review_independent` is stored and is false when only one family is configured (RFC-008).

Done when: `TestPROnce`, `TestMetadataRewrite`, `TestPushURL`, `TestAskpass`, `TestReviewerPack`, `TestReviewNotCounted`, `TestCIPending`, `TestCIRed`, `TestRouteClass`, `TestNoDeployClass`.

## Sprint 6: Approval is a person and a SHA

Goal: Only an allowlisted login can approve the current head. A later commit, a request for changes, or a reject clears that approval and does not deploy.

Backend step 7. Human gates in `internal/app` and the CLI.

- `approve --sha`, `request-changes`, `reject`, `unblock`. Missing or wrong SHA exits 1 and writes no approval row.
- Same SHA twice inserts one approval. Bot login cannot be an approver.
- Unblock destinations follow RFC-041: budget and reject go to `ready`; `health`, `deploy_failed`, and `deploy_config` go to `human_review`. `source_changed` stays blocked while a linked id is unaccepted. Default release at the cap raises `max_attempts` by one. `--reset-attempts` zeros `attempts_used`.

Done when: `TestApprovedHeadMoved`, `TestRequestChangesFromApproved`, `TestRejectFromApproved`, `TestApproveSha`, `TestApproveIdempotent`, `TestStaleSHA`, `TestUnblockDestinations`, `TestUnblockAtCap`, `TestUnblockReset`, `TestUnblockSourceChanged`, `TestCapFromChangesRequested`, plus the CLI approve and unblock checks.

## Sprint 7: Deploy the approved SHA, then test it

Goal: Deploy and health run only for the approved SHA. A failed probe rolls back and blocks. Dev tests run after health, on an export of that SHA. A crash mid-deploy reconciles before a second deploy.

Backend step 8. Packages: `internal/deploy`, `internal/verify`. Stub health server and `testdata/bin/deploy-stub`.

- Deploy and rollback commands are absolute paths outside the repo and the data directory, mode `0700` or `0500` (RFC-038). The SHA argument is 40 or 64 hex only.
- Probe allows a listed dev host, refuses link-local, loopback outside fixture mode, and unlisted hosts. A redirect is a failure (RFC-039).
- Empty `dev_check` plus a healthy probe is `health_only` and `done`. A failing dev check returns to `changes_requested` on the same PR and stores the triage note.
- Reconcile: deploy event without health probes or rolls back before another deploy exec. Commit without a gating check reruns the check and does not call the model.

Done when: `TestDeployFailed`, `TestHealthFail`, `TestHealthOnly`, `TestDevFailThenPass`, `TestVerifyAtDeployedSHA`, `TestDeployCommandPath`, `TestSHAArg`, `TestHealthLinkLocal`, `TestHealthLoopback`, `TestHealthPrivateListed`, `TestHealthUnlisted`, `TestHealthRedirect`, `TestReconcileDeploy`, `TestReconcileCommit`, `TestDevCheckNetwork`.

## Sprint 8: One worker runs the board

Goal: Settings and secrets live in the data directory, route changes apply to the next job only, and a single worker advances ready cards without two deploys at once.

Backend steps 9 and 10. Packages: `internal/settings`, `internal/router`, `internal/worker`.

- `config.yaml` and mode `0600` `secrets.json`. GET never returns a token. PUT without a secret field keeps the stored bytes. Short secrets and unknown model ids are `422`. A config change event names fields, not values (RFC-030).
- Router: retry primary once, then one fallback, then `blocked` with `provider_error`. Empty route blocks with `router_config`. A save during a running attempt does not change that attempt's model.
- Worker lock file. A second `serve` exits. Plan job creates cards only for accepted ids and refuses a dependency cycle. Implementation cap and the global deploy lock are enforced. A moved base ref blocks with `base_moved`.

Done when: `TestSecretOmitted`, `TestSecretKeep`, `TestShortSecret`, `TestUnknownModel`, `TestModelIdUnique`, `TestBotApprover`, `TestConfigEvent`, `TestSnapshot`, `TestRouterConfig`, `TestFallbackOnce`, `TestPlanCards`, `TestWorkerLock`, `TestDependency`, `TestDeployLock`, `TestCap`, `TestBaseRefUnchanged`, `TestLockHolder`.

## Sprint 9: The fixture script and the JSON API

Goal: `go run ./cmd/fixture` exits 0 on the nine REQ-043 steps, and every route in the data contract uses the same service functions as the CLI.

Backend steps 11 and 12. `cmd/fixture`, `internal/httpapi`.

- Fixture runner uses a temp data directory, copies the deploy stub outside the repo, replays the fake provider for two families, and requires a clean `git status` in the fixture repo (REQ-045).
- HTTP: bearer session, loopback `Host` check, CORS only for `http://127.0.0.1:4173`, 1 MiB body cap, the error shape from the contract. Card JSON includes server-computed `group`, `actions`, and `unblockTo`.
- Session lockout after 5 failures. Wrong `Origin` and evil `Host` are `403`.

Done when: `go run ./cmd/fixture` exits 0, and `TestActionsEmpty`, `TestActionsApproved`, `TestCORS`, `TestHostHeader`, `TestSessionLockout`, `TestRedactPack`, `TestSandboxOffRejected`, `TestAskpass` still pass inside `go test ./...`.

## Sprint 10: The operator UI and the release pass

Goal: A local SvelteKit app can log in, run the board, accept requirements, approve a SHA, and save GitHub, model, router, and deploy settings. Buttons come only from `actions`. The release checklist in the verification guide passes.

Frontend from [../tech/0004-frontend.md](../tech/0004-frontend.md), after Sprint 9. Node is the Vite toolchain only. No `+server.ts` and no React.

- App shell: client-only SvelteKit, `adapter-static`, CSP meta, session bearer in `sessionStorage`, `src/lib/api.ts` covering every route.
- Work pages: `/login`, `/` (six groups, Run, 3s poll), `/intake`, `/cards/[id]`, `/cards/[id]/trace`. `CardActions` renders only names in `actions`.
- Settings pages: GitHub, LLM APIs, router, deploy. `SecretField` omits an empty secret and never prints a token.
- Styles: system font, one `.primary` button, `.danger` only for blocked, 44px controls, visible focus, one column under 720px.

Done when, in order:

- `npm run check`, `npm run test`, `npm run build`, and `npm audit --omit=dev` from `frontend/`.
- Vitest: `CardActions` empty, approved, and `unblockTo: null`; `SecretField` omits the secret; `api` clears the bearer on 401; built `index.html` has the CSP meta and no `unsafe-inline`.
- Manual browser pass on fixture cards, from the table in the verification guide.
- Optional, after that pass: one real dev host using the same deploy code, with secrets only in `secrets.json`. That run is recorded by hand and is not part of `go test`.

## Sprint 11: A person is an account

Goal: Email and password replace the printed UI token. An admin can create a user. Each user stores their own GitHub host, login, and token. The board may still be the one existing project.

Backend and the login and account pages. Packages: a new identity store used by `internal/httpapi` and `internal/app`, migration `00003` for `users`, `sessions`, and `user_github`.

- Store passwords as argon2id hashes. Reject a password shorter than 12 characters.
- `POST /api/session` takes email and password and returns a bearer bound to that user. The row stores the bearer hash. Expiry is 12 hours and is checked on every request. `DELETE /api/session` drops that bearer.
- Five bad passwords for one email pause that email for one minute. A different email is not paused.
- `orchestrator users bootstrap` creates the first admin from `ORCHESTRATOR_ADMIN_EMAIL` and `ORCHESTRATOR_ADMIN_PASSWORD` when `users` is empty. The last admin cannot be removed or disabled.
- Admin `POST /api/users` and `PUT /api/users/{id}`. A non-admin receives `403`.
- `GET` and `PUT /api/me/github` and `POST /api/me/github/test`. The token is ciphertext under `tokenKey` from the service secrets file. GET returns `tokenSet` only.
- Login page: email and password. Account page: password change and GitHub settings. `sessionStorage` holds the bearer and no secret.

Done when: `TestPasswordLogin`, `TestWrongPassword`, `TestSessionLockout`, `TestBearerExpiry`, `TestLastAdmin`, `TestUserGitHubOmitted`, `TestNonAdminForbidden`.

## Sprint 12: A tribe hides its projects

Goal: An admin creates tribes, adds members, and creates projects. A member sees that tribe's projects. Anyone else receives `404`, including for an unknown project id.

Migration for `tribes`, `tribe_members`, `projects`, and `project_id` on board rows. Per-project `intake_state` and `deploy_lock`. Unique `(project_id, id)` for requirements and decisions. Unique `(project_id, branch)` and `(project_id, pr_url)`.

- `GET` and `POST /api/tribes`, member add and remove. Non-admin is `403`.
- `GET` and `POST /api/projects`. Admin receives every project record. Card and intake routes still require membership.
- Board, intake, run, and card routes move under `/api/projects/{projectId}/`.
- Project switcher in the layout stores `orchestrator.project` and no secret.
- The same `REQ-001` and the same branch name may exist on two projects.

Done when: `TestMemberSeesProject`, `TestOutsiderMissesProject`, `TestReqIdsPerProject`, `TestBranchPerProject`, `TestAdminManagesMembership`.

## Sprint 13: Work follows the project

Goal: Each project has its own data directory, settings, bot token, worker lock, and deploy lock. Two projects may implement and deploy at the same time. Inside one project, one deploy runs at a time. `approved` does not deploy until a Lead Developer member starts it.

- Project directory `<data>/projects/<project-id>/` outside that repo, with `config.yaml`, project secrets, `worker.lock`, and worktrees.
- Project GitHub settings are owner, repo, base branch, bot login, and bot token. Push and pull requests use the bot token. They do not use the signed-in user's token.
- `actions` is the status list filtered by RFC-046. Accept is Product Owner only. Deploy is Lead Developer only. A Developer can start a run and cannot approve or deploy.
- New transition events include `actorId`. Old events are not updated.
- `ORCHESTRATOR_CLOUD=1` may bind a non-loopback address. Without that variable, `TestLoopbackOnly` still passes.
- Reconcile and the deploy lock stay inside one project.

Done when: `TestParallelProjects`, `TestLeadDeploy`, `TestOwnerApproveStops`, `TestAcceptByOwner` (replaced by `TestAcceptSeniorRoles` in Sprint 16), `TestDeveloperRunOnly`, `TestActorOnTransition`, `TestCloudBind`, and `TestLoopbackOnly` still passes without `ORCHESTRATOR_CLOUD`.

## Sprint 14: One company account, one repo per project

Goal: An admin sets one company GitHub account. Each project names one repository in that organization, and the service clones it. Deploy, checks, sandbox, and doc paths are saved per project.

Migration `00006` creates `workspace_github`, drops the per-project host, owner, bot login, and bot token, and makes `github_repo` unique. Packages: `internal/company`, `internal/tenancy`, `internal/runtime`, `internal/settings`, `internal/intake`.

- `GET` and `PUT /api/settings/github`, `POST /api/settings/github/test`, and `GET /api/settings/github/repos`. Admin only, except the repo list, which a Lead Developer may call. The token is ciphertext and never returned. `gh` gets it in the child environment.
- `POST /api/projects` takes `repo` and `baseBranch`. A repo another project names is `422`.
- `GET` and `PUT /api/projects/{id}/settings/repository` and `POST /api/projects/{id}/sync`. Sync clones into `<data>/projects/<id>/repo` or fetches the base branch, with the token through askpass.
- Worktrees branch from the project base branch. Intake, accept, and the loop's requirement reads include `project_id`. Ingest reads the project clone.
- `GET` and `PUT /api/projects/{id}/settings/deploy` read and write that project's `config.yaml` and `secrets.json`, atomically. The service-wide deploy route and operator login are removed.
- UI: Company GitHub under Admin, the repository picker on New project, the Repository page with Sync, and Deploy on the project route.

Done when: `TestCompanyGitHubTokenNeverReturned`, `TestCompanyGitHubAdminOnly`, `TestRepoListUsesCompanyToken`, `TestProjectRepoUnique`, `TestCloneTokenNotInArgv`, `TestCheckoutUsesBaseBranch`, `TestIntakePerProject`, `TestProjectDeploySettingsIsolated`, `TestProjectDeployTokenNeverReturned`, `TestDeployLeadOnly`.

## Sprint 15: A project reads its neighbours

Goal: A Lead Developer links their project to other projects in the same tribe. The implementer and reviewer see the linked requirements, decisions, and shared files, and the implementer can read any linked file. Nothing writes to a linked repository.

Migration `00007` creates `project_links`. Packages: `internal/tenancy`, `internal/contextpack`, `internal/loop`, `internal/runtime`.

- `GET` and `PUT /api/projects/{id}/settings/links`. Same tribe only, no self link, shared paths checked like a tool path.
- The implementer and reviewer packs add a `LINKED` block per link inside the untrusted markers, capped per link.
- `read_linked_file` reads from an export of the linked base SHA through `safePath`. The tool list snapshot changes in this sprint.
- One event per attempt lists the linked project ids and SHAs.
- UI: the Linked projects page.
- LLM APIs and Router are company settings (RFC-052). Only an admin saves them or runs a family test. UI: they move under Admin.

Done when: `TestProjectLinkSameTribeOnly`, `TestProjectLinkNoSelf`, `TestProjectLinkOutsiderMisses`, `TestPackIncludesLinkedRequirements`, `TestSharedPathConfined`, `TestReadLinkedFileReadOnly`, `TestLinkedContextIsUntrusted`, `TestModelSettingsAdminOnly`.

## Sprint 16: Requirements by hand

Goal: A member adds a requirement to a project without a document in the repository. Every senior role can accept. Ingest never overwrites a requirement added by hand.

Migration `00008` adds `manual` to `requirements.source_kind` and adds `requirements.created_by`. Packages: `internal/intake`, `internal/app`, `internal/httpapi`.

- `POST /api/projects/{id}/requirements`, and `PUT` and `DELETE /api/projects/{id}/requirements/{reqId}`. Any member. A blank id takes the next free `REQ-` number. Only manual rows change. A row a card lists is not deleted.
- Ingest skips ids that are manual and returns them in `skipped`.
- Accept is allowed for Lead Developer, Senior Developer, QA Lead, and Product Owner members. `TestAcceptSeniorRoles` replaces `TestAcceptByOwner`.
- The intake response adds `canAccept` and `createdBy`.
- UI: Add requirement, `/intake/new`, `/intake/[id]`, a Manual badge, and the Accept button only when `canAccept`.

Done when: `TestManualRequirementAdd`, `TestManualRequirementEdit`, `TestIngestKeepsManual`, `TestAcceptSeniorRoles`.

## Sprint 17: Card owner

Goal: A card belongs to the member who brought its requirements in. Its commits and pull request are theirs, the reviewers they named are requested, and they cannot approve it (RFC-054).

Migration `00009` adds `cards.owner_id`, `requirement_reviewers`, and `user_github.github_id` and `verified_at`. Packages: `internal/intake`, `internal/loop`, `internal/board`, `internal/identity`, `internal/ghcli`, `internal/runtime`, `internal/vcs`, `internal/app`, `internal/httpapi`.

- Web ingest stores the caller as `created_by` on new parsed rows. `create_card` copies that creator to `owner_id` and rejects requirements from two creators.
- The account token test runs `gh api user` and stores the reported login and id as verified. A new token clears it.
- `app.OwnerGitHub` resolves the owner's login, noreply email, and token, or `owner_github`. `runtime.Git.Author` replaces the bot name.
- `app.Publish` pushes and opens or updates the pull request with the owner's token, then requests reviewers.
- The owner's `actions` has no `approve`, and approval by the owner is `403`.
- `PUT /api/projects/{id}/requirements/{reqId}/reviewers`. The intake response adds `reviewers`, `canEditReviewers`, and `reviewerOptions`.
- UI: owner on the board and the card page, a reviewer picker on Intake, and Verified on the account GitHub page.

Done when: `TestIngestRecordsCreator`, `TestCardOwnerFromRequirement`, `TestCreateCardMixedOwners`, `TestGitHubLoginVerified`, `TestCommitIdentityOwner`, `TestPushUsesOwnerToken`, `TestOwnerGitHubBlocks`, `TestRequestReviewers`, `TestReviewersSetByCreator`, `TestOwnerCannotApprove`.

## Sprint 18: Conventions and skills

Goal: An admin keeps the company coding rules by stack and the integration skills. Each project's stacks are detected on Sync, a Lead Developer adds stacks and enables skills, the planner attaches skills to cards, and the implementer and reviewer packs carry the matching rules and skills with their versions in the trace (RFC-055, RFC-056).

Migration `00010` adds `stacks`, `convention_rules`, `skills`, `convention_revisions`, `project_stacks`, `project_skills`, and `card_skills`, with seed stacks, two global rules, and one OAuth skill. Packages: `internal/conventions`, `internal/contextpack`, `internal/loop`, `internal/app`, `internal/trace`, `internal/httpapi`.

- `conventions` saves rules, stacks, and skills with versions, hashes, and revisions. Only an admin may save.
- `conventions.Detect` reads marker files and dependencies from the clone. `app.Sync` replaces detected stacks and appends `stacks_detected`.
- `GET` and `PUT /api/projects/{id}/settings/conventions` for manual stacks and enabled skills, Lead Developer only.
- `create_card` takes `skillIds` from the project's enabled skills and stores them in `card_skills`. The card JSON adds `skills`.
- The implementer and reviewer packs open the untrusted section with rule and skill blocks, under 16 KiB per block and 64 KiB in total. The attempt appends `conventions_context`.
- UI: admin Conventions and Skills pages, a project Conventions tab, and skills on the card page.

Done when: `TestConventionCatalogAdminOnly`, `TestConventionRevisions`, `TestDetectStacks`, `TestSyncKeepsManualStacks`, `TestProjectConventionsLeadOnly`, `TestCreateCardSkills`, `TestPackConventions`, `TestConventionsCannotAddTool`, `TestConventionsContextEvent`, `TestReviewerPackConventions`, and `TestPromptMarkers` with the new system hash.

## Sprint 19: Tribe admins and people on the board

Goal: A tribe admin manages one tribe's members and projects, and chooses a new project's links when creating it. The board shows who owns a card and who reviews it (RFC-057, REQ-058).

Migration `00011` adds `tribe_members.admin`. Migration `00012` fills `cards.owner_id` for cards created before `00009`, when every requirement on the card has the same creator. Packages: `internal/tenancy`, `internal/app`, `internal/httpapi`.

- Members, candidates, and the tribe admin flag are allowed for an admin or a tribe admin of that tribe. A tribe admin cannot clear their own flag or remove themselves.
- `POST /api/projects` is allowed for an admin or a tribe admin of the tribe, and accepts `links`, checked and stored in the project's transaction.
- `PUT /api/projects/{id}/settings/links` is allowed for a Lead Developer member or a tribe admin.
- `GET /api/me` adds `tribeAdminOf`. The card JSON adds `reviewers`.
- UI: tribe admins see Tribes and New project, the members page grants the flag, New project picks linked projects, the board and card page show owner and reviewers, LLM APIs gets search and filters, and Linked projects explains an empty list.

Done when: `TestTribeAdminManagesMembers`, `TestTribeAdminGrantsTribeAdmin`, `TestNonTribeAdminCannotManage`, `TestTribeAdminCreatesProjectWithLinks`, `TestCreateProjectLinkOtherTribeRollsBack`, `TestTribeAdminSavesLinks`, `TestCardReviewers`.

## Sprint 20: User display names

Goal: Pages show a person's name instead of their email (RFC-058, REQ-064).

Migration `00013` adds `users.name`, empty for existing rows. Packages: `internal/identity`, `internal/app`, `internal/intake`, `internal/tenancy`, `internal/httpapi`.

- `POST /api/users` requires `name`. `PUT /api/users/{id}` accepts it. `PUT /api/me` changes the caller's own name.
- The user, member, candidate, card owner, and reviewer objects add `name`.
- UI: Name on New user, Edit user, and Account. The board, card page, user menu, Users, tribe members, and reviewer pickers show `name`, then the GitHub login, then the email.

Done when: `TestUserNameRequired`, `TestAdminRenamesUser`, `TestOwnName`, `TestCardPeopleNames`.

## Sprint 21: Card context

Goal: The board and the card page read in words and show who and what a card is built from.

Packages: `internal/httpapi`, `internal/conventions`.

- The single-card response adds `requirements` and `rules`. `skills` adds `useWhen`.
- UI: `statusLabel` and `reasonLabel` replace raw codes. Board tiles show the owner's avatar and name and a reviewer avatar stack. The card page has People, Intake, Rules, Skills, and Details panels.
- UI: the board filters by owner with a row of owner avatars, several at a time. It replaces Everyone and Mine.

Done when: `TestCardDetail`, `status.test.ts`, the owner cases in `board.test.ts`, `OwnerFilter.test.ts`.

## Sprint 22: Settings for admins

Goal: A company admin edits every project's settings, and a tribe admin edits its tribe's (RFC-059, REQ-065).

Packages: `internal/tenancy`, `internal/app`, `internal/conventions`, `internal/httpapi`.

- `SettingsVisible` and `CanEditSettings` back repository, links, deploy, conventions, and Sync. A company admin who is not a member opens settings and still gets `404` on cards.
- Every project settings body adds `canEdit`.
- UI: the four settings pages lock their fields and Save when `canEdit` is false. `SaveBar` takes `disabled`.

Done when: `TestAdminEditsProjectSettings`, `TestTribeAdminEditsProjectSettings`.

## Sprint 23: Repository check

Goal: GitHub confirms a project's repository and base branch before they are saved (RFC-060, REQ-066).

Packages: `internal/ghcli`, `internal/company`, `internal/tenancy`, `internal/app`, `internal/httpapi`.

- `company.CheckRepo` looks up the repository and branch with the company token. Project create and the repository save call it.
- UI: New project and Repository pick the repository from the organization list only. Archived repositories cannot be picked.
- The repository list adds the organization's repositories from `user/repos`. Test connection reports the token kind, scopes, the repositories it sees, and a hint.

Done when: `TestProjectRepoCheckedOnGitHub`, `TestProjectRepoNeedsCompanyToken`, `TestProjectRepoGitHubDown`, `TestRepoListIncludesCollaboratorRepos`, `TestCompanyTestReportsAccess`.

## Sprint 24: One card at a time

Goal: A person queues one ready card, or every ready card, and the board shows which cards are queued (RFC-061, REQ-067).

Packages: `migrations`, `internal/app`, `internal/httpapi`.

- `00014_card_runs.sql` adds `runs.card_id` and one open row per card.
- `app.QueueCardRun` and `app.QueueReadyRuns` add card-scoped rows for ready cards whose dependencies are done. The card's status does not change.
- The card lists `run`, `queued`, and `waitingOn`. The board returns `readyCount`.
- UI: `BoardCard` shows Run, Queued, or Waiting on N cards. The top button reads "Run ready cards (N)". The card page shows Run.
- The worker loop that consumes these rows is a later sprint.

Done when: `TestCardRunQueues`, `TestCardRunNeedsDependencies`, `TestCardRunNotReady`, `TestCardRunRole`, `TestRunReadyCards`, and `BoardCard.test.ts`.

## Sprint 25: The worker makes cards

Goal: Accepted requirements become cards on the board without a model (RFC-062, REQ-068).

Packages: `internal/worker`, `internal/loop`, `internal/app`, `internal/cli`, `internal/httpapi`.

- `worker.Pass` promotes backlog cards, makes one card per accepted requirement without a card, and closes the project's run rows when idle. Card-scoped rows stay open.
- `worker.Run` in `serve` runs one loop per project with that project's lock, and stops on shutdown.
- `run-next` runs the passes until idle when the project lock is free.
- The board's `running` flag follows open project rows, so the board polls until the worker is idle.

Done when: `TestPassCreatesCardPerRequirement`, `TestPassPromotesReady`, `TestPassMarksRunsProcessed`, `TestWorkerCreatesCardsAfterAccept`, `TestProjectLockSkips`, `TestRunNextPlans`, `TestBoardRunningWhileProjectRunOpen`.

## Sprint 26: Implement job with real models

Goal: A queued card runs with a real model up to a pull request (RFC-063, REQ-069).

Packages: `internal/providers`, `internal/providers/openai`, `internal/providers/anthropic`, `internal/providers/llm`, `internal/sandbox`, `internal/loop`, `internal/board`, `internal/app`, `internal/worker`, `internal/cli`, `internal/httpapi`.

- `openai-go/v3` and `anthropic-sdk-go` clients behind `providers.Provider`, with tool schemas, token counts, and redacted errors. `llm.ForModel` picks the client by the family's protocol.
- `app.RoutedProvider` retries the route's model once, then uses its fallback.
- `POST /api/settings/llm/{family}/test` sends one short request.
- `sandbox.ExecuteCheck` runs non-fixture checks in the pinned image with `podman` or `docker`.
- `app.ImplementCard` runs preflight, one attempt, the gating check, and `app.Publish`, with the failure, `attempt_cap`, and `wall_cap` paths.
- `worker.StartQueued` starts implement jobs for card-scoped rows under `implementationCap`. A project loop cancels open attempts when it takes its lock. `serve` and `run-next` pass the job in.
- Run is also offered for `in_progress` and `changes_requested` cards with no open attempt.

Done when: `TestOpenAIToolCall`, `TestOpenAIError`, `TestAnthropicToolCall`, `TestAnthropicBaseURL`, `TestForModelPicksProtocol`, `TestFallbackAfterRetry`, `TestLLMTestCallsProvider`, `TestLLMTestBadKey`, `TestLLMTestNoKey`, `TestExecuteCheckBuildsContainerArgv`, `TestImplementReachesPROpen`, `TestImplementCheckFailsReturnsInProgress`, `TestImplementAttemptCap`, `TestImplementBlocksSandboxConfig`, `TestImplementBlocksUnpinnedImage`, `TestImplementBlocksRouterConfig`, `TestImplementRefusesOpenAttempt`, `TestPassStartsQueuedCardsUnderCap`, `TestPassSkipsUnqueuedReady`, `TestCardRunNotReady`, `TestCardRunResumesInProgress`.

## Sprint 27: The implementer finds files

Goal: The implementer can list and search the worktree, and a pasted model endpoint URL works (RFC-064).

Packages: `internal/loop`, `internal/contextpack`, `internal/providers/openai`, `internal/providers/anthropic`.

- `list_files` and `search_files` read the worktree through `safePath`, skip `.git`, and do not follow symlinks.
- The implementer system prompt names the seven tools and the order of work.
- The `openai` and `anthropic` clients drop an endpoint suffix from the family `baseUrl`.
- A `429` waits for `retry-after`, up to 60 seconds, and is sent again. `model_call` names the model that answered.
- `job_error` is an accepted event kind.
- Each attempt drops uncommitted worktree changes before it starts.
- The implement loop sends earlier turns as assistant tool calls and tool results, keeping the newest results under 16 KiB.

Done when: `TestImplementerTools`, `TestListAndSearchFiles`, `TestOpenAIEndpointBaseURL`, `TestAnthropicBaseURL`, `TestRateLimitWaitsThenRetries`, `TestImplementProviderErrorRecordsMessage`, `TestDiscardChangesKeepsCommits`, `TestHistoryKeepsNewestResults`, `TestOpenAISendsTurns`, `TestAnthropicSendsTurns`.

## Sprint 28: A card reaches Approved

Goal: A card with a checked commit gets a pull request, a model review, and a CI wait, and a person approves it (RFC-065). Deploy is not part of this sprint.

Packages: `internal/runtime`, `internal/sandbox`, `internal/machine`, `internal/board`, `internal/loop`, `internal/app`, `internal/worker`, `internal/httpapi`.

- `runtime.Git.Export` writes the archive to a file before it is read. Git errors are cut at 2 KiB.
- The implement job probes the sandbox engine first, and gates and publishes an earlier attempt's commit without a model call.
- A job's `gh` child gets the git directory as its `PATH`, so `gh pr create` runs.
- Leaving `blocked` clears `blocked_reason`. Attempts, gates, and approvals carry the card's `project_id`. `POST /api/cards/{id}/{action}` checks the role and project membership.
- `app.ReviewCard` runs the review job with `read_diff`, `comment_pr`, and `verdict`, stores the review on the attempt, and applies the pull request checks.
- `app.RefreshCard` stores the head, sends a moved `approved` card back to `human_review`, and finishes the CI wait.
- `worker.Advancer` starts reviews and refreshes every tick. The card JSON fills `reviewedSha` and `reviewIndependent`.

Done when: `TestExportLargeArchive`, `TestImplementGatesExistingCommit`, `TestImplementRepublishesPassedCommit`, `TestRunnerPathHasOnlyGitDir`, `TestImplementBlocksWhenEngineDown`, `TestLeavingBlockedClearsReason`, `TestCardRowsKeepCardProject`, `TestLegacyCardRoutesCheckRole`, `TestReviewerTools`, `TestReviewApproveNoCI`, `TestReviewRequestChanges`, `TestReviewPendingCIWaits`, `TestPassFinishesCIWait`, `TestPassFailedCIRequestsChanges`, `TestReviewParseFallback`, `TestReviewWithoutVerdictBlocks` (superseded in Sprint 33 by RFC-069), `TestRefreshStoresHeadSHA`, `TestApprovedHeadMovedReturnsToReview`, `TestAdvancerSchedulesReviewAndRefresh`, `TestCardShowsLatestReview`, `TestCardReachesApproved`.

## Sprint 29: Pull request text

Goal: A pull request reads well to a person: a real title, the requirement and its acceptance lines, the changed files, the check result, and the AI review as a comment (RFC-066).

Packages: `internal/runtime`, `internal/vcs`, `internal/contextpack`, `internal/loop`, `internal/app`.

- `app.PRTitle` and `app.PRBody` build the title and the harness region of the body. `runtime.Git.CommitMessage` and `runtime.Git.NumStat` read the commit and its files.
- `vcs.MergeBody` rewrites only the region between the body markers and keeps text a person wrote around it.
- `gh pr create` passes the title. A later push edits the title and body, and comments are posted, through `gh api` REST calls with a JSON input file.
- The review job posts the verdict as one pull request comment with the company token.
- The implementer is asked for a 72-character subject and a short body in each commit message.

Done when: `TestPRTitleFromCommit`, `TestPRBodySections`, `TestPRBodyKeepsHumanText`, `TestImplementReachesPROpen`, `TestPublishEditsExistingPR`, `TestReviewPostsComment`, `TestReviewRequestChanges`, `TestReviewCommentFailureKeepsStatus`.

## Sprint 30: Solo approval

Goal: A one-person project can approve its own cards, and the record says the approval was not independent (RFC-067, REQ-070).

Packages: `internal/settings`, `internal/app`, `internal/httpapi`, `migrations`, frontend Deploy settings.

- `approval.solo` in the project `config.yaml`, read and written as `soloApproval` by the Deploy settings route and shown as a checkbox.
- `app.GateConfig.Solo` skips the owner and bot-login refusals. The card's `actions` keeps `approve` for the owner.
- Migration `00015` adds `approvals.independent`, `false` for an owner or bot-login approval.

Done when: `TestSoloOwnerApproves`, `TestOwnerCannotApprove`, `TestBotApprover`.

## Sprint 31: Approval on GitHub and the Changes panel

Goal: An approval in the orchestrator shows on the pull request, and the card page shows the code the card changes (RFC-068, REQ-071).

Packages: `internal/app`, `internal/runtime`, `internal/httpapi`, `internal/cli`, frontend card page.

- `app.ApproveFresh` reports a new approval. `app.AnnounceApproval` posts a comment and a `success` `orchestrator/approval` status on the approved SHA with the company token, from the HTTP route and the CLI. Request changes and Reject on an approved card post `failure`. A failed post writes `job_error` and leaves the card.
- The review job skips `orchestrator/approval` when it reads checks.
- `runtime.Git.Diff` passes `--no-renames`, and `runtime.Git.FetchCard` fetches the card branch. `app.ReadCardDiff` and `GET /api/projects/{projectId}/cards/{id}/diff` return the parsed, capped, redacted diff to members.
- `CardChanges` on the card page lists files, opens colored hunks, and links to the GitHub compare view.

Done when: `TestApprovalPostsCommentAndStatus`, `TestApprovalAnnounceFailureKeepsApproved`, `TestRequestChangesWithdrawsApprovalStatus`, `TestReviewIgnoresApprovalStatus`, `TestCardDiff`, `TestCardDiffRoute`, and the `CardChanges` Vitest cases.

## Sprint 32: Readable card trace

Goal: A person reads a card's trace as a story: status changes, then each attempt as one line that opens to its steps. It adds no decision; it refines the trace contract in tech/0002 and the Trace page in tech/0004 (REQ-039, REQ-042).

Packages: `internal/trace`, `internal/board`, `internal/httpapi`, `internal/app`, `migrations`, frontend Trace page.

- Migration `00016` adds `events.seq`. `trace.List` orders by `ts` then `seq`, so events in the same second keep their order.
- `board.CardAttempts` and the trace route's `attempts` list give each attempt's model, tokens, start and end, outcome, commit, and review.
- `app.jobError` drops NUL bytes and keeps the head and tail of a message over 8 KiB.
- The Trace page groups events into milestones and attempt blocks, writes each event as one line in words, cuts long text with Show all, hides binary output, and keeps the raw JSON one click away.

Done when: `TestTraceOrderAndAttempts`, `TestJobErrorCapped`, and the `trace`, `TraceRow`, and `TraceBlock` Vitest cases.

## Sprint 33: Checks decide when the AI review gives no verdict

Goal: A card whose pull request checks are green, or which has none, reaches Waiting for review even when the AI review fails, and an empty check list is not trusted until GitHub had time to register its runs (RFC-069, REQ-072).

Packages: `internal/app`, `internal/review`, `internal/board`, `internal/httpapi`, `migrations`, frontend card page and Trace page.

- Migration `00017` lets `review_verdict` be `failed`.
- `app.ReviewCard` records a review with no verdict as `failed`, posts "AI review: Not done", and applies the checks instead of blocking with `router_config`, `review_parse`, or `provider_error`. It reads the checks again after the model.
- `app.settleChecks` waits `review.NoChecksWait` (2 minutes) after `board.PublishedAt` before an empty check list counts as none.
- `app.RefreshCard` treats `failed` like `recommend_approve`, and finishes an `agent_review` card with no verdict when the card lock is free.
- The card JSON adds `reviewVerdict`. The card page shows "AI review: Failed, no verdict", and the Trace review block shows the same badge.

Done when: `TestReviewWithoutVerdictUsesChecks`, `TestReviewWithoutVerdictRedChecks`, `TestReviewWithoutVerdictWaitsForChecks`, `TestNoChecksWaitsToRegister`, `TestChecksInsideWaitDecide`, `TestRefreshFinishesStoppedReview`, `TestCardShowsLatestReview`, and the `TraceBlock` Vitest case for a failed review.

## Sprint 34: Self review on a requirement

Goal: A Lead Developer or an admin can mark one requirement as Self review, so the owner of its card approves alone, without turning Solo approval on for the whole project (RFC-070, REQ-073).

Packages: `internal/intake`, `internal/app`, `internal/httpapi`, `migrations`, frontend New requirement page, Reviewers panel, and card page.

- Migration `00018` adds `requirements.self_review`, default `false`.
- `intake.Add` stores `selfReview`. `intake.SetSelfReview` and the route `PUT /api/projects/{projectId}/requirements/{id}/self-review` change it. Both need `intake.CanSetSelfReview`: a Lead Developer or an admin.
- The intake JSON adds `selfReview` and `canSetSelfReview` to each requirement.
- `app.CardSelfReview` is true when every requirement of the card is flagged. The approval gate and the owner's `actions` treat such a card as Solo approval. The card JSON adds `selfReview`.
- The New requirement page and the Reviewers panel show the Self review checkbox to a Lead Developer or an admin. The card page shows Self review in Details.

Done when: `TestSelfReviewNeedsLead`, `TestSelfReviewOwnerApproves`, `TestCardSelfReview`, and the `ReviewersPanel` Vitest cases.

## Sprint 35: Markdown statements and a growing requirement form

Goal: A requirement statement and its acceptance lines can be written in markdown and read formatted on the intake pages, and the requirement form grows with its text (REQ-055, RFC-066). It adds no decision; it refines the REQ-055 display in tech/0004 and the RFC-066 body layout in tech/0003.

Packages: `internal/intake`, `internal/app`, frontend `$lib/markdown`, `$lib/requirements`, `Markdown`, `AcceptanceList`, `RequirementForm`, `RequirementRow`, `Field`, and the card page.

- `$lib/markdown` parses a statement into a token tree, and `Markdown.svelte` renders it as elements and text nodes, with links only to GitHub or the local harness. `plainText` gives the clamped excerpt and joins the search text.
- `RequirementForm` has one Write/Preview toggle for the statement and acceptance, an 8-row statement and a 6-row acceptance box, and both grow with their text through the `autogrow` action. Preview shows the acceptance lines as a checklist through `AcceptanceList`.
- The intake row renders the statement and acceptance lines as markdown. The table cell and the card page show a plain excerpt.
- The statement limit is 10000 characters, and CR and CRLF are stored as LF.
- An acceptance line drops one leading list or checkbox marker and keeps one line, in `toLines` and in the API, so the pull request checklist does not read `- [ ] - [ ]`.
- When the acceptance text has a markdown list, only its items become checks, as ingest reads an Acceptance section, and Preview counts the lines left out. The edit page loads saved checks as a `- ` list.
- The pull request body puts each id in bold on its own line, then the statement, and closes a code fence the statement left open.

Done when: `TestStatementLimitAndLineEndings`, `TestAcceptanceLineMarkers`, `TestPRBodyMarkdownStatement`, and the `markdown`, `Markdown`, `RequirementForm`, `IntakeActions`, and `intake` Vitest cases.

## Sprint 36: Card notes and a reason for Request changes

Goal: A card in `changes_requested` shows what to change. The AI reviewer's and the approver's notes read on the card page, and Request changes takes an optional reason that the next attempt reads. It adds no decision; it refines the card JSON and the request-changes body in tech/0002 and the card page in tech/0004.

Packages: `internal/board`, `internal/app`, `internal/httpapi`, `internal/cli`, frontend `CardNotes`, `CardActions`, and the card page.

- `board.CardNotes` lists a card's newest 10 failure notes in its project, newest first, with the attempt number. The single-card JSON adds `notes`. The board list does not.
- `app.ReviewCard` redacts the review body with the job's secrets before it stores the `review` note.
- `POST .../request-changes` takes an optional `reason`. `app.RequestChanges` stores it trimmed, with LF line ends, as the `approver` note in the transition's transaction, or `changes requested` when blank. Over 4000 characters is `422` with `fields.reason`. The CLI sends no reason.
- The card page shows a Notes panel above Next step. Review and approver notes render as markdown, check output as plain text, and earlier notes wait behind a button.
- Request changes opens a form with an optional "What should change?" box.

Done when: `TestCardDetailNotes`, `TestRequestChangesReason`, `TestReviewNoteRedacted`, and the `CardNotes` and `CardActions` Vitest cases.

## Sprint 37: Retry after a failed attempt, and the AI review mode

Goal: A card a person ran keeps going without a second click. A failed attempt queues the next one while attempts remain, and each requirement chooses what an AI request for changes does: run the next attempt, or let a person decide in Human review (RFC-071, RFC-072, REQ-074).

Packages: migration `00019`, `internal/app`, `internal/intake`, `internal/httpapi`, frontend `ReviewersPanel`, `AIReviewModeField`, `/intake/new`, and the card page.

- `no_commit` and `checks_failed` with attempts left insert a card-scoped `runs` row with trigger `retry`. At the cap the card blocks with `attempt_cap`. `CanStartImplement` refuses a card at the cap, so Run is not listed and a stale queued row blocks instead of starting.
- `requirements.ai_review_mode` is `continue` or `manual`. A card is `manual` when any of its requirements is.
- A `continue` card's `request_changes` moves to `changes_requested` with a `retry` row in one transaction, or blocks at the cap. A `manual` card keeps the `review` note and lets the pull request checks decide, so green checks reach `human_review`. Refresh sends back a waiting card whose mode turned `continue`.
- `PUT .../requirements/{id}/ai-review-mode` and the add body's `aiReviewMode` need a Lead Developer or an admin. The intake and card JSON return `aiReviewMode`.
- The Reviewers panel and the New requirement page show "When the AI review asks for changes". The card Details show "After AI review: A person reviews" for a manual card.
- A red check and a person's Request changes still wait for Run.

Done when: `TestRetryAfterNoCommit`, `TestRetryAfterChecksFailed`, `TestRunRefusedAtCap`, `TestReviewContinueQueuesRetry`, `TestReviewManualReachesHumanReview`, `TestRefreshSendsBackWhenNoLongerManual`, `TestCardAIReviewMode`, `TestAIReviewModeNeedsLead`, the updated `TestReviewRequestChanges`, and the `ReviewersPanel` and `api` Vitest cases.

## Sprint 38: Instructions for the next attempt

Goal: Before the next attempt runs, a person can tell the implementer more than the newest note says, and every later attempt reads it until someone changes it (RFC-073, REQ-075).

Packages: migration `00020`, `internal/board`, `internal/contextpack`, `internal/loop`, `internal/trace`, `internal/app`, `internal/httpapi`, frontend `CardActions`, `CardInstructions`, `CardNotes`, `api.ts`, and the card page.

- Instructions are a failure note with source `instructions`. The newest one is current, and a blank one clears them. The implement job's failure note skips them, and the pack writes them in their own block after it.
- The pack block runs from `INSTRUCTIONS FROM THE TEAM` to `END OF INSTRUCTIONS`, inside the markers. Builders drop any copy of those lines from untrusted text. The system string names the block and says it cannot add a tool, name a command, or change a card status.
- `PUT .../cards/{id}/instructions` and the Run body's optional `instructions` save them for the roles that may run, on any card that is not `done`, with an `instructions_saved` event. Run saves them in the run's transaction. Over 4000 characters is `422` on `instructions`.
- The card JSON lists `instructions` in `actions` and returns `instructions` on the single card.
- On the card page, Run opens "Instructions for the next attempt", prefilled. The Instructions panel shows and edits them. Notes label them "Instructions". The board's Run stays one click.

Done when: `TestPackInstructions`, `TestAttemptReadsInstructionsAndNote`, `TestSetInstructions`, `TestRunWithInstructions`, `TestInstructionsRoute`, `TestInstructionsRole`, and the `CardActions`, `CardInstructions`, `CardNotes`, `trace`, and `api` Vitest cases.

## Sprint 39: A live, verbose trace

Goal: A person who presses Run can follow the attempt on the Trace page without reloading, and each row says what the agent asked for and what the model said (RFC-074, REQ-076).

Packages: `internal/trace`, `internal/loop`, `internal/board`, `internal/app`, `internal/cli`, `internal/httpapi`, `internal/providers/fake`, frontend `trace.ts`, `pager.svelte.ts`, `TraceBlock`, `api.ts`, and the Trace page.

- `trace.Clip` drops NUL bytes and invalid UTF-8, redacts, then keeps the head and tail around a cut marker. `job_error` uses it at 8 KiB.
- `model_call` stores `text`, the model's reply, at most 4 KiB. `tool_call` stores `args` per tool, each string at most 1 KiB; `edit_file` stores `bytes` and `lines` and never the file text.
- `JobDeps.Secrets` carries the `databaseUrl` and `tokenKey` text from `serve` and `run-next`, and both jobs redact it.
- The trace route takes `since` (10 seconds of overlap, `422` when it is not RFC3339), and returns `seq` per event, `maxTurns` per attempt, and `live`.
- The Trace page polls every 3 seconds while `live`, merges events by id, shows a Live badge and a Now running strip, and follows the newest row while Follow latest is on. It announces only when an attempt ends.

Done when: `TestTraceClip`, `TestModelCallText`, `TestToolCallArgs`, `TestTraceSince`, `TestTraceLive`, `TestReviewerPackHasNoAuthorText`, `TestTraceOrderAndAttempts`, `TestJobErrorCapped`, and the `trace`, `pager`, `TraceBlock`, and Trace page Vitest cases.

## Sprint 40: The reviewer's reply limit

Goal: A reviewer that thinks at length before it answers still gets to give a verdict, and a review that fails because its reply was cut says so (RFC-075, under REQ-072).

Packages: `internal/providers`, `internal/providers/fake`, `internal/loop`, `internal/app`.

- Reviewer requests set `MaxTokens` to 32,768. Other task classes keep 8,192.
- The Anthropic client sets a request timeout sized to `MaxTokens`, so the SDK sends a 32,768-token request instead of refusing it as streaming-only.
- A reviewer reply with no tool call that stopped at `max_tokens` or `length` is asked again once on the same model. A second one is `ErrReviewCut`, an `ErrReviewParse`, so the fallback and RFC-069 apply.
- That failure's `job_error` and "AI review: Not done" comment name the token limit.

Done when: `TestReviewMaxTokens`, `TestAnthropicReviewMaxTokens`, `TestReviewRetriesCutReply`, `TestReviewCutTwiceFails`, and the rest of the review suite.

## Sprint 41: Retry an AI review that gave no verdict

Goal: A card waiting in `agent_review` after an AI review that gave no verdict can have the review run again, from the card page or the CLI (RFC-076, under REQ-072).

Packages: `internal/machine`, `internal/board`, `internal/app`, `internal/httpapi`, `internal/cli`, frontend `CardActions`.

- New edge `agent_review -> pr_open` with the attempt and the actor as evidence and no `local_checks` gate.
- `app.RetryReview` takes the card lock, requires a `failed` latest review, moves the card to `pr_open`, and clears the review. The worker runs the review job again. `attempts_used` does not change.
- The card JSON lists `retry_review` for the run roles. `POST .../retry-review` and `orchestrator retry-review CARD` call `app.RetryReview`.
- The card page shows "Retry AI review".

Done when: `TestEdgesMatchPRD`, `TestAgentReviewToPROpen`, `TestRetryReviewQueuesReview`, `TestRetryReviewNeedsFailedVerdict`, `TestRetryReviewLocked`, `TestRetryReviewAction`, `TestRetryReviewRole`, and the `CardActions` retry test.

## Sprint 42: Red checks retry, and the attempt cap on every send-back

Goal: No card waits in `changes_requested` with no attempts left and no action. A red check queues the next attempt like an AI request for changes (RFC-077, under REQ-074).

Packages: `internal/review`, `internal/app`, `internal/worker`.

- `review.ApplyCIThen` runs a hook in the transaction of the red-check move. `app.settleChecks` passes `queueOrCap`, which `sendBack` also uses: a `retry` run, or `blocked` with `attempt_cap`. A `manual` card gets `capOnly`: no run, but the cap still blocks.
- `app.RequestChanges` blocks a card out of attempts with `attempt_cap` in the same transaction.
- The Advancer refreshes `changes_requested` cards, and `RefreshCard` calls `AtCap` for them, which releases cards already stuck.

Done when: `TestCIRedQueuesRetry`, `TestCIRedAtCapBlocks`, `TestRefreshCapsChangesRequested`, `TestRequestChangesAtCapBlocks`, `TestAdvancerRefreshesChangesRequested`, and the rest of the review, gate, and worker suites.

## Sprint 43: Terminal view on the Trace page

Goal: A person watches a working card's trace as terminal output, with every reply and tool output in full, as it arrives (RFC-078, under REQ-076).

Packages: frontend `lib/terminal.ts`, `TerminalView`, the Trace page. No backend change.

- `terminal.terminalLines` turns each stored event into a head line and its full text, with a line where each attempt begins.
- `TerminalView` renders them as a dark monospaced log that follows new output, folds text past 200 lines, renders the newest 300 events, and offers "Jump to latest".
- The Trace page has a Structured and Terminal switch in `?view=terminal`, and polls every second in the terminal view while live.

Done when: the `terminal`, `TerminalView`, and Trace page terminal view Vitest suites, `npm run check`, and the backend suite unchanged.

## Sprint 44: Thinking and effort per model

Goal: An admin turns thinking on or off and picks the effort for each model, and every request to that model sends them (RFC-079, under REQ-077).

Packages: `internal/settings`, `internal/providers`, `internal/providers/anthropic`, `internal/providers/openai`, `internal/providers/llm`, `internal/loop`, `internal/app`, `internal/httpapi`, frontend `lib/llm.ts`, `FamilyForm`.

- `settings.Model` gains `thinking`, `thinkingBudget`, and `effort`, and `SaveLLM` checks them against the family's protocol.
- `GET` and `PUT /api/settings/llm` carry each family's `modelOptions`. `models` stays a list of ids.
- `app.RoutedProvider` and "Test connection" send each model's settings. The anthropic client sends `thinking`, `output_config.effort`, and raises `max_tokens` by the budget. The openai client sends `reasoning_effort` and the `thinking` body field.
- Both clients return the reply's reasoning, the loops keep it on the turn, and the clients send it back to the same model only.
- `FamilyForm` has Thinking, Budget, and Effort under each model id.

Done when: `TestNoModelOptionsSendsNothing`, `TestSaveLLMModelOptions`, `TestLLMSettingsModelOptions`, `TestAnthropicThinking`, `TestAnthropicReplaysThinking`, `TestOpenAIEffortAndThinking`, `TestOpenAIReplaysReasoningContent`, `TestRoutedProviderModelOptions`, the `llm` and `FamilyForm` model options Vitest cases, `npm run check`, and the rest of the backend suite.

## Sprint 45: Tools that do not destroy files

Goal: The implementer reads ranges, replaces exact text, and cannot empty a file or write a history placeholder into it (RFC-080, under REQ-078).

Packages: `internal/loop`, `internal/providers`, `internal/providers/fake`, `internal/providers/anthropic`, frontend `lib/trace.ts`, `lib/terminal.ts`, `TraceRow`.

- The fake provider returns several tool calls in one turn and records the turns it receives.
- `read_file` and `read_linked_file` take `offset` and `limit` and number their lines.
- New `replace_in_file`, `delete_file`, and `move_file`. `edit_file` writes through a rename and refuses placeholders and empty text over a non-empty file.
- `search_files` takes `regex`, `path`, `glob`, `ignoreCase`, and `context`. `list_files` takes `glob` and `depth` and skips `node_modules`, `vendor`, and `dist`.
- Tool errors start with `error: ` and go to Anthropic with `is_error`. Results are redacted. The history drops `edit_file` text.
- The Trace and Terminal views label the new tools.

Done when: `TestReadFileRange`, `TestReplaceInFileUnique`, `TestReplaceInFileNotFound`, `TestEditRefusesPlaceholder`, `TestHistoryDropsEditText`, `TestSearchRegexGlobContext`, `TestListGlobDepth`, `TestDeleteMoveSafePath`, `TestToolErrorsFlagged`, `TestToolResultsRedacted`, the updated `TestImplementerTools`, `TestToolCallArgs`, and `TestHistoryKeepsNewestResults`, the trace and terminal Vitest cases, and the rest of the suite.

## Sprint 46: A loop that finishes

Goal: An attempt keeps what it read, does not stop early, commits a tree that passed, stops at its caps, and keeps its attempt on an infrastructure failure (RFC-081, RFC-085, under REQ-079 and REQ-082).

Packages: `internal/settings`, `internal/loop`, `internal/providers`, both clients, `internal/app`, `internal/httpapi`, `migrations`, `cmd/fixture`, frontend `lib/llm.ts`, `FamilyForm`.

- `historyBudgetKiB` in `modelOptions`, a stable history that evicts in chunks, re-read dedupe, and check output that keeps its tail.
- `Turn.Note`, the continue nudge, the turns-left note at `max(3, cap/5)`, and the 40-turn default with a 200-turn limit.
- Auto-commit on a tree whose last check exited 0.
- `turn_cap` and `token_cap` with `budget_stop`, in the loop and in the fixture.
- Engine exits 125 and 127, provider errors, and publish errors leave `attempts_used`. Bad tool JSON goes back to the model. Outcome `no_change` goes to the gate.

Done when: `TestHistoryBudgetPerModel`, `TestHistoryStableEviction`, `TestRereadUnchanged`, `TestCheckOutputKeepsTail`, `TestTextReplyNudged`, `TestTruncatedReplyNudged`, `TestAutoCommitAfterPassingCheck`, `TestNoAutoCommitOnDirtyTree`, `TestTurnCapStops`, `TestTokenCapStops`, `TestInfraErrorKeepsAttempt`, `TestProviderErrorKeepsAttempt`, `TestBadToolJSONReturnedToModel`, `TestNoChangeGoesToGate`, the `FamilyForm` history budget Vitest case, the fixture, and the rest of the suite.

## Sprint 47: Pack, prompt, and todo list

Goal: The implementer starts with a map of the repository and its agent notes, keeps a plan, and reads in parallel (RFC-082, under REQ-080).

Packages: `internal/contextpack`, `internal/loop`.

- A path map up to 12 KiB, and `AGENTS.md` and `CLAUDE.md` up to 16 KiB each and 32 KiB in total, inside the markers, with a `repo_context` event.
- Requirements with heading and acceptance lines, and the decisions that name them.
- A new `ImplementerSystem`.
- `todo_write`, echoed in every request's last note.
- Concurrent read-only calls.

Done when: `TestPackPathMap`, `TestPackRepoMemory`, `TestPackAcceptanceLines`, `TestPromptMarkers`, `TestTodoWriteEchoed`, `TestTodoWriteCap`, `TestReadOnlyCallsConcurrent`, and the rest of the suite.

## Sprint 48: Prompt caching

Goal: Anthropic requests reuse their prefix, and cached tokens are recorded and weighed fairly (RFC-083, under REQ-081).

Packages: `internal/providers`, both clients, `internal/board`, `internal/loop`, `migrations`.

- Cache breakpoints on the system block, the last tool, and the newest turn's last result.
- `Response.CachedInputTokens` from both clients, `attempts.tokens_cached`, and `cachedInputTokens` on `model_call`.
- The token cap counts cached reads at a tenth.

Done when: `TestAnthropicCacheBreakpoints`, `TestCacheUsageRecorded`, `TestTokenCapCountsCachedAtTenth`, `TestHistoryPrefixStable`, and the rest of the suite.

## Sprint 49: Sandboxed run_command

Goal: The implementer runs an allowlisted argv in the sandbox on a throwaway copy (RFC-084, under REQ-082, amending REQ-017 and REQ-018).

Packages: `internal/sandbox`, `internal/settings`, `internal/loop`, `internal/app`, `internal/httpapi`, frontend project checks settings.

- `checks.commandExecutables`, refusing shells and wrappers.
- `sandbox.ExecuteArgv` shared with `ExecuteCheck`, a fresh copy per call under `<data>/exports` with the export assertion, no network, 5 minutes, 1 MiB, and 30 calls per attempt.
- Refused in fixture mode, and never check evidence.
- An "Allowed commands" field on the project checks page.

Done when: `TestRunCommandParse`, `TestRunCommandRefusesShell`, `TestRunCommandFlags`, `TestRunCommandOnCopy`, `TestRunCommandDisabled`, `TestRunCommandBudget`, `TestRunCommandCannotPassGate`, `TestRunCommandRedactsOutput`, `TestRunCommandExportAssertion`, `TestSaveDeployCommandExecutables`, the project settings Vitest case, `npm run check`, and the rest of the suite.

## Sprint 50: Refine a requirement with AI

Goal: A member refines a manual requirement with the implement model, compares the proposal, and applies it to the form (RFC-086, under REQ-083).

Packages: `internal/intake`, `internal/contextpack`, `internal/app`, `internal/httpapi`, frontend intake pages.

- `POST /api/projects/{projectId}/requirements/refine` with the `propose_refinement` tool, the save checks on the proposal, and a `requirement_refined` event without text.
- A "Refine with AI" button, a before and after comparison, and Apply and Discard on the new and edit pages.

Done when: `TestRefineRequirementProposal`, `TestRefineDoesNotSave`, `TestRefineNoImplementRoute`, `TestRefineInvalidProposal`, `TestRefineRequiresMember`, `TestRefinePromptMarkers`, `TestRefineRedactsKeys`, the `textdiff`, `RefineCompare`, `RequirementForm`, and edit page Vitest cases, `npm run check`, and the rest of the suite.

## Sprint 51: Refine one field at a time, streamed

Goal: The statement and the acceptance lines refine apart, and the reply streams into the page (RFC-087, under REQ-083).

Packages: `internal/providers`, `internal/router`, `internal/app`, `internal/contextpack`, `internal/httpapi`, frontend `api.ts`, `Field`, `RequirementForm`, `RefinePanel`, `RefineCompare`.

- `providers.Streamer` in the anthropic, openai, and fake clients, and a routed stream that falls back only before the first piece of text.
- `app.RefineField` with plain-text field prompts, a redact buffer, the save rules on the finished text, and `field` on the event. The refine route streams `start`, `delta`, and `done` or `error`.
- A spark button and a refine box per field, with streamed text, Stop, the field's comparison, Apply, and Discard.

Done when: `TestAnthropicStream`, `TestOpenAIStream`, `TestRoutedStreamFallsBackBeforeFirstToken`, `TestRoutedStreamNoRetryAfterToken`, `TestRefineFieldStreams`, `TestRefineFieldAcceptanceLines`, `TestRefineFieldRedactsSplitKey`, `TestRefineFieldErrorsBeforeStream`, `TestRefineFieldCancel`, `TestRefinePromptMarkers`, the `streamRefine`, `RefinePanel`, `RequirementForm`, and edit page Vitest cases, `npm run check`, and the rest of the suite.

## Sprint 52: Stream the card trace

Goal: While a card works, the Trace page shows stored events as they commit and the model's reply token by token, over one streamed connection (RFC-088, under REQ-076).

Packages: `internal/live`, `internal/trace`, `internal/providers`, `internal/app`, `internal/loop`, `internal/httpapi`, `internal/cli`, frontend `api.ts`, `terminal.ts`, `TerminalView`, and the Trace page.

- `live.Hub` with per-card viewers, the text so far, non-blocking sends, caps, and a close on shutdown. The stream redactor moves to `trace`.
- `providers.RestartStreamer`: the routed provider keeps its retry and fallback after text and says when it starts again.
- The implementer and reviewer loops stream each turn as `turn`, `delta`, `reset`, and `turn_end`, redacted with every job secret.
- `GET /api/projects/{projectId}/cards/{id}/trace/stream`: the stored trace, live frames, a database read every second and after each model call, a heartbeat, and `done` or `resync`.
- The Trace page reads the stream while live and polls only as a fallback. The terminal view shows the reply being written with a caret.

Done when: `TestHubSnapshotAndSlowSubscriber`, `TestStreamRedactorEverySplit`, `TestRoutedStreamRestartAfterToken`, `TestLoopStreamsTurnText`, `TestReviewStreamsText`, `TestTraceStreamFrames`, `TestTraceStreamRedactsSplitSecret`, `TestTraceStreamOtherProcessEvents`, `TestTraceStreamRequiresMember`, `TestTraceStreamCap`, the `streamTrace`, Trace page, and `TerminalView` Vitest cases, `npm run check`, the fixture, and the rest of the suite.

## Sprint 53: Token usage across projects

Goal: An admin sees the token usage of every stored model call across projects, and filters, groups, and sorts it by LLM, model, project, user, and task type (RFC-089, REQ-084).

Packages: `migrations`, `internal/loop`, `internal/board`, `internal/app`, `internal/httpapi`, `internal/cli`, frontend `api.ts`, `usage.ts`, the admin Token usage page, and the layout nav.

- An index on `events (kind, ts)`, and `taskClass` on new `model_call` events.
- `board.UsageRows` totals `model_call` and `requirement_refined` events per day, project, model, task type, and user.
- `app.Usage` checks the admin flag and the range and adds each model's family from `config.yaml`. `GET /api/usage` and `orchestrator usage` share it.
- `/admin/usage` with the range, five filters, Group by, a sortable table, the totals, and a bar per day.

Done when: `TestUsageAggregates`, `TestUsageTaskClass`, `TestUsageUser`, `TestUsageFamily`, `TestUsageAdminOnly`, `TestUsageRange`, `TestModelCallTaskClass`, `TestUsageCLI`, the `usage` lib, Token usage page, and nav Vitest cases, `npm run check`, the fixture, and the rest of the suite.

## Sprint 54: Token usage for a tribe and for each member

Goal: A tribe admin sees the token usage of their tribe's projects, and every member sees their own (RFC-090, REQ-085).

Packages: `internal/board`, `internal/tenancy`, `internal/app`, `internal/httpapi`, `internal/cli`, frontend `api.ts`, `usage.ts`, the `UsageDashboard` component, the tribe and account usage pages, and the layout menu.

- `board.UsageScope` narrows `board.UsageRows` to a tribe's projects or to one user, and a call on a card counts in the card's project.
- `app.TribeUsage` checks the tribe like the other tribe routes, and `app.MyUsage` reads the caller's own calls. `GET /api/tribes/{id}/usage`, `GET /api/me/usage`, and `orchestrator usage --tribe --user` share them.
- `UsageDashboard` serves `/admin/usage`, `/tribes/{id}/usage`, and `/account/usage`, with links from the tribe page, the account page, and the user menu.

Done when: `TestUsageTribeScope`, `TestUsageSelfScope`, `TestTribeUsageAccess`, `TestMyUsage`, `TestUsageCLIScope`, the `usage` lib, tribe and account usage page, admin usage page, and menu Vitest cases, `npm run check`, the fixture, and the rest of the suite.

## Sprint 55: Token usage by source and card

Goal: Every Token usage page filters and groups by source, Intake or Card, and by card (RFC-091, REQ-086).

Packages: `internal/board`, `internal/app`, `internal/cli`, frontend `types.ts`, `usage.ts`, the `UsageDashboard` component, and the account usage page.

- `board.UsageRows` groups by card too and returns the card id, its title, and the source.
- `app.GroupUsage` and `orchestrator usage --by` take `source` and `card`.
- The usage pages filter and group by Source and Card.

Done when: `TestUsageSourceAndCard`, `TestUsageCLICard`, the existing usage tests, the `usage` lib, admin usage page, and account usage page Vitest cases, `npm run check`, the fixture, and the rest of the suite.

## Sprint 56: Tribe MCP servers, settings

Goal: A tribe admin registers MCP servers, keeps their secrets write-only, and allowlists their tools (RFC-092, RFC-093, REQ-087).

Packages: migration `00024`, `internal/tenancy`, `internal/netguard`, `internal/mcp`, `internal/app`, `internal/httpapi`, frontend `types.ts`, `api.ts`, and the tribe MCP pages.

- `tribe_mcp_servers` and `tribe_mcp_tools` store a tribe's servers, the sealed secrets, and the allowlist.
- `netguard` refuses internal addresses, proxies, and redirects. `mcp.DialHTTP` lists and calls tools with a 256 KiB cap.
- `/api/tribes/{id}/mcp-servers` lists, creates, saves, deletes, and discovers, checked like the other tribe routes.
- `/tribes/{id}/mcp` lists the servers, and `/tribes/{id}/mcp/{serverId}` edits one.

Done when: `TestMCPServerSecretsNeverReturned`, `TestMCPServerAccess`, `TestMCPSlugAndToolNames`, `TestMCPNameCollision`, `TestNetguardRefusesPrivate`, `TestNetguardRedirectAndProxy`, `TestMCPHTTPListAndCall`, `TestMCPResultCap`, `TestMCPDiscoverNoBody`, the tribe MCP pages Vitest cases, `npm run check`, the fixture, and the rest of the suite.

## Sprint 57: MCP tools in the implementer loop

Goal: The implementer calls the tribe's allowlisted tools as untrusted data (RFC-094, REQ-087).

Packages: `internal/loop`, `internal/contextpack`, `internal/providers/anthropic`, `internal/app`, frontend `trace.ts`.

- `loop.Input.Extra` adds frozen `mcp_` tools after the constant list, with call caps, wrapped and redacted results, and no live frame.
- `ImplementCard` loads the tribe's servers once per attempt, checks schema hashes, redacts the secrets, and records `mcp_context`.
- The Anthropic client keeps a tool schema's other top-level keys.

Done when: `TestImplementerMCPTools`, `TestMCPResultWrappedAndStripped`, `TestMCPHeaderRedacted`, `TestMCPCallCap`, `TestMCPToolListChanged`, `TestMCPCannotMoveCard`, `TestMCPNoLiveFrame`, `TestMCPOtherTribe`, `TestAnthropicSchemaExtraFields`, the updated `TestImplementerTools` and `TestPromptMarkers`, the trace Vitest case, `npm run check`, the fixture, and the rest of the suite.

## Sprint 58: Stdio MCP servers in the sandbox

Goal: A stdio server runs in its pinned image with no network for the length of an attempt (RFC-093, REQ-087).

Packages: `internal/execx`, `internal/sandbox`, `internal/mcp`, `internal/app`.

- `execx.Start` keeps a child's stdin and stdout open, with a frame cap and a process-group kill.
- `sandbox.StartArgv` runs the server with `-i`, `--network=none`, a name and label, and removes the container on close.
- `mcp.DialStdio` speaks line-delimited JSON-RPC.

Done when: `TestStdioNoNetworkNoArgvSecret`, `TestStdioRefusesShell`, `TestStdioContainerRemoved`, `TestStdioFixtureRefused`, `TestStdioListAndCall`, the fixture, and the rest of the suite.

## Sprint 59: Refine with MCP tools

Goal: Requirement refine may look things up with the tribe's tools before it answers (RFC-095, REQ-087).

Packages: `internal/providers`, `internal/app`, `internal/httpapi`, frontend `types.ts` and `RefinePanel`.

- `providers.Request.ToolChoice` lets the last turn send `none`.
- `RefineField` runs at most 3 turns and 3 calls when the tribe has a server, and streams `reset` and `tool` events.

Done when: `TestRefineNoServersNoTools`, `TestRefineMCPLoop`, `TestRefineMCPResetEvent`, `TestRefineLastTurnNoTools`, `TestRefineMCPCancel`, the `RefinePanel` reset Vitest case, `npm run check`, the fixture, and the rest of the suite.

## Sprint 60: Database connectors

Goal: A tribe reads its Postgres and SQL Server databases through read-only connectors in the harness (RFC-097, REQ-088).

Packages: migration `00025`, `internal/dbtools`, `internal/tenancy`, `internal/app`.

- `tribe_mcp_servers` takes the `postgres` and `sqlserver` transports and a `config_json` with host, port, database, user, and TLS mode.
- `dbtools` offers `query`, `list_tables`, and `describe_table`, read-only, with a row cap, and refuses the board database.
- `MCPDialer.Open` opens a connector, so jobs, refine, and discovery use it like any MCP server.

Done when: `TestDBConfigValidation`, `TestPostgresToolReadOnly`, `TestPostgresSingleStatement`, `TestPostgresRowCap`, `TestDBConnectorRefusesBoard`, `TestDBPasswordRedacted`, `TestImplementerDBTools`, `TestSQLServerStatementCheck`, `TestSQLServerLive` when `ORCH_TEST_MSSQL` is set, the fixture, and the rest of the suite.

## Sprint 61: MCP catalog API and image pull

Goal: The harness lists the built-in connectors and searches the MCP Registry, and pins a stdio image by digest (RFC-096, RFC-098, REQ-088).

Packages: `internal/mcpcatalog`, `internal/app`, `internal/httpapi`, `internal/cli`.

- `GET /api/tribes/{id}/mcp-catalog` returns the built-in entries and registry entries, cached for 10 minutes, with a fixed notice when the registry is down.
- `POST /api/tribes/{id}/mcp-images/pull` pulls an image and returns it pinned by digest.

Done when: `TestMCPCatalogBuiltins`, `TestMCPCatalogRegistry`, `TestMCPCatalogRegistryDown`, `TestMCPCatalogAccess`, `TestMCPImagePull`, the fixture, and the rest of the suite.

## Sprint 62: MCP catalog page

Goal: A tribe admin sets up a server from the catalog by filling in its credentials (RFC-096, REQ-088).

Packages: frontend `types.ts`, `api.ts`, `MCPServerForm`, and the tribe MCP pages.

- `/tribes/{id}/mcp/catalog` searches the catalog, and `Set up` opens the form filled from the entry.
- The form has the database fields, pulls and pins a stdio image, and discovers tools after a create.

Done when: the catalog page, database form, registry prefill, image pull, and auto-discover Vitest cases, `npm run check`, the fixture, and the rest of the suite.

## Sprint 63: Featured MCP servers and a catalog that answers at once

Goal: The catalog shows featured and built-in servers at once and searches the registry on its own (RFC-099, RFC-100, REQ-088).

Packages: `internal/mcpcatalog`, `internal/app`, `internal/httpapi`, frontend `api.ts`, `types.ts`, and the catalog page.

- `mcpcatalog.Featured`, `Local`, `Registry`, and `LocalEntry`; the `source` parameter on the catalog route.
- The registry fetch has 60 seconds, remembers a failure for 30 seconds, runs at most 4 at once, and reads a registry entry by exact name for Set up.
- The catalog page loads local entries and the registry apart, aborts a stale search, and shows Featured, Databases, and MCP Registry sections.

Done when: `TestMCPCatalogBuiltins`, `TestMCPCatalogFeatured`, `TestMCPCatalogRegistrySlow`, `TestMCPCatalogEntryByName`, `TestMCPCatalogRegistryDown`, `TestMCPCatalogAccess`, the catalog page Vitest case, `npm run check`, the fixture, and the rest of the suite.
