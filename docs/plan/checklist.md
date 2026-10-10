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

- [ ] Ensure-PR is idempotent on card id. Metadata block is rewritten on each push. The summary block stays. Push uses the settings URL and the card refspec only, with the token in askpass, not in argv (RFC-034).
- [ ] Reviewer verdicts: `request_changes` to `changes_requested`, `recommend_approve` plus green required CI to `human_review`. Pending CI stays in `agent_review`. Red CI goes to `changes_requested` with a `ci` note.
- [ ] Review and triage do not increment `attempts_used` (RFC-042). `review_independent` is stored and is false when only one family is configured (RFC-008).
- [ ] `TestPROnce`
- [ ] `TestMetadataRewrite`
- [ ] `TestPushURL`
- [ ] `TestAskpass`
- [ ] `TestReviewerPack`
- [ ] `TestReviewNotCounted`
- [ ] `TestCIPending`
- [ ] `TestCIRed`
- [ ] `TestRouteClass`
- [ ] `TestNoDeployClass`

## Sprint 6: Approval is a person and a SHA

- [ ] `approve --sha`, `request-changes`, `reject`, `unblock`. Missing or wrong SHA exits 1 and writes no approval row.
- [ ] Same SHA twice inserts one approval. Bot login cannot be an approver.
- [ ] Unblock destinations follow RFC-041: budget and reject go to `ready`; `health`, `deploy_failed`, and `deploy_config` go to `human_review`. `source_changed` stays blocked while a linked id is unaccepted. Default release at the cap raises `max_attempts` by one. `--reset-attempts` zeros `attempts_used`.
- [ ] `TestApprovedHeadMoved`
- [ ] `TestRequestChangesFromApproved`
- [ ] `TestRejectFromApproved`
- [ ] `TestApproveSha`
- [ ] `TestApproveIdempotent`
- [ ] `TestStaleSHA`
- [ ] `TestUnblockDestinations`
- [ ] `TestUnblockAtCap`
- [ ] `TestUnblockReset`
- [ ] `TestUnblockSourceChanged`
- [ ] `TestCapFromChangesRequested`
- [ ] CLI approve and unblock checks

## Sprint 7: Deploy the approved SHA, then test it

- [ ] Deploy and rollback commands are absolute paths outside the repo and the data directory, mode `0700` or `0500` (RFC-038). The SHA argument is 40 or 64 hex only.
- [ ] Probe allows a listed dev host, refuses link-local, loopback outside fixture mode, and unlisted hosts. A redirect is a failure (RFC-039).
- [ ] Empty `dev_check` plus a healthy probe is `health_only` and `done`. A failing dev check returns to `changes_requested` on the same PR and stores the triage note.
- [ ] Reconcile: deploy event without health probes or rolls back before another deploy exec. Commit without a gating check reruns the check and does not call the model.
- [ ] `TestDeployFailed`
- [ ] `TestHealthFail`
- [ ] `TestHealthOnly`
- [ ] `TestDevFailThenPass`
- [ ] `TestVerifyAtDeployedSHA`
- [ ] `TestDeployCommandPath`
- [ ] `TestSHAArg`
- [ ] `TestHealthLinkLocal`
- [ ] `TestHealthLoopback`
- [ ] `TestHealthPrivateListed`
- [ ] `TestHealthUnlisted`
- [ ] `TestHealthRedirect`
- [ ] `TestReconcileDeploy`
- [ ] `TestReconcileCommit`
- [ ] `TestDevCheckNetwork`

## Sprint 8: One worker runs the board

- [ ] `config.yaml` and mode `0600` `secrets.json`. GET never returns a token. PUT without a secret field keeps the stored bytes. Short secrets and unknown model ids are `422`. A config change event names fields, not values (RFC-030).
- [ ] Router: retry primary once, then one fallback, then `blocked` with `provider_error`. Empty route blocks with `router_config`. A save during a running attempt does not change that attempt's model.
- [ ] Worker lock file. A second `serve` exits. Plan job creates cards only for accepted ids and refuses a dependency cycle. Implementation cap and the global deploy lock are enforced. A moved base ref blocks with `base_moved`.
- [ ] `TestSecretOmitted`
- [ ] `TestSecretKeep`
- [ ] `TestShortSecret`
- [ ] `TestUnknownModel`
- [ ] `TestModelIdUnique`
- [ ] `TestBotApprover`
- [ ] `TestConfigEvent`
- [ ] `TestSnapshot`
- [ ] `TestRouterConfig`
- [ ] `TestFallbackOnce`
- [ ] `TestPlanCards`
- [ ] `TestWorkerLock`
- [ ] `TestDependency`
- [ ] `TestDeployLock`
- [ ] `TestCap`
- [ ] `TestBaseRefUnchanged`
- [ ] `TestLockHolder`

## Sprint 9: The fixture script and the JSON API

- [ ] Fixture runner uses a temp data directory, copies the deploy stub outside the repo, replays the fake provider for two families, and requires a clean `git status` in the fixture repo (REQ-045).
- [ ] HTTP: bearer session, loopback `Host` check, CORS only for `http://127.0.0.1:4173`, 1 MiB body cap, the error shape from the contract. Card JSON includes server-computed `group`, `actions`, and `unblockTo`.
- [ ] Session lockout after 5 failures. Wrong `Origin` and evil `Host` are `403`.
- [ ] `go run ./cmd/fixture` exits 0
- [ ] `TestActionsEmpty`
- [ ] `TestActionsApproved`
- [ ] `TestCORS`
- [ ] `TestHostHeader`
- [ ] `TestSessionLockout`
- [ ] `TestRedactPack`
- [ ] `TestSandboxOffRejected`
- [ ] `TestAskpass` still passes inside `go test ./...`

## Sprint 10: The operator UI and the release pass

- [ ] App shell: client-only SvelteKit, `adapter-static`, CSP meta, session bearer in `sessionStorage`, `src/lib/api.ts` covering every route.
- [ ] Work pages: `/login`, `/` (six groups, Run, 3s poll), `/intake`, `/cards/[id]`, `/cards/[id]/trace`. `CardActions` renders only names in `actions`.
- [ ] Settings pages: GitHub, LLM APIs, router, deploy. `SecretField` omits an empty secret and never prints a token.
- [ ] Styles: system font, one `.primary` button, `.danger` only for blocked, 44px controls, visible focus, one column under 720px.
- [ ] `npm run check`, `npm run test`, `npm run build`, and `npm audit --omit=dev` from `frontend/`
- [ ] Vitest: `CardActions` empty, approved, and `unblockTo: null`
- [ ] Vitest: `SecretField` omits the secret
- [ ] Vitest: `api` clears the bearer on 401
- [ ] Built `index.html` has the CSP meta and no `unsafe-inline`
- [ ] Manual browser pass on fixture cards, from the table in the verification guide
- [ ] Optional, after that pass: one real dev host using the same deploy code, with secrets only in `secrets.json`. That run is recorded by hand and is not part of `go test`
