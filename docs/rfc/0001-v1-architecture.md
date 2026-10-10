# RFC-0001: AI Coding Orchestrator v1 architecture

Status: Accepted for v1. Date: 2026-10-10.

This RFC records the decisions that the v1 PRD depends on. Hardening decisions RFC-024 through RFC-034 are in [0002-hardening.md](0002-hardening.md). Decisions RFC-035 through RFC-043, including the ones that supersede RFC-016 and part of RFC-020, are in [0003-review-corrections.md](0003-review-corrections.md). RFC-044, which supersedes the store engine in RFC-002 and RFC-022 and the SQLite phrase in RFC-036, is in [0004-postgres.md](0004-postgres.md). RFC-045 through RFC-048, which supersede parts of RFC-006, RFC-010, RFC-018, RFC-030, RFC-032, RFC-035, RFC-036, RFC-043, and RFC-044, are in [0005-teams-and-cloud.md](0005-teams-and-cloud.md). Requirements and acceptance checks live in [docs/prd/coding-orchestrator.md](../prd/coding-orchestrator.md). Implementation guides live in [docs/tech/](../tech/). [docs/README.md](../README.md) lists every document and which one wins when two disagree.

Decision headings use `RFC-001` and up. The first paragraph under each heading is the statement. A later RFC supersedes a decision by naming its id. It does not rewrite the statement in place.

## Decision index

| Id | Decision |
| --- | --- |
| RFC-001 | The harness is the only writer of card status |
| RFC-002 | The card store is the source of truth. Store engine superseded by RFC-044 |
| RFC-003 | The inner loop and the outer loop stay separate |
| RFC-004 | Exit codes and the health probe grade the work |
| RFC-005 | One card has one worktree, one branch, and one pull request |
| RFC-006 | Approval binds an allowlisted person to one SHA. Allowlist sentence superseded by RFC-046 |
| RFC-007 | The reviewer does not see the author transcript |
| RFC-008 | Review prefers a different provider family |
| RFC-009 | Deploy is harness code, and v1 stops at dev |
| RFC-010 | One global lock covers the shared dev server. Service-wide lock superseded by RFC-048 |
| RFC-011 | Failed health rolls back and blocks |
| RFC-012 | A new attempt is a new conversation |
| RFC-013 | Packs and tool lists are fixed per role |
| RFC-014 | A model turn cannot supply the shell that runs |
| RFC-015 | Startup reconciles an in-flight deploy first |
| RFC-016 | The v1 control surface is the CLI. Superseded by RFC-035 |
| RFC-017 | The router is a function of card facts and config |
| RFC-018 | A person accepts extracted ids before any worktree. Who accepts superseded by RFC-046 and RFC-053. Manual ids added by RFC-053 |
| RFC-019 | An edit to an accepted requirement stops affected cards |
| RFC-020 | Budgets end in blocked, and models cannot raise them. Release sentence superseded by RFC-041 |
| RFC-021 | The event log is append-only and redacted |
| RFC-022 | v1 edges are GitHub, markdown, SQLite, and a deploy command. Board engine superseded by RFC-044 |
| RFC-023 | Accepted decision statements stay stable |

## RFC-001 The harness is the only writer of card status

Card status changes only inside the harness transition function. Models propose edits, review comments, and failure notes. A model tool list has no status write.

The board has to survive a crashed process and a new session. A chat transcript cannot reject an illegal edge such as `in_progress` to `deploying_dev`. Putting the edges in code makes that rejection a test.

Set aside: one long agent session whose prompt says which status comes next. A session can skip a gate, and a restart has nothing solid to resume from.

Consequences: every PRD edge has a precondition check and a unit test. `transition_rejected` is an event. Adding a status means adding edges here and in REQ-006 together.

Requirements: REQ-006, REQ-017.

## RFC-002 The card store is the source of truth

SQLite owned by the orchestrator stores cards, attempts, gates, approvals, and events. GitHub stores the pull request and its comments. A kanban product, including a future UI, may mirror the store. It may not be the writer that moves a card.

The git host does not know the deploy lock, the approved SHA, or the attempt cap. If those live only on the host, the harness cannot enforce them.

Set aside: GitHub Projects, or issues, as the board. Useful as a mirror later. Wrong as the authority, because a person or a bot can drag a card into a column the preconditions forbid.

Consequences: `orchestrator status` reads SQLite. Webhooks update the store through the same transition checks as the CLI. Losing the SQLite file loses the board even if the pull requests remain.

Requirements: REQ-005, REQ-039, REQ-042.

Superseded in part by RFC-044 in [0004-postgres.md](0004-postgres.md). The store engine in the statement, and the SQLite sentences in Consequences, no longer apply. The card store is still the harness database. GitHub is still not the board.

## RFC-003 The inner loop and the outer loop stay separate

The inner loop edits a worktree, runs the local check, and stops when one pull request is open. The outer loop starts at review. Deploy runs only on the outer loop, after a person has approved the head SHA.

Deploy from the inner loop would ship a commit no person has approved. Keeping the loops in separate code paths makes that path unreachable.

Set aside: a single worker that deploys when it believes the tests passed. That worker holds too many credentials, and its prompt becomes the approval gate.

Consequences: the implementer process has no deploy tool and no deploy credential. A trace for a card that never reached `approved` contains no `deploy` event. Features that want to deploy earlier need a new RFC.

Requirements: REQ-020, REQ-022, REQ-033.

## RFC-004 Exit codes and the health probe grade the work

A local check or a dev check passes when the process exits 0. Health passes when the configured probe succeeds. The triage model may write a failure note. Its prose cannot flip a failure into a pass.

Models are rewarded for sounding finished. The grade has to be a fact the harness recorded.

Set aside: asking the implementer to report whether the suite passed. That report can disagree with the process, and the disagreement is hard to see in review.

Consequences: `run_local_check` and the dev runner capture the exit code and the log. Snapshot tests include a case where the model says the tests passed and the exit code is 1. The card stays out of `pr_open` or `done`.

Requirements: REQ-021, REQ-035, REQ-036.

## RFC-005 One card has one worktree, one branch, and one pull request

The branch is `orch/<card-id>`. The pull request is created once, keyed by card id. Later attempts push to that branch and update that pull request. A lock stops a second worker from using the same card.

Retries have to accumulate on a reviewable branch. A second pull request splits the comment thread and makes the approved SHA ambiguous.

Set aside: a fresh branch and a fresh pull request per attempt. Clean diffs, and a lost review history. Also set aside: several agents committing to one card. The lock would be a lie.

Consequences: create and update are idempotent on card id. Commit trailers `Orchestrator-Card`, `Orchestrator-Attempt`, and `Model` identify the writer. The base branch is not updated by the inner loop.

Requirements: REQ-008, REQ-020, REQ-024.

## RFC-006 Approval binds an allowlisted person to one SHA

`approved` stores the approver identity and the pull request head SHA. The approver login is on the repo allowlist. The orchestrator bot is not on that list, even if GitHub would accept the bot's review as approval. A push that moves the head clears `approvedSha` and returns the card to `human_review`.

Dev deploy has to mean "this commit". Approving a branch, or approving as the bot that wrote the branch, deploys work nobody has looked at.

Set aside: treating the reviewer model's `recommend_approve` as permission to deploy. The recommendation is a comment. Also set aside: approving the branch name without a SHA. The branch moves.

Consequences: `orchestrator approve CARD --sha SHA` checks the allowlist and the head. A webhook must pass the same checks. Deploy reads `approvedSha` and compares it to the head again inside the deploy lock. CLI and webhook share one approval record.

Requirements: REQ-029, REQ-030, REQ-031.

Superseded in part by RFC-046 in [0005-teams-and-cloud.md](0005-teams-and-cloud.md). The allowlist sentence no longer decides who may approve. The SHA binding and the bot exclusion stay.

Superseded in part by RFC-054 in [0009-card-owner.md](0009-card-owner.md). The card owner is also excluded from approving their own card. The pull request is opened by the owner, not the bot.

## RFC-007 The reviewer does not see the author transcript

The reviewer pack contains the linked requirements, the linked decisions, the diff, the check output, the pull request body, and human comments already on the pull request. It excludes the author's messages and tool trace.

A reviewer that reads the author's story tends to repeat it. The point of a second model is a check against the requirements and the diff.

Set aside: passing the implementer thread through "for context". It saves a summary step and weakens the review.

Consequences: the reviewer pack has a snapshot test. The trace stores the reviewer prompt so REQ-027 can be asserted from evidence. Putting the transcript back is a failed test, not a prompt tweak.

Requirements: REQ-016, REQ-027.

## RFC-008 Review prefers a different provider family

When two families are configured, the review model is chosen from the family that did not author the attempt. When only one family is configured, review still runs, and the attempt stores `review.independent = false`.

Same-model review shares blind spots with the author. Hiding that fact in the log makes the board look safer than it is.

Set aside: always blocking until a second vendor is configured. Correct in a strict shop, and it stops v1 on a machine with one API key. The flag keeps the gap visible.

Consequences: route config names families, not only model ids. The router records the family comparison on the review attempt. A log line must not say the review was independent when the families match.

Requirements: REQ-011, REQ-013.

## RFC-009 Deploy is harness code, and v1 stops at dev

The deployer is a harness component. It has no model. It runs the configured dev command with the approved SHA, then the health probe, then rollback when health fails. v1 has no production credential and no production command.

A model with a deploy tool can be talked into shipping whatever the repo file asks for. Production in the same process as experiments will eventually be called.

Set aside: an agent tool named `deploy` with a prompt rule that says "dev only". The rule is not a credential boundary. Also set aside: auto-merge to the default branch after a green dev test. Merge stays a human action outside this loop.

Consequences: deploy credentials exist only on the deployer path. Model environments do not. `environment` on a v1 card is `dev`. A production target requires a new RFC and a new credential boundary, not a new config string in the same command.

Requirements: REQ-017, REQ-019, REQ-033. Non-goals: NG-001, NG-002.

## RFC-010 One global lock covers the shared dev server

v1 deploys to one shared dev environment. A single deploy lock serializes those deploys. Implementation worktrees may still run in parallel up to the configured cap. The card's `service` field is stored now. v1 does not lock per service.

Two deploys to one server overwrite each other, and the health probe cannot say which SHA is live.

Set aside: a preview environment per pull request in v1. The deploy record already keeps `environment` and `url`, so a later RFC can change the lock key without changing gate order. Building previews now delays the rollback path.

Consequences: two cards in `approved` deploy one after the other. A card in `in_progress` is not blocked by the deploy lock. Operators waiting on a queue see it in `status`.

Requirements: REQ-009, REQ-034. Non-goal: NG-005.

Superseded in part by RFC-048 in [0005-teams-and-cloud.md](0005-teams-and-cloud.md). The lock is per project. One project still deploys one card at a time.

## RFC-011 Failed health rolls back and blocks

If the health probe fails, the harness runs the rollback command with the SHA recorded before deploy, stores the result, and moves the card to `blocked` with reason `health`. It does not start `devCheck`. It does not open an implementer attempt from that failure.

A down server is not a product-test failure. Sending it to the implementer produces an unrelated code change.

Set aside: treating health failure as a normal test failure and looping. The next attempt would edit application code to fix a bad release.

Consequences: rollback is mandatory config beside `devCommand` and `healthUrl`. Missing any of the three blocks the card with `deploy_config` before a deploy call. The operator reads the trace and releases the card if a retry is warranted.

Requirements: REQ-033, REQ-035.

## RFC-012 A new attempt is a new conversation

Each attempt has its own model request. The next attempt receives the context pack and the failure note from triage or review. It does not receive the previous transcript. `parentAttemptId` links the chain.

Transcripts grow, repeat bad guesses, and hide which instruction applied. The card, the diff, and the log are enough to continue.

Set aside: appending to the same chat until the card is done. Easier to code, and resume after a crash depends on a private transcript store.

Consequences: triage must leave a note that includes the command, the exit code, and a short suspected cause. The log tail in the pack is capped, default the last 200 lines. Implementer packs are tested to exclude prior messages.

Requirements: REQ-016, REQ-023, REQ-037. Non-goal: NG-008.

## RFC-013 Packs and tool lists are fixed per role

Planner, implementer, reviewer, and triage each have one context pack and one tool profile, as listed in the PRD. The deployer has commands and no pack. Snapshot tests lock both. A change to a pack or a tool list is a deliberate release, stored by prompt hash and by the test diff.

If each call builds its own context, the reviewer slowly gains the author transcript and the implementer slowly gains deploy.

Set aside: a shared "agent" profile with every tool, and a prompt that tells the model which ones to use. The model can call the others.

Consequences: new tools require a profile update and a snapshot update in the same change. There is no runtime switch that grants the implementer the reviewer tools.

Requirements: REQ-016, REQ-017.

Superseded in part by RFC-051 in [0006-company-github-and-linked-projects.md](0006-company-github-and-linked-projects.md). Packs add linked project context, and the implementer adds `read_linked_file`. Each role still has one fixed pack and one tool list.

Superseded in part by RFC-055 and RFC-056 in [0010-conventions-and-skills.md](0010-conventions-and-skills.md). The implementer and reviewer packs add the company coding rules for the project's stacks and the card's skills, inside the untrusted markers. The planner's `create_card` adds `skillIds`. The tool lists do not change.

## RFC-014 A model turn cannot supply the shell that runs

`localCheck` and `devCheck` are fields on the card, visible before the inner loop starts. The runner executes that stored command with the working directory pinned to the worktree. The executable must be on an allowlist. Text emitted during a model turn is not a command.

The repo and the model output are untrusted. A free shell is a deploy, a secret read, or a rewrite of the check.

Set aside: `bash -lc` on a string the model returns. Fast for prototypes, and incompatible with REQ-019.

Consequences: card create, or a person editing the card before it is `ready`, is the moment a check command changes. An unknown executable fails closed and writes a harness error. No partial run.

Requirements: REQ-005, REQ-018, REQ-019.

## RFC-015 Startup reconciles an in-flight deploy first

On startup the harness loads each unfinished card. If a deploy event exists and no health event follows it, the harness probes health or rolls back before any model call and before any second deploy. Committed model turns are not replayed.

A kill during deploy is the normal failure, not an edge. Replaying the deploy command can ship a second SHA or roll a healthy server back by accident.

Set aside: restarting the card from `in_progress` after any crash. That drops the deploy on the floor and invites a second one.

Consequences: deploy writes the "started" event before the command, including `previousSha`. Health or rollback writes the matching event after. Resume is covered by the fixture script in REQ-043.

Requirements: REQ-040, REQ-043.

## RFC-016 The v1 control surface is the CLI

Humans accept requirements, inspect the board, approve a SHA, request changes, reject, and unblock through the CLI in REQ-042. A later UI has to call those same transitions. It does not write status itself.

A UI before the transitions exist becomes a second board. The CLI is enough to run the fixture script and a real dev target.

Set aside: a graphical kanban in v1. It duplicates `status` and tempts a drag-and-drop writer. Also set aside: chat as the only control surface. Approval would be a sentence again.

Consequences: every human gate has a command and a non-zero exit on rejection. `status` on an empty board tells the operator to run `ingest`. Webhooks, when added, share the approval predicate with `approve`.

Requirements: REQ-042. Non-goal: NG-006.

Superseded by RFC-035 in [0003-review-corrections.md](0003-review-corrections.md).

## RFC-017 The router is a function of card facts and config

The harness assigns `plan`, `implement`, `implement_hard`, `review`, or `triage` from risk, failure count, and the current gate. There is no deploy class. Model ids live in config with a fallback. The decision stores model, fallback, tool profile, context pack, max turns, and reason. The model that produced the output is the one stored on the attempt.

Routing hidden inside an SDK client cannot be audited. Letting the model pick a larger model spends the budget without a record.

Set aside: one model for every class. Simple, and it throws away the family split in RFC-008. Also set aside: the implementer requesting `implement_hard` mid-turn. Failure count and risk already do that, outside the model.

Call rule: retry the primary once on provider error, call the fallback once, then block with both errors stored.

Consequences: the `router` section of `config.yaml` in the data directory is the route table. Changing it changes the next decision without a code edit. A fallback is its own event.

Requirements: REQ-010, REQ-011, REQ-012, REQ-014.

## RFC-018 A person accepts extracted ids before any worktree

Intake parses markdown under the configured PRD and RFC paths into records. Existing ids are kept. If a PRD has no ids, the planner may propose a draft. `run-next` creates no worktree until the operator accepts the id set. The acceptance stores the operator and the ids.

A planner will invent scope if implementation starts from a draft. The accept step is the scope gate.

Set aside: starting cards from the first model summary of the PRD. Faster, and the summary is not reviewable as a list of ids.

Consequences: `ingest` and `accept` are separate commands. Re-parse of unchanged files does not duplicate records. Cards reference requirement ids and decision ids from this RFC series and from the PRD.

Requirements: REQ-001, REQ-002, REQ-003.

Superseded in part by RFC-046 in [0005-teams-and-cloud.md](0005-teams-and-cloud.md). A Product Owner who is a member of the project accepts. The gate before any worktree stays.

Superseded in part by RFC-053 in [0008-manual-requirements.md](0008-manual-requirements.md). A member may also add a requirement by hand, and ingest keeps it. Any senior role who is a member accepts. The gate before any worktree stays.

## RFC-019 An edit to an accepted requirement stops affected cards

Changing a requirement statement, its acceptance list, or a decision statement clears acceptance for that id. Cards still in `backlog` or `ready` return to `backlog`. Cards past `ready` move to `blocked` with reason `source_changed`. They do not deploy and do not start an attempt until the operator accepts the new text and releases them. Unchanged ids stay accepted.

Otherwise a card implements the text from last Tuesday while the file says something else, and the pull request body still looks linked.

Set aside: continuing in-flight cards against a snapshot and ignoring the file. The link on the card would point at a statement the file no longer has.

Consequences: parse has to detect a statement change, not only an id change. Operators should expect a block when they edit an in-flight requirement. The event log keeps the old acceptance.

Requirements: REQ-004.

## RFC-020 Budgets end in blocked, and models cannot raise them

v1 defaults, all configurable, are 3 attempts per card, 12 model turns per attempt, 200000 tokens per card, and 30 minutes of wall time per attempt. These are caps, not measured usage. Hitting one moves the card to `blocked` with that reason. No model tool raises a cap. `unblock` returns the card to `ready`. The default release keeps the attempt counter.

An open retry loop will spend a night on one card and still fail. A person should see that stop.

Set aside: the agent raising its own cap after a polite explanation. The explanation is not a budget.

Consequences: each cap has a fixture that blocks. `trace` shows the reason. Changing a number in config applies to later attempts. It does not revive a card already blocked.

Requirements: REQ-014, REQ-015.

The release sentence is superseded by RFC-041 in [0003-review-corrections.md](0003-review-corrections.md).

## RFC-021 The event log is append-only and redacted

Events cover route, model call, tool call, gate, transition, rejected transition, deploy, health, verify, and budget stop. A correction is a new event. Model calls store model id, prompt hash, input tokens, output tokens, and stop reason. Configured secret patterns are stripped before append.

The log is how an operator answers who wrote the pull request, which SHA was approved, and which SHA was tested. A mutable log cannot support that after the fact.

Set aside: storing the raw provider payload. It is convenient for debugging and it will contain a key the moment a prompt quotes an env file.

Consequences: `orchestrator trace CARD` is the reader. Tests search a fixture log for a known raw token and expect no match. Resume uses events, so a missing "deploy started" event is a bug in the writer, not a case for the reader to guess.

Requirements: REQ-039, REQ-041.

## RFC-022 v1 edges are GitHub, markdown, SQLite, and a deploy command

The git host is GitHub. PRDs and RFCs are markdown under `docs/prd/` and `docs/rfc/` unless settings change the paths. The board is SQLite. Deploy is an executable command plus a health URL plus a rollback command. v1 does not include a cloud provider SDK.

The gates are the product. The edges should be replaceable without rewriting transitions.

Set aside: picking a cloud SDK, a ticket system, and a custom docs site before the fixture script passes. Each one freezes an integration the gates do not need.

Consequences: `config.yaml` in the data directory holds paths, deploy commands, checks, and approvers (RFC-036). Placeholders in examples are not live endpoints. Another git host is a new RFC because approval identity and webhook shape change. The transition table does not change with the host.

Requirements: REQ-001, REQ-033, REQ-042.

Superseded in part by RFC-044 in [0004-postgres.md](0004-postgres.md). The board engine in the statement is PostgreSQL. GitHub, markdown, and the deploy command stand.

## RFC-023 Accepted decision statements stay stable

The statement of an accepted decision is the first paragraph under its heading. Implementations and cards may cite the id. Changing the meaning requires a new RFC that names the superseded id. Wording fixes that do not change the decision can be made in place and called out at the top of the new RFC.

Cards store decision ids. If `RFC-006` quietly changes from "SHA" to "branch", old approvals lie.

Set aside: editing this file as a living wiki. Easy to keep current, and impossible to know what an in-flight card was bound to.

Consequences: superseding entries start at `RFC-024` in a new file or at the end of a follow-up RFC. The index in this file marks each superseded entry. RFC-024 to RFC-034 are in 0002, RFC-035 to RFC-043 are in 0003, and RFC-044 is in 0004.

## What stays configurable

These values are config under the decisions above. Changing them does not change the gate order.

| Config | v1 default | Bounded by |
| --- | --- | --- |
| Model ids and families | LLM and router settings in `config.yaml` | RFC-008, RFC-017 |
| Attempt, turn, token, and wall caps | 3, 12, 200000, 30 minutes | RFC-020 |
| PRD and RFC paths | `docs/prd`, `docs/rfc` | RFC-018, RFC-022 |
| Local and dev check commands | Deploy settings | RFC-014, RFC-026 |
| Deploy, health, and rollback commands | Deploy settings. Commands are absolute paths outside the repo | RFC-009, RFC-011, RFC-038 |
| Approver logins | GitHub settings | RFC-006 |
| Parallel implementation cap | 2, in deploy settings | RFC-010 |

Exact provider model ids, the health URL, and the approver logins are chosen per repo. This RFC does not assign them.

## How to apply this RFC

1. Cite decision ids from cards when a change touches a gate, a pack, or a credential.
2. A pull request that contradicts an accepted statement names a superseding RFC, or it does not merge.
3. Snapshot tests for packs and tool lists are the enforcement for RFC-007 and RFC-013.
4. The fixture script in REQ-043 is the enforcement for RFC-003, RFC-006, RFC-011, and RFC-015.
