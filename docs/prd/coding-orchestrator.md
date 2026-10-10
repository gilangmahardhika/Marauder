# PRD: AI Coding Orchestrator

Status: v1 draft, revised after review. Date: 2026-10-10.

This document is the requirements source. [docs/README.md](../README.md) lists every document and which one wins when two disagree. Decisions are in `docs/rfc/`, RFC-0001 through RFC-0005. Implementation guides are in `docs/tech/`.

## Problem

A PRD and its RFCs describe the change. Getting that change onto a dev server still depends on a person copying scope into a coding session, opening a pull request, finding a reviewer, deploying the right commit, and reading the test log. When a test fails, the next session often starts from chat memory instead of from the card, the diff, and the log.

v1 is a harness that runs that path. Models propose edits and review comments. The board, the command exit code, the health check, and a person's approval decide what happens next.

## Users

| Person | What they do |
| --- | --- |
| Admin | A flag on a user, not a job role. Creates users, sets the job role and the admin flag, sets passwords, creates tribes, adds members, and creates projects. Does not approve or deploy unless the job role allows it. |
| Product Owner | Accepts requirements for a project whose tribe they belong to. May request changes, reject, approve, and unblock. Does not deploy. |
| Lead Developer | May start a run, request changes, reject, approve, unblock, and start deploy. Edits that project's repo settings, checks, router, models, and deploy settings. |
| QA Lead | May request changes, reject, and approve. Does not accept requirements or deploy. |
| Senior Developer | May start a run and request changes. Does not accept, approve, reject, or deploy. |
| Developer | May start a run. Does not accept, approve, reject, request changes, or deploy. |
| QA | May start a run. Does not accept, approve, reject, request changes, or deploy. |

A user has one of those six job roles. The role applies in every tribe they belong to. They sign in with email and password and keep their own GitHub host, login, and token. A user may belong to more than one tribe. They see a project only when they are a member of its tribe. The orchestrator bot is never an approver.

Models are workers the harness calls. They are not users, and they do not hold the board.

## v1 outcome

A member can give the orchestrator a markdown PRD and its RFCs and get this result:

1. Requirement and decision records exist, and no code is written until a Product Owner who is a member accepts them.
2. Accepted requirements become kanban cards with dependencies, risk, and check commands.
3. Each card is implemented in its own worktree by a routed model, then opened as one pull request.
4. A reviewer model comments. A Lead Developer, QA Lead, or Product Owner who is a member approves a specific commit.
5. A Lead Developer who is a member starts deploy of that commit. The harness sends it to the project's dev server, checks health, and runs the card's dev tests.
6. A failed dev test starts a new attempt on the same pull request. A passed dev test marks the card done and unblocks cards that depend on it.

The operator can stop the process in the middle and start it again without a second deploy of an unknown commit.

## Requirement index

| Id | Requirement |
| --- | --- |
| REQ-001 | Parse PRD and RFC markdown into records |
| REQ-002 | Propose ids when the source has none |
| REQ-003 | Acceptance of the extract gates all implementation |
| REQ-004 | A source edit invalidates acceptance for the changed ids |
| REQ-005 | Cards carry scope, risk, checks, and links |
| REQ-006 | Only legal transitions run, and each one has evidence |
| REQ-007 | A card waits until its dependencies are done |
| REQ-008 | One writer holds a card |
| REQ-009 | The scheduler starts ready work and serializes deploys |
| REQ-010 | The router chooses a task class from card facts |
| REQ-011 | Every route decision is stored |
| REQ-012 | Provider failure retries once, then falls back, then blocks |
| REQ-013 | Review uses another model family when one is configured |
| REQ-014 | A model cannot change its route or its budget |
| REQ-015 | Budgets are configurable and they stop the card |
| REQ-016 | Each role receives a fixed context pack |
| REQ-017 | Each role has a fixed tool allowlist |
| REQ-018 | Check commands come from settings and run on a copy |
| REQ-019 | Deploy credentials stay out of model processes |
| REQ-020 | The inner loop works in a fresh worktree and commits there |
| REQ-021 | A local check passes only on exit code 0 |
| REQ-022 | The inner loop has no deploy path |
| REQ-023 | A new attempt starts a new conversation |
| REQ-024 | One pull request per card, updated in place |
| REQ-025 | The harness owns the pull request metadata |
| REQ-026 | Required CI must be green before human review |
| REQ-027 | The reviewer model sees a cold pack |
| REQ-028 | Review verdicts map to two states |
| REQ-029 | The reviewer model cannot approve, edit, or deploy |
| REQ-030 | Human approval is a permitted member and one SHA |
| REQ-031 | A new commit clears approval |
| REQ-032 | Request-changes and reject are explicit |
| REQ-033 | Deploy sends the approved SHA to dev |
| REQ-034 | One deploy runs at a time in a project |
| REQ-035 | Failed health rolls back and blocks |
| REQ-036 | Dev tests run only after health passes |
| REQ-037 | A failed check produces a triage note for the next attempt |
| REQ-038 | The outer loop repeats until done or blocked |
| REQ-039 | An append-only event log explains the card |
| REQ-040 | Restart resumes from the last gate |
| REQ-041 | The log stores model usage and redacts secrets |
| REQ-042 | The CLI and the local web UI share one service layer |
| REQ-043 | The fixture script is the release check |
| REQ-044 | Settings are edited through the API, and secrets stay write-only |
| REQ-045 | Orchestrator state lives outside the target repo |
| REQ-046 | A person signs in with email and password |
| REQ-047 | A job role and an admin flag gate human actions |
| REQ-048 | A tribe has members |
| REQ-049 | A project is visible to its tribe |
| REQ-050 | The worker runs per project |
| REQ-051 | Each user has their own GitHub settings |

## Requirements

### REQ-001 Parse PRD and RFC markdown into records

The orchestrator reads markdown PRDs and RFCs from the target repo and stores requirement and decision records. The default paths are `docs/prd/` and `docs/rfc/`. A record keeps its id, source path, heading, statement, and acceptance list. A decision record keeps its id, statement, and source path.

#### Acceptance

- A fixture PRD that already contains `REQ-` headings yields one requirement record per id, with the acceptance bullets under that heading.
- A fixture RFC that contains decision ids yields one decision record per id.
- Re-running parse on unchanged files does not create duplicate records.
- A file outside the configured paths is not ingested.

### REQ-002 Propose ids when the source has none

When a PRD has no requirement ids, a planner model may propose ids, statements, and acceptance lines. The proposal is stored as a draft. It does not create cards and it does not start a worktree.

#### Acceptance

- A PRD with no `REQ-` ids produces a draft list and zero cards in `ready` or later.
- The draft records the planner model id and prompt hash.
- The operator can see the draft list before accepting it.

### REQ-003 Acceptance of the extract gates all implementation

The operator accepts the requirement list as a recorded gate. Until that gate exists, every card stays in `backlog` and no worktree is created.

#### Acceptance

- `run-next` before acceptance creates no worktree, no branch, and no pull request.
- Acceptance stores the accepting user's id and the set of requirement ids accepted. A caller who is not a Product Owner member is rejected.
- After acceptance, cards may be created for those ids only.

### REQ-004 A source edit invalidates acceptance for the changed ids

If a requirement statement, acceptance list, or decision statement changes after acceptance, that id is unaccepted. Cards linked to it do not keep running against the old text.

#### Acceptance

- Editing an accepted requirement's statement or acceptance list clears acceptance for that id.
- A card in `backlog` or `ready` linked to that id returns to `backlog`.
- A card already past `ready` moves to `blocked` with reason `source_changed`. It does not deploy and it does not open a new attempt until the operator accepts the new text and releases the card.
- Unchanged ids stay accepted.

### REQ-005 Cards carry scope, risk, checks, and links

Each card stores its id, title, requirement ids, decision ids, dependencies, risk (`low` or `high`), service name, status, local check, dev check, attempt count, max attempts, branch, pull request URL, approved SHA, deployed SHA, and environment. v1 environment is `dev`.

A check command may be empty only when the card is marked manual. A manual card still has a written acceptance list. An empty dev check means verification is the health check only, and the card says so.

#### Acceptance

- A card missing requirement ids, or missing acceptance while not marked manual, cannot enter `ready`.
- Risk accepts `low` and `high` only.
- `environment` is `dev` for every v1 card.
- The operator can read every field above from `orchestrator status`.

### REQ-006 Only legal transitions run, and each one has evidence

Card status changes only through the harness transition function. The legal edges are the ones below. Any other edge is rejected and logged. A model has no tool that sets status.

| From | To | Evidence |
| --- | --- | --- |
| `backlog` | `ready` | Requirements accepted. Dependencies done. Acceptance present, or the card is manual. |
| `ready` | `in_progress` | Lock held. Route stored. Worktree created. |
| `in_progress` | `self_check` | The attempt ended, and a new commit exists on the card branch. |
| `in_progress` | `blocked` | Budget, cap, or harness error, with a reason. |
| `self_check` | `in_progress` | Gating local check exit non-zero, and attempts remain. Failure note stored. The next attempt starts on the next pass. |
| `self_check` | `pr_open` | Local check exit 0. Pull request URL stored. |
| `self_check` | `blocked` | Attempts exhausted, or the check could not be run. |
| `pr_open` | `agent_review` | Reviewer route stored. Author model id stored. |
| `agent_review` | `changes_requested` | Review body stored. Verdict `request_changes`, or a required CI check failed. |
| `agent_review` | `human_review` | Review body stored. Verdict `recommend_approve`. Required CI green, or none required. |
| `agent_review` | `blocked` | Reviewer call failed after retry and fallback. |
| `changes_requested` | `in_progress` | New attempt id. Same branch. Approval cleared. Attempts remain. |
| `changes_requested` | `blocked` | Attempt cap reached. Reason `attempt_cap`. |
| `human_review` | `changes_requested` | Approver requested changes. Approval cleared. |
| `human_review` | `approved` | Allowlisted approver. Approval names the current head SHA. |
| `human_review` | `blocked` | Approver rejected the card. |
| `approved` | `deploying_dev` | Deploy lock held. Approved SHA equals pull request head. Deploy settings complete. |
| `approved` | `human_review` | Pull request head no longer equals the approved SHA. Approval cleared. |
| `approved` | `changes_requested` | Approver requested changes. Approval cleared. |
| `approved` | `blocked` | Approver rejected the card, or deploy settings are incomplete (`deploy_config`). |
| `deploying_dev` | `verifying` | Health passed for the deployed SHA. |
| `deploying_dev` | `blocked` | Deploy command failed or health failed. Rollback result stored. |
| `verifying` | `done` | Dev check exit 0, or a health-only card, against the deployed SHA. |
| `verifying` | `changes_requested` | Dev check exit non-zero, and attempts remain. Log stored. Approval cleared. |
| `verifying` | `blocked` | Dev check failed and the attempt cap is hit. |
| `blocked` | `ready` | Operator released the card. Linked requirements accepted. Reason is not a deploy or health reason. |
| `blocked` | `human_review` | Operator released a card blocked for `health`, `deploy_failed`, or `deploy_config`. Approval cleared. |
| `done` | none | Terminal. A follow-up is a new card. |

#### Acceptance

- A test suite covers every legal edge and at least the illegal edge `in_progress` to `deploying_dev`.
- A transition with missing evidence fails and writes `transition_rejected`.
- Calling the transition function as a model tool is impossible because that tool is absent from every model profile.
- `done` has no outbound edge.
- A transition whose stored status changed after it was read writes nothing and reports a conflict.

### REQ-007 A card waits until its dependencies are done

The planner proposes `dependsOn`. The harness enforces it. A card enters `ready` only when every dependency is `done` and REQ-003 acceptance holds.

#### Acceptance

- Given card B depends on card A, B does not enter `in_progress` while A is any status other than `done`.
- When A reaches `done`, a later `run-next` may start B.
- A dependency cycle is rejected at card create time and no card in the cycle starts.

### REQ-008 One writer holds a card

A card has one worktree and one lock. A second worker cannot edit that card's branch while the lock is held.

#### Acceptance

- Two overlapping `run-next` invocations leave a single `in_progress` attempt on the card.
- The second invocation does not create a second branch or a second worktree for that card.
- The lock is released when the attempt ends, when the card blocks, and when the process resumes a card that has no live worker.

### REQ-009 The scheduler starts ready work and serializes deploys

`run-next` starts cards that are `ready`, continues cards whose next harness step needs no model, and queues work for that project. Implementation worktrees may run in parallel up to a configured cap, inside one project. Deploys do not run in parallel inside one project. Two projects may deploy at the same time (RFC-048). A card in `approved` waits until a Lead Developer starts deploy.

#### Acceptance

- With the implementation cap set to 2, a third ready card in that project stays `ready` until one of the first two leaves `in_progress`.
- Two cards in one project that both reach `approved` deploy one after the other, and only after a Lead Developer starts each deploy.
- Two projects may deploy at the same time.
- `run-next` on an empty project exits 0 and reports that no card is ready.

### REQ-010 The router chooses a task class from card facts

The harness picks the task class. The classes are `plan`, `implement`, `implement_hard`, `review`, and `triage`. Deploy has no class and no model.

| Class | When |
| --- | --- |
| `plan` | After the deterministic parse, when ids are missing or cards are being proposed. |
| `implement` | A ready card with risk `low` and failure count 0. |
| `implement_hard` | Risk `high`, or failure count at least 1. |
| `review` | The pull request is open and required local checks are green. |
| `triage` | A check exited non-zero and the next attempt needs a failure note. |

#### Acceptance

- A low-risk card on its first attempt routes to `implement`.
- The same card after one failed check routes to `implement_hard`.
- A high-risk card routes to `implement_hard` on the first attempt.
- No route decision has task class `deploy`.

### REQ-011 Every route decision is stored

A route records task class, model id, fallback model id, tool profile, context pack, max turns, and the reason. The attempt stores the model that actually produced the result, plus the prompt hash.

Model names and families live in configuration, not in scattered conditionals. v1 configuration supports two families. Exact model names are config values.

#### Acceptance

- After any model call, the attempt row has a model id and a prompt hash.
- The event log has a `route` event for that attempt whose model id matches the call, or whose fallback id matches when the fallback was used.
- Changing the route config changes the next decision without a code edit.

### REQ-012 Provider failure retries once, then falls back, then blocks

On a transport or provider error, the harness retries the same model once, then calls the fallback model once. If both fail, the card moves to `blocked` and both errors are stored.

#### Acceptance

- A forced primary failure followed by a successful fallback records both calls and continues the card.
- A forced primary failure and a forced fallback failure ends in `blocked` with both errors visible in `orchestrator trace`.
- The harness does not loop on provider errors past that sequence.

### REQ-013 Review uses another model family when one is configured

The review route selects a model whose provider family differs from the author model. If only one family is configured, the review still runs, and the attempt records `review.independent = false`.

#### Acceptance

- With two families configured, the review model family differs from the author family on the attempt.
- With one family configured, the review runs and `review.independent` is false.
- The log never claims independence when the families match.

### REQ-014 A model cannot change its route or its budget

The implementer, reviewer, planner, and triage tools include no action that sets the model, raises `maxAttempts`, or raises the token cap.

#### Acceptance

- A tool list snapshot for each model profile excludes route changes and budget changes.
- A prompt that asks to switch model does not change the stored route decision.

### REQ-015 Budgets are configurable and they stop the card

v1 ships with these configurable defaults: 3 attempts per card, 12 model turns per attempt, 200000 tokens per card, 30 minutes of wall time per attempt. Hitting a cap moves the card to `blocked` with the cap name stored. The model cannot raise the cap.

An attempt is one implementer conversation. The current attempt ends, and counts, when its gating local check fails, when the agent or a person requests changes, when required CI fails, when its dev check fails, when it ends without a commit, or when it passes and moves on. Review, triage, and planner calls do not count against the attempt cap. Their tokens count against the token cap.

The operator may release a blocked card. A card blocked for `health`, `deploy_failed`, or `deploy_config` returns to `human_review` with approval cleared, so the same head can be re-approved and redeployed without a code change. Other cards return to `ready`. A card blocked for `source_changed` cannot be released until its linked requirements are accepted again. The default release keeps the attempts used. If that count has already reached the cap, release raises the card's cap by one so exactly one more attempt can run. The operator may instead reset the count. The release event records which.

These numbers are defaults, not measured usage.

#### Acceptance

- A fixture that exceeds the turn cap on an attempt ends in `blocked` with reason `turn_cap`.
- Token cap and wall-clock cap each have a fixture that ends in `blocked` with that reason.
- After `unblock` with the default policy, the next failure still counts against the existing attempt counter.
- Releasing a card blocked at the attempt cap allows exactly one more attempt.
- Releasing a card blocked for `health` returns it to `human_review` and starts no implementer attempt.
- Review and triage calls leave the attempt count unchanged.

### REQ-016 Each role receives a fixed context pack

Pack contents are fixed. A snapshot test locks each pack.

| Pack | Includes | Excludes |
| --- | --- | --- |
| Planner | PRD outline, requirement candidates, RFC decisions, existing card titles and dependencies | File diffs, credentials, attempt logs |
| Implementer | The one card, linked requirement statements, linked decisions, repo path map, local check command, and the last failure note if any | Other cards' diffs, deploy secrets |
| Reviewer | Linked requirements, linked decisions, the diff, local check output, pull request body | Author messages, author tool trace |
| Triage | Failing command, exit code, log tail, card acceptance | Any instruction or tool that edits files |

The log tail included in a pack is capped. The default cap is the last 200 lines.

#### Acceptance

- The reviewer pack fixture contains the diff and the requirement statements.
- The reviewer pack fixture contains neither the author transcript nor the author tool trace.
- The implementer pack for card A contains no diff from card B.
- A log longer than the cap is truncated in the pack, and the event records the truncation.

### REQ-017 Each role has a fixed tool allowlist

| Profile | Tools | Absent |
| --- | --- | --- |
| Planner | Read docs, propose requirements, create cards | Shell, git push, deploy, transition |
| Implementer | Read file, edit file, run the card's local check, commit inside the worktree | Push to the default branch, merge, deploy, review comment, transition |
| Reviewer | Read diff, comment on the pull request, verdict `recommend_approve` or `request_changes` | Edit, commit, merge, deploy, human approve |
| Triage | Read the log, write a failure note | Edit, commit, deploy |
| Deployer | Deploy the dev SHA, health check, rollback | Every model tool. The deployer is harness code. |

#### Acceptance

- Each profile's tool list matches the table in a snapshot test.
- An implementer turn that requests a deploy tool receives a tool error, and no deploy command runs.

### REQ-018 Check commands come from settings and run on a copy

`localCheck` and `devCheck` are copied from settings when the card is created and are visible on the card before the inner loop starts. No model tool can set or change them. The runner executes the command in the sandbox on a copy of the files, never on the worktree itself. The first word must be on the allowed-executables list, and shell syntax is refused. A shell string invented during a model turn is not executed.

#### Acceptance

- The command that runs is the command stored on the card.
- A model turn that emits a different shell command does not execute that command.
- An executable outside the allowlist fails the check with a harness error and does not run.
- A `create_card` call that includes a command leaves the card's checks equal to settings.
- A check cannot write the worktree's `.git` file.

### REQ-019 Deploy credentials stay out of model processes

The implementer, planner, reviewer, and triage environments have no deploy token. The deployer has the dev deploy credential and has no model API key. Repo file contents are untrusted input to models.

#### Acceptance

- A test inspects the implementer process environment and finds no deploy credential.
- A fixture repo file that instructs the model to deploy does not result in a deploy command.
- The deployer path is reachable only from the `approved` to `deploying_dev` transition.

### REQ-020 The inner loop works in a fresh worktree and commits there

Starting a card creates a git worktree from the configured base branch. The implementer edits and commits only inside that worktree. Commit messages include trailers `Orchestrator-Card`, `Orchestrator-Attempt`, and `Model`.

#### Acceptance

- The card branch is `orch/<card-id>`.
- Commits exist only on that branch, not on the base branch.
- A commit from the orchestrator contains all three trailers.
- The base branch in the target repo stays unchanged by the inner loop.

### REQ-021 A local check passes only on exit code 0

Inside an attempt the implementer may run the local check as a tool as often as its turn budget allows. Those runs inform the model and do not move the card. When the attempt ends with a new commit, the harness runs `localCheck` on the committed SHA. That run is the gating check. Exit 0 moves the card toward the pull request. A non-zero exit ends the attempt, stores the log and a failure note, and starts a new attempt while attempts remain. An attempt that ends without a commit also counts as a failed attempt. The triage model's prose cannot mark the check passed.

#### Acceptance

- Exit 0 on the fixture check allows `pr_open` once a pull request exists.
- Exit 1 stores a log and does not allow `pr_open`.
- A model reply that says the tests passed, while the exit code is 1, leaves the card out of `pr_open`.

### REQ-022 The inner loop has no deploy path

No state available to the inner loop can call deploy. Deploy is reachable only from `approved`.

#### Acceptance

- From `in_progress` and `self_check`, the deploy function is not called, including when the model asks for it.
- A trace of a card that never reached `approved` contains no `deploy` event.

### REQ-023 A new attempt starts a new conversation

An attempt receives its context pack and, when relevant, the previous failure note. It does not receive the previous model transcript.

#### Acceptance

- The second attempt's model request body excludes the first attempt's messages.
- The second attempt's pack includes the failure note produced after the first failure.
- `parentAttemptId` on the second attempt points at the first.

### REQ-024 One pull request per card, updated in place

The first time local checks pass, the harness opens one pull request for the card. Later attempts push to `orch/<card-id>` and update that pull request. The idempotency key is the card id.

#### Acceptance

- Running the open step twice results in one pull request URL on the card.
- A second attempt adds commits to the same branch and the same pull request.
- The card never stores two open pull request URLs.

### REQ-025 The harness owns the pull request metadata

The harness writes a metadata section on the pull request body: requirement ids, decision ids, local check command, attempt number, and model id. The implementer may write a summary section. On each push, the harness rewrites the metadata section.

#### Acceptance

- After a push, the metadata section matches the card record.
- A summary section written earlier is still present after the metadata rewrite.
- Requirement ids in the body match `requirementIds` on the card.

### REQ-026 Required CI must be green before human review

The pull request runs the same local check on CI. Local exit 0 is not sufficient to enter `human_review` while a required check is still failing or pending. A pending check leaves the card in `agent_review` until it resolves. A failed required check moves the card to `changes_requested`, and the failure note names the failed checks.

#### Acceptance

- With required CI red, the card does not enter `human_review`.
- With required CI green, and a reviewer verdict of `recommend_approve`, the card can enter `human_review`.
- A repository with no required checks configured may enter `human_review` on local exit 0 plus the reviewer verdict, and the card records that CI was not required.

### REQ-027 The reviewer model sees a cold pack

The reviewer is a different role from the author. Its pack is the reviewer pack in REQ-016. Early human comments on the pull request are included. The author transcript is not.

#### Acceptance

- The stored reviewer prompt contains the diff, the linked requirements, the check output, and any human review comments already on the pull request.
- The stored reviewer prompt excludes the author transcript and author tool calls.
- This is asserted from the trace, not from the review text.

### REQ-028 Review verdicts map to two states

`request_changes` moves the card to `changes_requested`. `recommend_approve` moves the card to `human_review`. Any other model output does not move the card. The review body is stored either way.

#### Acceptance

- Each verdict maps to the state above in a test.
- A review body with no parseable verdict leaves the card in `agent_review` and records a parse failure.
- Repeated parse failure follows REQ-012 and then blocks.

### REQ-029 The reviewer model cannot approve, edit, or deploy

The reviewer verdict is a recommendation. It is not the approval gate in REQ-030.

#### Acceptance

- A `recommend_approve` verdict leaves `approvedSha` empty.
- No deploy event is written as a result of the reviewer verdict alone.
- The reviewer tool list matches REQ-017.

### REQ-030 Human approval is a permitted member and one SHA

A Lead Developer, QA Lead, or Product Owner who is a member of the project may approve the current pull request head. The approval stores that user's GitHub login and the SHA. The user must have a GitHub login saved, and that login must not be the project bot. The orchestrator bot identity is rejected even if the git host would accept its approval.

v1 accepts the approval from the CLI and from the web UI. Both call the same service. A git host webhook may deliver the same facts later, through the same checks. Approval does not start deploy (REQ-050).

#### Acceptance

- Approval by a permitted member, for the current head SHA, moves the card to `approved` and stores that SHA. It does not start deploy.
- Approval by a Developer, Senior Developer, or QA is rejected and the card stays in `human_review`.
- Approval by the bot login, or by a member with no GitHub login, is rejected and the card stays in `human_review`.
- Approval that names a SHA other than the current head is rejected.
- Approving the same SHA twice, from the CLI, the UI, or both, results in one approval record.

### REQ-031 A new commit clears approval

If the pull request head moves after approval and before deploy starts, the harness clears `approvedSha` and returns the card to `human_review`. The harness refreshes the head from the git host before deploy and on each worker pass. A push after deploy starts is not covered by that card's verification. The deploy record keeps the SHA that shipped.

#### Acceptance

- A push after `approved` and before `deploying_dev` clears `approvedSha` and sets status to `human_review`.
- The next deploy does not run until a new approval names the new head.
- The cleared approval remains in the event log.

### REQ-032 Request-changes and reject are explicit

An approver may request changes or reject a card in `human_review` or `approved`. Request-changes clears any approval and moves the card to `changes_requested`. Reject moves the card to `blocked` with reason `rejected`.

#### Acceptance

- Request-changes on an approved SHA clears `approvedSha` and does not deploy.
- Reject does not deploy and does not start a new attempt until a member allowed to unblock releases the card.
- Both actions record the approver identity.

### REQ-033 Deploy sends the approved SHA to dev

Deploy runs the configured dev command with the approved SHA. The harness records the SHA it asked for, the SHA the deploy reports, the previous dev SHA, and the dev URL. v1 does not deploy the default branch tip in place of the approved SHA.

The deploy command, rollback command, health URL, and dev host list come from orchestrator settings (REQ-044). The two commands are absolute paths to operator-owned files outside the target repo and the data directory, so no pull request can change what deploy runs. The health URL host must be on the dev host list. v1 does not embed a cloud provider SDK. A missing command, health URL, or dev host list blocks the card with reason `deploy_config` before any deploy call. A deploy command that exits non-zero runs rollback and blocks the card with reason `deploy_failed`.

#### Acceptance

- The argument passed to the dev command is `approvedSha`.
- The deploy record stores requested SHA, deployed SHA, previous SHA, and URL.
- A card whose head differs from `approvedSha` never reaches the dev command (REQ-031).
- Missing `devCommand`, `rollbackCommand`, `healthUrl`, or dev hosts blocks the card and does not invoke a command.
- A deploy command path inside the target repo is refused when settings are saved and again before deploy.

### REQ-034 One deploy runs at a time in a project

Each project takes one deploy lock. A second approved card in that project waits. Another project's deploy is not blocked by that lock. Parallel worktrees are unaffected. Deploy starts only when a Lead Developer member requests it (REQ-050).

#### Acceptance

- Two approved cards in one project produce non-overlapping deploy events.
- Two approved cards in different projects may deploy at the same time.
- A card that is still in `in_progress` can proceed while another card in the same project holds the deploy lock.

### REQ-035 Failed health rolls back and blocks

After deploy, the harness probes the configured health URL. Failure runs the rollback command for the previous SHA, stores the rollback result, and moves the card to `blocked`. The failure is not converted into a product-test failure and does not start an implementer attempt by itself.

#### Acceptance

- A stub health probe that fails causes one rollback call and status `blocked` with reason `health`.
- No `verify` event is written for that attempt.
- The previous SHA is the rollback target recorded before the deploy.

### REQ-036 Dev tests run only after health passes

On health success the harness sets `deployedSha` and runs `devCheck` on a fresh export of that SHA, with the dev URL available to the command. The worktree is not mounted. Empty `devCheck` means verification is health only. Exit 0 moves the card to `done`. A card with an empty dev check moves to `done` on health success and records that verification was health only.

The harness grades the test. Exit code is the signal.

#### Acceptance

- `devCheck` is not started while health is failing.
- Exit 0 after health success sets status `done` and `deployedSha` to the approved SHA.
- Exit non-zero does not set status `done`.
- A health-only card reaches `done` without a dev command and the verify record says `health_only`.
- The files the dev check sees match the deployed SHA, even when the worktree has later edits.

### REQ-037 A failed check produces a triage note for the next attempt

When a local check or a dev check exits non-zero, and attempts remain, the harness stores the log and calls the triage role to write a failure note. The note includes the command, the exit code, and a short suspected cause. The next implementer attempt receives that note (REQ-023). Triage does not edit files and does not change the pass/fail result.

#### Acceptance

- After a failing dev check with attempts left, a failure note exists and the card is `changes_requested`.
- The following implementer pack contains that note and the exit code.
- The triage call does not create a commit.

### REQ-038 The outer loop repeats until done or blocked

A failed dev test with attempts remaining clears approval and returns the card to `changes_requested` on the same pull request. The next `run-next` starts a new attempt. When the attempt cap is hit, the card moves to `blocked` and waits for the operator. When the dev test passes, dependent cards become eligible under REQ-007.

#### Acceptance

- A failing dev test on attempt 1 of a card whose max attempts is 3 returns to `changes_requested` and keeps the same pull request URL.
- Attempt 3 failing moves the card to `blocked` and does not open another attempt.
- A passing dev test sets the card to `done`, and a dependent ready card can start on a later `run-next`.

### REQ-039 An append-only event log explains the card

The log records route, model call, tool call, gate, transition, rejected transition, deploy, health, verify, budget stop, settings change, and failed harness assertion. An operator can answer, from the log, which model wrote the pull request, which SHA was approved, which SHA was deployed, and which log closed the card.

#### Acceptance

- `orchestrator trace CARD` prints those events in time order.
- Events are not updated in place. A correction is a new event.
- A completed happy-path card has at least one event of kinds `route`, `transition`, `deploy`, `health`, and `verify`.

### REQ-040 Restart resumes from the last gate

On startup the harness loads each unfinished card and its latest attempt and gate. If a deploy started and health was never recorded, the harness probes health or rolls back before any new model call or any new deploy. It does not replay model turns that already committed.

#### Acceptance

- Killing the process after a commit and before pull request creation resumes without a second commit of the same work, and continues toward `pr_open`.
- Killing the process after deploy starts and before health is recorded does not call deploy a second time until health is reconciled.
- A card in `done` is not restarted.

### REQ-041 The log stores model usage and redacts secrets

Each model call event stores model id, prompt hash, input tokens, output tokens, and stop reason. Known token and key patterns are redacted before append. Deploy credentials and model API keys are not written to the log.

#### Acceptance

- A model call event contains the five usage fields.
- A fixture prompt that contains a configured secret pattern is stored with the secret replaced.
- Searching the log fixture for the raw deploy token finds no match.

### REQ-042 The CLI and the local web UI share one service layer

v1 ships these commands:

| Command | Result |
| --- | --- |
| `orchestrator ingest` | Runs REQ-001 and, when needed, REQ-002. |
| `orchestrator accept` | Records REQ-003 for the current draft. |
| `orchestrator run-next` | Advances the board per REQ-009. When `serve` is running, it queues the run for the server. |
| `orchestrator serve` | Starts the JSON API and one worker per project. Without `ORCHESTRATOR_CLOUD`, the API binds loopback. With `ORCHESTRATOR_CLOUD=1`, it may bind the configured address (RFC-045). |
| `orchestrator users bootstrap` | Creates the first admin when the user table is empty (REQ-046). |
| `orchestrator status` | Lists cards, status, attempt, pull request URL, approved SHA, deployed SHA. |
| `orchestrator status CARD` | Prints one card, including checks, requirement ids, and the latest failure reason. |
| `orchestrator trace CARD` | Prints the event log for the card. |
| `orchestrator approve CARD --sha SHA` | REQ-030. |
| `orchestrator request-changes CARD` | REQ-032. |
| `orchestrator reject CARD` | REQ-032. |
| `orchestrator unblock CARD [--reset-attempts]` | Releases a blocked card to `ready` or `human_review` per REQ-006 and REQ-015. |

The local web UI offers the same actions by calling the same service functions through the JSON API. It shows a board, card detail with trace, intake, human gates, and settings. It renders only the actions the API lists for a card, and it never writes status on its own.

#### Acceptance

- Each command exists and returns a non-zero exit code on rejected gates, with the reason printed.
- `status` with no cards prints that the board is empty and names `ingest` as the next command.
- `approve` without `--sha` is rejected.
- A card in `in_progress` offers no approve action in either client.
- An action the API does not list for a card is rejected when called directly.

### REQ-043 The fixture script is the release check

v1 is acceptable when the following script passes on a fixture repo and the same script's deploy and health steps pass against one real dev target after the fixture deploy stub has passed.

1. Ingest a PRD with two requirements. Implementation waits until accept.
2. Two cards exist. The second depends on the first.
3. Card 1 gets a worktree, a local check exit 0, and one pull request.
4. The reviewer model is the other configured family, and its stored prompt has no author transcript.
5. Approving the head SHA allows deploy. A push before deploy returns the card to human review and clears the SHA.
6. A failing health probe rolls back and blocks. No dev test runs. Release returns the card to human review, and the same head is re-approved.
7. A healthy deploy with a failing dev test opens a new attempt on the same pull request and carries the log.
8. A healthy deploy with a passing dev test marks card 1 `done` and only then allows card 2 to start.
9. Killing the process during deploy and starting again reconciles health before any second deploy.

#### Acceptance

- The nine steps are automated against the fixture.
- Step 6 uses a stub health endpoint.
- The real dev target is not required for the fixture suite to pass.

### REQ-044 Settings are edited through the API, and secrets stay write-only

A Lead Developer member edits project settings from the web UI, through the JSON API. Each user edits their own GitHub settings. There is no approver list and no operator login.

| Section | Fields | Who saves |
| --- | --- | --- |
| Account GitHub | Host, login, token | That user |
| Project GitHub | Host, owner, repo, base branch, bot login, bot token | Lead Developer member |
| LLM | Families, each with protocol, base URL, model ids, and an API key | Lead Developer member |
| Router | Model and fallback per task class, review independence, budgets | Lead Developer member |
| Deploy | Dev URL, dev hosts, deploy and rollback commands, health URL, deploy token, check commands, allowed executables, sandbox engine and image, implementation cap, doc paths | Lead Developer member |

No response contains a secret value. A response says whether each secret is set. Saving with an empty secret field keeps the stored value. Clearing a secret is a separate explicit action. A saved change applies to the next job of that project and never to an attempt in flight. The project bot login cannot approve. A settings change writes an event that names the section and the fields, never the values.

#### Acceptance

- `GET` on each settings section returns `tokenSet` or `keySet` flags and no secret.
- Saving the account GitHub section with an empty token keeps the stored token.
- A Developer member who saves project deploy settings is rejected.
- A router model id that no family lists is rejected.
- An implementer attempt running while the router is saved finishes on the route it started with.

### REQ-045 Orchestrator state lives outside the target repo

The service data directory holds `secrets.json` with `databaseUrl` and `tokenKey`. Each project has its own directory under that root, outside that project's repo, for settings, project secrets, the worker lock, worktrees, check exports, and logs. Every project uses the one PostgreSQL database named by `databaseUrl` (RFC-044, RFC-047). Startup refuses a project data directory inside that project's repo, and a repo inside its project data directory. The service directory and each project directory are mode `0700`. Secret files are mode `0600`.

#### Acceptance

- Starting with a project data directory inside that project's repo exits non-zero before opening the database.
- After a full fixture run, `git status` in the fixture repo shows no orchestrator files.
- The service secrets file is created with mode `0600`.
- Two projects in one database do not share a worker lock or a deploy lock.

### REQ-046 A person signs in with email and password

An admin creates a user with an email, a password, and one job role. The user signs in with that email and password and receives a bearer. The bearer names that user. A password shorter than 12 characters is rejected. Five bad passwords for one email pause that email for one minute. `orchestrator users bootstrap` creates the first admin from `ORCHESTRATOR_ADMIN_EMAIL` and `ORCHESTRATOR_ADMIN_PASSWORD` when no user exists. The last admin cannot be removed or disabled.

#### Acceptance

- A correct email and password returns a bearer. Later requests with that bearer run as that user.
- A wrong password returns `401` and does not return a bearer.
- The sixth bad password for that email inside the pause window returns `429`.
- A bearer older than 12 hours is `401`.
- Disabling the last admin is rejected.
- A non-admin who creates a user is rejected.

### REQ-047 A job role and an admin flag gate human actions

The server allows a human action only when the caller is a member of the project's tribe and the job role allows it, as in RFC-046. Admin does not add approve or deploy. The `actions` list on a card is that allowed subset. The same check runs on the POST that would perform the action.

#### Acceptance

- A Product Owner member can accept requirements. A Lead Developer member cannot.
- A Lead Developer member can start deploy from `approved`. A Product Owner member who approved the same SHA cannot.
- A Developer member can start a run and cannot approve, reject, request changes, or deploy.
- A QA Lead member can approve. A QA member cannot.
- A Senior Developer member can request changes and cannot approve.
- An action omitted from `actions` is rejected when called directly.

### REQ-048 A tribe has members

An admin creates a tribe and adds users to it. A user may be in more than one tribe. Removing a member hides that tribe's projects from them. The membership row is the only visibility grant.

#### Acceptance

- An admin can create a tribe and add a user.
- A non-admin cannot create a tribe or change its members.
- A user in two tribes is a member of both.

### REQ-049 A project is visible to its tribe

An admin creates a project on one tribe. The project names one target repo. Members of that tribe can list and open it. Anyone else receives `404`, including for an id that does not exist. Cards, intake, runs, and project settings for that project are included in that rule.

#### Acceptance

- A member lists the project and its cards.
- A user outside the tribe receives `404` for that project and for one of its cards.
- An unknown project id returns the same `404`.
- The same requirement id may exist on two projects.
- The same branch name may exist on two projects.

### REQ-050 The worker runs per project

Each project has its own worker lock and its own deploy lock. The service may run those workers at the same time. Inside one project, implementation stays under that project's cap, and one deploy runs at a time. `approved` does not deploy by itself.

#### Acceptance

- Two projects can have an implement job in progress at the same time.
- A card left in `approved` does not move to `deploying_dev` until a Lead Developer member starts deploy.
- Reconcile on one project does not clear the other project's deploy lock.

### REQ-051 Each user has their own GitHub settings

Each user stores a GitHub host, login, and token. The token is write-only. The project stores owner, repo, base branch, bot login, and bot token for the worker. The worker pushes and opens pull requests with the project bot token. It does not use the signed-in user's token. A user can test their own token. The API returns whether a token is set and never returns the token.

#### Acceptance

- `GET` of the caller's GitHub settings returns `tokenSet` and no token.
- One user cannot read or replace another user's token.
- A job's push uses the project bot token.
- Approval is rejected when the caller's GitHub login is empty.

## Non-goals

- NG-001 Production deploy, and any credential that can reach production.
- NG-002 Auto-merge. Merging to the default branch is a separate human action. Dev deploy does not merge.
- NG-003 More than one writer on a card.
- NG-004 A model choosing card status, model route, or budget.
- NG-005 A preview environment per pull request. Deploy records include `environment` and `url` so a later version can change the lock key without changing gate order. Each project uses one shared dev server.
- NG-006 A UI that writes card status itself, including drag and drop between columns. The web UI shows status and calls the same transitions as the CLI.
- NG-007 The model grading its own tests. Exit code and the health probe are the oracles.
- NG-008 Continuing a model conversation across attempts.

## Configuration defaults

Project settings live in that project's `config.yaml` and secrets (REQ-045). A Lead Developer member edits them from the settings pages (REQ-044). Each user edits their own GitHub settings (REQ-051). The full shape is in [tech/0002-data-and-api.md](../tech/0002-data-and-api.md). The defaults that shape behavior:

| Setting | v1 default |
| --- | --- |
| Attempts per card | 3 |
| Model turns per attempt | 12 |
| Tokens per card | 200000 |
| Wall minutes per attempt | 30 |
| Parallel implementation cap | 2 |
| PRD and RFC paths | `docs/prd`, `docs/rfc` |
| Review independence | On. The reviewer is another family than the author when one is configured |
| Sandbox engine | Podman |
| Environment | `dev` only |

Model ids, the project bot login, dev hosts, deploy and rollback scripts, the sandbox image digest, and check commands have no default. Each is chosen per project. A card whose job needs a missing setting blocks with a reason that names the section.

Git host default is GitHub. The card store is PostgreSQL, reached by `databaseUrl` in the data directory (RFC-044). The git host stores the pull request. It does not store card status as the source of truth.

## Flows

### Happy path

A member runs `ingest` from the CLI or the Intake page. A Product Owner member reads the draft and accepts it. The planner has written two cards, B depending on A. `run-next` locks A, routes `implement` or `implement_hard`, and creates `orch/A`. The inner loop commits and the local check exits 0. One pull request opens. CI goes green. The reviewer model recommends approval. A permitted member approves the head SHA from the CLI or the card page. A Lead Developer member starts deploy. The deployer ships that SHA, health passes, the dev check exits 0, and A is `done`. A later `run-next` may start B.

### Reviewer requests changes

The reviewer verdict is `request_changes`. The card returns to `changes_requested`. The next attempt is a new conversation with the review body in the failure note. The branch and the pull request stay the same. Approval is empty.

### Approver requests changes, or the branch moves

`request-changes`, or a push after approval, clears `approvedSha`. Deploy does not run. The card waits in `changes_requested` or `human_review` as specified in REQ-031 and REQ-032.

### Health failure

Deploy runs, the probe fails, rollback runs, the card is `blocked` with reason `health`. The dev check does not run. The operator inspects the trace and may release the card. Release returns it to `human_review`, so the same head can be re-approved and redeployed without a new code attempt.

### Dev test failure, then pass

Health passes and the dev check exits non-zero. Triage writes a note. The card returns to `changes_requested` with approval cleared. A new attempt pushes to the same pull request. After a later approval, deploy, healthy probe, and exit 0, the card is `done`.

### Cap, then release

The turn, token, wall-clock, or attempt cap blocks the card. `run-next` does not start it. `unblock` moves it to `ready`. When the attempt count is already at the cap, the default release grants one more attempt (REQ-015).

### Crash during deploy

Startup sees a deploy without a health event. It probes or rolls back before opening a model call and before calling deploy again.

## Glossary

| Term | Meaning |
| --- | --- |
| Harness | The state machine, locks, credentials, check runner, and deployer. Models sit inside it. |
| Card | One unit of work on the board, linked to requirement ids. |
| Attempt | One implementer conversation on a card. A new attempt is a new conversation. Review and triage calls attach to the attempt they serve. |
| Gate | Evidence the harness requires before a transition, such as exit code 0 or an approval record. |
| Context pack | The fixed set of documents and fields a model profile receives. |
| Inner loop | Edit, local check, and commit in the worktree. It ends at one pull request. |
| Outer loop | Review, human approval, dev deploy, health, and dev tests. |
| Cold review | A review whose prompt excludes the author's transcript. |
| Job role | One of Lead Developer, Senior Developer, Developer, QA Lead, QA, and Product Owner. Not a model profile. |
| Tribe | A team. Members see that tribe's projects. |
| Project | One target repo and its cards, settings, and worker. |
| Data directory | The service root, plus one directory per project outside that project's repo. The board is the PostgreSQL database named by `databaseUrl` in the service secrets file. |
| Export | A fresh copy of one SHA, or of the working tree without `.git`, that a check runs against. |

## Reading order for implementation

Build and test in this order. A later step is not a reason to skip an earlier gate.

1. Transitions, rejected edges, and the event log (REQ-006, REQ-039).
2. Parse, accept, and cards that stay in `backlog` (REQ-001, REQ-003, REQ-005).
3. Worktree, commit, and exit-code checks on a fixture repo (REQ-020, REQ-021).
4. One implementer model with the implementer pack and no deploy tool (REQ-016, REQ-017, REQ-022).
5. Idempotent pull request (REQ-024, REQ-025).
6. Cold reviewer and verdict mapping (REQ-027, REQ-028).
7. SHA-bound approval (REQ-030, REQ-031).
8. Deploy stub, health failure, and dev-check pass and fail (REQ-033, REQ-035, REQ-036).
9. Router, fallback, and family flag (REQ-011, REQ-012, REQ-013).
10. Dependencies and the per-project deploy lock (REQ-007, REQ-034), then the fixture script (REQ-043).
11. The data directory and the settings API (REQ-045, REQ-044), then the web UI (REQ-042).
12. Accounts, tribes, and per-project workers (REQ-046 through REQ-051).
