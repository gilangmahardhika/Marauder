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
