# PRD: AI Coding Orchestrator

Status: v1 draft, revised after review. Date: 2026-10-10.

This document is the requirements source. [docs/README.md](../README.md) lists every document and which one wins when two disagree. Decisions are in `docs/rfc/`, RFC-0001 through RFC-0010. Implementation guides are in `docs/tech/`.

## Problem

A PRD and its RFCs describe the change. Getting that change onto a dev server still depends on a person copying scope into a coding session, opening a pull request, finding a reviewer, deploying the right commit, and reading the test log. When a test fails, the next session often starts from chat memory instead of from the card, the diff, and the log.

v1 is a harness that runs that path. Models propose edits and review comments. The board, the command exit code, the health check, and a person's approval decide what happens next.

## Users

| Person | What they do |
| --- | --- |
| Admin | A flag on a user, not a job role. Creates users, sets the job role and the admin flag, sets passwords, creates tribes, adds members, creates projects, and sets the company GitHub account, LLM APIs, router, coding rules, stacks, and integration skills. Does not approve or deploy unless the job role allows it. |
| Product Owner | Accepts requirements for a project whose tribe they belong to. May request changes, reject, approve, and unblock. Does not deploy. |
| Lead Developer | Accepts requirements. May start a run, request changes, reject, approve, unblock, and start deploy. Edits that project's repository, linked projects, checks, deploy settings, added stacks, and enabled skills. |
| QA Lead | Accepts requirements. May request changes, reject, and approve. Does not deploy. |
| Senior Developer | Accepts requirements. May start a run and request changes. Does not approve, reject, or deploy. |
| Developer | May start a run. Does not accept, approve, reject, request changes, or deploy. |
| QA | May start a run. Does not accept, approve, reject, request changes, or deploy. |

Every member of a project's tribe may add a requirement to that project by hand (REQ-055). A user has one of those six job roles. The role applies in every tribe they belong to. They sign in with email and password and keep their own GitHub host, login, and token, which name them as an approver and as the author of the cards they own (REQ-057). Clone and fetch use the company GitHub account (REQ-052). A user may belong to more than one tribe. They see a project only when they are a member of its tribe. The orchestrator bot is never an approver, and an owner never approves their own card, unless the project turns on Solo approval (REQ-070) or every requirement of the card has Self review on (REQ-073).

Models are workers the harness calls. They are not users, and they do not hold the board.

## v1 outcome

A member can give the orchestrator a markdown PRD and its RFCs and get this result:

1. Requirement and decision records exist, and no code is written until a member with a senior role (Lead Developer, Senior Developer, QA Lead, or Product Owner) accepts them.
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
| REQ-052 | The company has one GitHub account |
| REQ-053 | A project is one repository that the service clones |
| REQ-054 | A project can read context from linked projects |
| REQ-055 | A member can add a requirement by hand |
| REQ-056 | A card belongs to the person who brought its requirements in |
| REQ-057 | The owner commits and opens the pull request |
| REQ-058 | The requirement's creator picks pull request reviewers |
| REQ-059 | An admin keeps the company coding rules |
| REQ-060 | A project's stacks are detected, and a Lead Developer may add more |
| REQ-061 | A project enables integration skills, and the planner attaches them to cards |
| REQ-062 | Packs carry the rules and skills, and the trace names their versions |
| REQ-063 | A tribe admin runs one tribe |
| REQ-064 | A user has a display name |
| REQ-065 | Admins and tribe admins edit project settings |
| REQ-066 | GitHub confirms a project's repository |
| REQ-067 | Run one ready card |
| REQ-068 | Accepted requirements become cards |
| REQ-069 | A queued card runs with a real model up to a pull request |
| REQ-070 | Solo approval |
| REQ-071 | Approval shows on GitHub, and the card page shows the code changes |
| REQ-072 | The pull request checks decide when the AI review gives no verdict |
| REQ-073 | Self review on a requirement |
| REQ-074 | A failed attempt retries, and a requirement chooses what an AI request for changes does |
| REQ-075 | A person writes instructions that later attempts of a card read |
| REQ-076 | A person follows an attempt live on the Trace page |
| REQ-077 | An admin sets thinking and effort per model |
| REQ-078 | The implementer changes files without destroying them |
| REQ-079 | An implementer attempt keeps its history and finishes |
| REQ-080 | The implementer pack maps the repository and carries its agent notes |
| REQ-081 | Repeated prompt text is cached |
| REQ-082 | The implementer runs allowlisted commands on a copy, and infrastructure failures keep the attempt |
| REQ-083 | A member can refine a requirement with AI and compare it before applying |
| REQ-084 | An admin sees token usage across projects |
| REQ-085 | A tribe admin sees their tribe's token usage, and a member sees their own |
| REQ-086 | Token usage filters by source and card |
| REQ-087 | A tribe admin gives the implementer and refine read-only tools from the tribe's MCP servers |
| REQ-088 | A tribe admin adds MCP servers from a catalog, including read-only Postgres and SQL Server connectors |
| REQ-089 | A requirement names the MCP servers its cards may use |
| REQ-090 | A requirement names reviewers before they verify GitHub |
| REQ-091 | The sidebar groups the active tribe's pages and the active project's pages |
| REQ-092 | A tribe sets the conventions of its projects, and a project switches each on or off |
| REQ-093 | A person changes the skills, rules, and MCP servers of one card |
| REQ-094 | An operator installs runners that claim implement jobs, and an admin sees each runner's status |
| REQ-095 | People who must act on a card are notified, and open pages update without polling |
| REQ-096 | Feedback appears where the person acts |

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
- Acceptance stores the accepting user's id and the set of requirement ids accepted. A Developer or QA member is rejected (RFC-053).
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
| `agent_review` | `human_review` | Review body stored. Verdict `recommend_approve`, or the review gave no verdict (REQ-072). Required CI green, or none required. |
| `agent_review` | `blocked` | The company GitHub account is not set up, so the checks cannot be read. |
| `agent_review` | `pr_open` | The latest AI review is `failed`, and a person asked to retry it. Review cleared. Attempts unchanged (REQ-072). |
| `changes_requested` | `in_progress` | New attempt id. Same branch. Approval cleared. Attempts remain. |
| `changes_requested` | `blocked` | Attempt cap reached. Reason `attempt_cap`. |
| `human_review` | `changes_requested` | Approver requested changes. Approval cleared. |
| `human_review` | `approved` | Allowlisted approver. Approval names the current head SHA. |
| `human_review` | `blocked` | Approver rejected the card. |
| `approved` | `deploying_dev` | Deploy lock held. Approved SHA equals pull request head. Deploy settings complete. |
| `approved` | `done` | Project deploy is off. A person squash-merged the pull request at the approved SHA. Merge commit SHA stored (RFC-111). |
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
| `blocked` | `wont_fix` | A Lead Developer, Product Owner, or the card owner closed the card with a reason. An open pull request was commented on and closed with that person's GitHub token. Blocked reason kept (RFC-112). |
| `wont_fix` | `blocked` | The same people reopened the card. The kept blocked reason is restored (RFC-112). |
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
| Planner | PRD outline, requirement candidates, RFC decisions, existing card titles and dependencies, and the project's enabled skills by slug and "use when" (REQ-061) | File diffs, credentials, attempt logs |
| Implementer | The one card, the company rules and the card's skills (REQ-062), linked requirement statements, linked decisions, repo path map, local check command, the last failure note if any, and linked project context (REQ-054) | Other cards' diffs, deploy secrets |
| Reviewer | The company rules and the card's skills (REQ-062), linked requirements, linked decisions, the diff, local check output, pull request body, and linked project context (REQ-054) | Author messages, author tool trace |
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
| Implementer | List, search, and read files, replace text in a file, write, delete, and move a file, keep a todo list, run the card's local check, run an allowlisted command on a throwaway copy, commit inside the worktree | Push to the default branch, merge, deploy, review comment, transition |
| Reviewer | Read diff, comment on the pull request, verdict `recommend_approve` or `request_changes` | Edit, commit, merge, deploy, human approve |
| Triage | Read the log, write a failure note | Edit, commit, deploy |
| Deployer | Deploy the dev SHA, health check, rollback | Every model tool. The deployer is harness code. |

#### Acceptance

- Each profile's tool list matches the table in a snapshot test.
- An implementer turn that requests a deploy tool receives a tool error, and no deploy command runs.

### REQ-018 Check commands come from settings and run on a copy

`localCheck` and `devCheck` are copied from settings when the card is created and are visible on the card before the inner loop starts. No model tool can set or change them. The runner executes the command in the sandbox on a copy of the files, never on the worktree itself. The first word must be on the allowed-executables list, and shell syntax is refused. A shell string invented during a model turn is not executed. The implementer's `run_command` runs an argv, never a shell string, whose first element is on the project's allowed-commands list, in the same sandbox on a throwaway copy, and its exit code is never check evidence (REQ-082).

#### Acceptance

- The command that runs is the command stored on the card.
- A model turn that emits a different shell command does not execute that command as the check. A `run_command` call with a shell, a wrapper, or a name off the allowed-commands list runs nothing.
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
- With required CI green, and a reviewer verdict of `recommend_approve` or no verdict (REQ-072), the card can enter `human_review`.
- A repository with no required checks configured may enter `human_review` on local exit 0 plus the reviewer verdict, or no verdict (REQ-072), and the card records that CI was not required.

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
- Repeated parse failure follows REQ-012, and then the pull request checks decide the card (REQ-072).

### REQ-029 The reviewer model cannot approve, edit, or deploy

The reviewer verdict is a recommendation. It is not the approval gate in REQ-030.

#### Acceptance

- A `recommend_approve` verdict leaves `approvedSha` empty.
- No deploy event is written as a result of the reviewer verdict alone.
- The reviewer tool list matches REQ-017.

### REQ-030 Human approval is a permitted member and one SHA

A Lead Developer, QA Lead, or Product Owner who is a member of the project may approve the current pull request head. The approval stores that user's GitHub login and the SHA. The user must have a GitHub login saved, and that login must not be the project bot. The orchestrator bot identity is rejected even if the git host would accept its approval. Solo approval (REQ-070) and Self review (REQ-073) are the exceptions.

v1 accepts the approval from the CLI and from the web UI. Both call the same service. A git host webhook may deliver the same facts later, through the same checks. Approval does not start deploy (REQ-050).

#### Acceptance

- Approval by a permitted member, for the current head SHA, moves the card to `approved` and stores that SHA. It does not start deploy.
- Approval by a Developer, Senior Developer, or QA is rejected and the card stays in `human_review`.
- Approval by the bot login, or by a member with no GitHub login, is rejected and the card stays in `human_review`. With Solo approval on (REQ-070), or on a card whose requirements all have Self review (REQ-073), the bot login may approve.
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

A Lead Developer member, a tribe admin of the project's tribe, or a company admin edits project settings from the web UI, through the JSON API (REQ-065). An admin edits the company GitHub account, LLM APIs, and router, which every project uses (RFC-052). Each user edits their own GitHub settings. There is no approver list and no operator login.

| Section | Fields | Who saves |
| --- | --- | --- |
| Account GitHub | Host, login, token | That user |
| Company GitHub | Host, organization, token. The bot login comes from the test | Admin |
| Project repository | Repo in the company organization, base branch | Lead Developer member, tribe admin, or admin |
| Linked projects | Projects in the same tribe, shared paths per link | Lead Developer member, tribe admin, or admin |
| Tribe conventions | Stacks and skills for every project of the tribe, company rules switched off (REQ-092) | Tribe admin or admin |
| Project conventions | Each stack, skill, and rule switched on or off over the tribe (REQ-092) | Lead Developer member, tribe admin, or admin |
| LLM | Families, each with protocol, base URL, model ids, and an API key. One list for the company | Admin |
| Router | Model and fallback per task class, review independence, budgets. One router for the company | Admin |
| Deploy | Dev URL, dev hosts, deploy and rollback commands, health URL, deploy token, check commands, allowed executables, sandbox engine and image, implementation cap, doc paths | Lead Developer member, tribe admin, or admin |

No response contains a secret value. A response says whether each secret is set. Saving with an empty secret field keeps the stored value. Clearing a secret is a separate explicit action. A saved change applies to the next job of that project and never to an attempt in flight. The company bot login cannot approve unless the project turns on Solo approval (REQ-070) or the card's requirements all have Self review (REQ-073). A settings change writes an event that names the section and the fields, never the values.

#### Acceptance

- `GET` on each settings section returns `tokenSet` or `keySet` flags and no secret.
- Saving the account GitHub section with an empty token keeps the stored token.
- A Developer member who saves project deploy settings is rejected.
- A Lead Developer who is not an admin and saves LLM or router settings is rejected.
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

- A Product Owner, Lead Developer, Senior Developer, or QA Lead member can accept requirements. A Developer or QA member cannot.
- A Lead Developer member can start deploy from `approved`. A Product Owner member who approved the same SHA cannot.
- A Developer member can start a run and cannot approve, reject, request changes, or deploy.
- A QA Lead member can approve. A QA member cannot.
- A Senior Developer member can request changes and cannot approve.
- An action omitted from `actions` is rejected when called directly.

### REQ-048 A tribe has members

An admin creates a tribe and adds users to it. A tribe admin may also add and remove that tribe's members (REQ-063). A user may be in more than one tribe. Removing a member hides that tribe's projects from them. The membership row is the only visibility grant.

#### Acceptance

- An admin can create a tribe and add a user.
- A non-admin cannot create a tribe. A user who is neither an admin nor a tribe admin of that tribe cannot change its members.
- A user in two tribes is a member of both.

### REQ-049 A project is visible to its tribe

An admin, or a tribe admin of that tribe, creates a project on one tribe and may choose its linked projects in the same request (REQ-054, REQ-063). The project names one repository (REQ-053). Members of that tribe can list and open it. Anyone else receives `404`, including for an id that does not exist. Cards, intake, runs, and project settings for that project are included in that rule, except that a company admin may open any project's settings (REQ-065).

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

Each user stores a GitHub host, login, and token. The token is write-only. The login names the user as an approver. A user can test their own token. The test asks GitHub which login the token belongs to and stores that login as verified. The worker pushes and opens pull requests for a card with its owner's token (REQ-057, RFC-054). The API returns whether a token is set and never returns the token.

#### Acceptance

- `GET` of the caller's GitHub settings returns `tokenSet` and no token.
- One user cannot read or replace another user's token.
- A job's push uses the card owner's token, and its clone uses the company token.
- Approval is rejected when the caller's GitHub login is empty.
- Testing a token stores the login GitHub reports, not the typed one.

### REQ-052 The company has one GitHub account

An admin sets one GitHub account for the company: a host, an organization, and a token. The token is ciphertext in the database and write-only. The test calls GitHub with the token, checks the organization, and stores the login it reports as the bot login. Every job in every project clones, fetches, pushes, and opens pull requests with this token. The token is passed to `git` and `gh` in the child environment, never in argv. A Lead Developer can list the organization's repositories to pick one for a project.

#### Acceptance

- `GET /api/settings/github` returns `tokenSet` and no token.
- A non-admin who saves the company GitHub account is rejected.
- Saving with an empty token keeps the stored token.
- Approval is rejected when the caller's GitHub login equals the company bot login, unless the project has Solo approval on (REQ-070) or the card's requirements all have Self review (REQ-073).

### REQ-053 A project is one repository that the service clones

A project names one repository in the company organization and one base branch. Two projects cannot name the same repository. The service clones the repository into that project's data directory and fetches the base branch before ingest and on Sync. Worktrees branch from the project's base branch. Deploy, checks, sandbox, scheduler, and doc paths are settings of that project, and its deploy token is in that project's secrets file.

#### Acceptance

- Creating a second project on the same repository is rejected.
- The clone command has no token in argv.
- A worktree branches from the project's base branch.
- Ingest on one project does not add requirements to another project.
- Deploy settings saved on one project are not returned for another project.
- A Developer member who saves project deploy settings is rejected.
- A Lead Developer who is not an admin and saves LLM or router settings is rejected.

### REQ-054 A project can read context from linked projects

A Lead Developer member, or a tribe admin of the tribe, links a project to other projects in the same tribe. A link is one way. Each link may name shared paths, which are relative paths in the linked repository. The implementer and reviewer packs add each linked project's accepted requirements and decisions, and the shared files read at the linked base branch, inside the untrusted markers. The implementer may call `read_linked_file` to read any other file of a linked project at that SHA. It cannot write there.

#### Acceptance

- A link to a project in another tribe is rejected, also when it is sent with a new project, and then no project is stored.
- A link from a project to itself is rejected.
- A shared path that escapes the repository is rejected.
- The implementer pack contains the linked project's accepted requirement statements, marked untrusted.
- `read_linked_file` rejects `..`, an absolute path, and `.git`.

### REQ-055 A member can add a requirement by hand

A member of the project's tribe adds a requirement on the Intake page or through the API: an optional id, a statement, and acceptance lines. A blank id gets the next free `REQ-` number in that project. The requirement is not accepted until a senior member accepts it. A member may edit or delete a requirement that was added by hand. A requirement parsed from the repository is edited in the repository. Ingest never overwrites a requirement that was added by hand (RFC-053).

#### Acceptance

- Adding with a blank id after `REQ-001` stores `REQ-002`.
- Adding an id that already exists in that project is rejected. The same id on another project is allowed.
- A new requirement is not accepted.
- Editing the statement of an accepted manual requirement clears acceptance.
- Editing or deleting a requirement parsed from the repository is rejected.
- Deleting a manual requirement that a card lists is rejected.
- Ingest of a PRD that defines an id already added by hand keeps the manual text and reports the id as skipped.
- A user outside the tribe receives `404`.

### REQ-056 A card belongs to the person who brought its requirements in

Each requirement records the member who added it by hand or ran the ingest that first stored it. A card's owner is that member, read from the card's requirements when the planner creates it. The planner cannot choose the owner. A card's requirements all come from one owner. The board and the card page show the owner (RFC-054).

#### Acceptance

- Ingest from the web stores the caller as the creator of each new parsed requirement. A re-ingest keeps the first creator.
- A card created from a requirement stores that requirement's creator as `owner_id`.
- `create_card` with requirements from two creators is rejected.
- An owner passed in the `create_card` arguments is ignored.

### REQ-057 The owner commits and opens the pull request

A card's commits use the owner's verified GitHub login as the author and committer name and the owner's GitHub noreply address as the email. The worker pushes the card branch and opens or updates the pull request with the owner's token, in the child environment only. Clone and fetch use the company token. When the owner is missing, disabled, unverified, or has no token, the card is blocked with `owner_github` and nothing is pushed. The owner cannot approve their own card (RFC-054), unless the project has Solo approval on (REQ-070) or the card's requirements all have Self review (REQ-073).

#### Acceptance

- A commit on an owned card has the owner's login and `<id>+<login>@users.noreply.github.com` as author and committer.
- The push and the pull request calls carry the owner's token in the environment and never in argv.
- A card whose owner has no verified token moves to `blocked` with `owner_github`, and no push runs.
- With Solo approval off, approval by the owner is rejected, and the owner's `actions` list has no `approve`.

### REQ-058 The requirement's creator picks pull request reviewers

The member who brought a requirement in, or a Lead Developer member, names reviewers for it on the Intake page. Reviewers are tribe members; one without a verified GitHub login is asked on GitHub after they verify (REQ-090). When a card's pull request is open, the worker requests the union of its requirements' reviewers on GitHub, without the owner and the company bot. A failed request writes an event and does not change the card (RFC-054). The board and the card page show that union as the card's reviewers.

#### Acceptance

- The creator can set reviewers on a parsed or a manual requirement. Another Developer member is rejected.
- A reviewer who is not a tribe member is rejected (REQ-090).
- The reviewer request on the pull request excludes the owner and the bot.
- A failed reviewer request leaves the card status unchanged.
- The card JSON lists the reviewers of its requirements, without the owner, and only from the card's project.

### REQ-059 An admin keeps the company coding rules

The company has one catalog of coding rules. A rule is global (naming, database tables and columns, style) or belongs to one stack (a language or framework). Each stack lists the marker files and dependencies that identify it. Only an admin creates, edits, or disables rules and stacks. Every signed-in user may read them. Each save increments the version, stores the content hash, and keeps the old text as a revision (RFC-055).

#### Acceptance

- A user who is not an admin cannot create, edit, or disable a rule, a stack, or a skill.
- Editing a rule increments its version and appends a revision with the previous hash still readable.
- A rule body longer than 8000 characters is rejected with a field error.
- A catalog save appends `config_changed` with section `conventions` and field names only.

### REQ-060 A project's stacks are detected, and a Lead Developer may add more

Each Sync detects the project's stacks from the clone's marker files and dependencies, two directory levels deep, without following links. A Lead Developer member, a tribe admin, or an admin may add stacks by hand (REQ-065). A Sync replaces the detected stacks and keeps the added ones. The rules for a card are the global rules plus the rules of the project's stacks (RFC-055), with the tribe, project, and card choices of REQ-092 and REQ-093 applied.

#### Acceptance

- A repository with `go.mod` and a `package.json` that depends on `@sveltejs/kit` is detected as Go, TypeScript, and SvelteKit.
- A Sync keeps a stack a Lead Developer added by hand.
- Saving a project's stacks as a Developer is rejected.

### REQ-061 A project enables integration skills, and the planner attaches them to cards

An admin writes a skill: a slug, when to use it, and a body with steps, configuration, security rules, and a test checklist. A Lead Developer member, a tribe admin, or an admin enables skills for a project (REQ-065). The planner sees the enabled skills and may attach them to a card with `skillIds`. A skill that is not enabled for the project fails the tool call (RFC-056). A tribe may enable skills for all its projects, and a person may change one card's skills (REQ-092, REQ-093).

#### Acceptance

- `create_card` with an enabled skill stores it on the card, and the card JSON lists it.
- `create_card` with a skill the project has not enabled stores no card.

### REQ-062 Packs carry the rules and skills, and the trace names their versions

The implementer and reviewer packs carry the card's rules and skills inside the untrusted markers, after the card block. Each item is capped at 16 KiB and the section at 64 KiB, and an item that does not fit is replaced by a note. The text is redacted. An attempt reads the rules once and appends a `conventions_context` event with each id, version, and hash. Rule and skill text cannot add a tool or a command (RFC-055, RFC-056).

#### Acceptance

- The implementer pack places rule and skill blocks between the markers.
- A section over the cap ends with an `omitted` note, and the pack stays under the pack cap.
- A rule that asks for a deploy tool leaves the implementer tool list unchanged.
- An attempt's trace lists the rules and skills it used, with versions and hashes.

### REQ-063 A tribe admin runs one tribe

A company admin, or a tribe admin, may make a member of that tribe a tribe admin. A tribe admin adds and removes the tribe's members, grants or clears the tribe admin flag on other members, creates projects on the tribe with their links, and saves the settings of any project in the tribe (REQ-065). A tribe admin cannot create tribes or users, edit company settings, or clear their own flag (RFC-057).

#### Acceptance

- A tribe admin adds a user to their tribe and removes one. A plain member is rejected.
- A tribe admin cannot change the members of another tribe.
- A tribe admin makes another member a tribe admin, and cannot clear their own flag.
- A tribe admin creates a project on their tribe with a link to another project in it, and is rejected on another tribe.
- A tribe admin who is not a Lead Developer saves a project's links.

### REQ-064 A user has a display name

An admin gives each new user a display name, and may change it later. Each user may change their own name. Pages that list a person show the name first, and the email second where there is room. A user without a name shows their verified GitHub login, then their email. The email stays the sign-in id (RFC-058).

#### Acceptance

- Creating a user without a name, or with only spaces, is rejected. With a name, the user list shows it.
- An admin renames a user. An empty name is rejected.
- A user changes their own name on the Account page, and the user menu shows it. Another user's name is unchanged.
- The board and the card page show the owner's and reviewers' names. A user without a name shows the login, then the email.

### REQ-065 Admins and tribe admins edit project settings

A company admin opens and saves the repository, linked projects, deploy, and conventions settings of every project, including a project in a tribe they are not a member of. A tribe admin does the same for every project in their tribe. A Lead Developer member keeps the same rights. Each may run Sync. Other members read the settings and cannot save them, and the pages lock their fields. Opening a project's settings does not open its board (RFC-059).

#### Acceptance

- A company admin with the Product Owner role who is not in the tribe reads and saves all four sections, each with `canEdit: true`, and runs Sync.
- The same admin receives `404` for the project's cards.
- A tribe admin with the Developer role saves the repository, deploy, and conventions of a project in their tribe, and receives `404` for a project in another tribe.
- A Developer member reads each section with `canEdit: false`, and each save is rejected.

### REQ-066 GitHub confirms a project's repository

Creating a project, or saving a new repository or base branch for one, asks GitHub with the company token whether the repository exists in the company organization, is not archived, and has that branch. Otherwise nothing is saved, and the field says why. With no company token saved, the save is rejected and asks an admin to save one. The pages pick the repository from the organization's list, with no free-text field (RFC-060).

#### Acceptance

- Creating a project on a repository GitHub does not find, an archived repository, or a repository of another organization is rejected on `repo`, and stores no project.
- A base branch GitHub does not find is rejected on `baseBranch`.
- With no company token, creating a project is rejected and names the company GitHub token.
- When GitHub fails, the response is `502` and nothing is saved.
- Saving the repository settings with a missing repository is rejected. A repository GitHub finds is saved.

### REQ-067 Run one ready card

A Lead Developer, Senior Developer, Developer, or QA member can queue one ready card whose dependencies are done, from its board tile or its card page. The board's top button queues every such card and shows how many there are. A queued card shows Queued until the worker picks it up, and a ready card whose dependencies are not done shows how many it waits on. Queueing does not change the card's status (RFC-061).

#### Acceptance

- A ready card with done dependencies lists `run` in `actions`. Queueing it twice leaves one open run request, and the card then shows `queued: true` without `run`.
- A ready card with a dependency that is not done lists it in `waitingOn`, has no `run` action, and a request to run it is `409`.
- A card that is not ready is `409`, except an `in_progress` or `changes_requested` card with no open attempt (RFC-063, REQ-069).
- A Product Owner sees no `run` action and is refused with `403`.
- The top button queues only the ready cards with done dependencies, a repeat queues none, and the board's `readyCount` then reads `0`.

### REQ-068 Accepted requirements become cards

After a requirement is accepted, the project's worker makes one card for it, owned by the person who brought it in, without a model call. A card whose requirements are accepted, whose dependencies are done, and that has acceptance lines or is manual moves to Ready. The board refreshes until the worker is idle. A project whose worker lock another process holds is skipped (RFC-062).

#### Acceptance

- Two accepted requirements and one unaccepted requirement make two backlog cards, titled with the headings and owned by each requirement's creator. A second pass makes none.
- A card with accepted requirements and acceptance lines moves to `ready`. A card without acceptance lines that is not manual, and a card with an unfinished dependency, stay in `backlog`.
- An idle pass marks the project's run requests processed, and leaves card-scoped requests open.
- With `serve` running, an accept row leads to a ready card, then a processed row.
- `run-next` on a project with one accepted requirement prints `created 1`.

### REQ-069 A queued card runs with a real model up to a pull request

An admin's Test connection on an LLM family sends one short request to its first model and shows the provider's answer or its error, without the key. A family uses the `openai` protocol (OpenAI, Groq, or any compatible server) or the `anthropic` protocol (Anthropic, Deepseek, or any compatible server). When a member presses Run, the project's worker starts the implement job for that card under the project's cap: the routed model edits and commits in the card's worktree, the project's local check runs on the commit in the pinned sandbox image, and an exit `0` pushes the branch and opens the pull request with the card owner's token. A failed check sends the card back to In progress with a failure note until attempts run out. Missing settings block the card with the reason to fix. A ready card nobody queued does not start (RFC-063).

#### Acceptance

- The OpenAI client sends the tools with their schemas and the bearer key to `<baseUrl>/chat/completions`, and maps tool calls and token counts. Its errors do not contain the key.
- The Anthropic client sends the tools as `input_schema` to `<baseUrl>/v1/messages` with `x-api-key`, and maps text, tool calls, and token counts.
- A model is served by the client of the family that lists it. A remote family without a key is refused, and a loopback one is allowed.
- A primary model that fails twice is replaced by the route's fallback. With no fallback, the error is `provider_error` without the key.
- Test connection returns `ok: true` after one request, and `ok: false` with a status and without the key for a bad key. An unknown family is `404`.
- A non-fixture check runs as `<engine> run --rm --pull=never --network=none` with the export at `/work`. Docker gets no `relabel`. A floating image, a relative engine, or a shell engine runs nothing.
- A queued card whose check exits `0` reaches `pr_open` with the pull request URL, a `local_checks` gate, outcome `checks_passed`, and one attempt used.
- A check that exits non-zero leaves the card `in_progress` with a `local_check` note and nothing pushed, and the card can be run again. With no attempts left it is blocked with `attempt_cap`.
- A check that is not allowlisted, or an unpinned image on a real repo, blocks with `sandbox_config` before any network call. A card made without a check gets the project's check.
- A route with no model blocks with `router_config` and makes no attempt. A card with an open attempt does not start.
- With a cap of 2 and three queued cards, two start, the third waits, then starts oldest first. A ready card nobody queued does not start.
- An `in_progress` card with no open attempt lists `run`. One with an open attempt, and a `backlog`, `pr_open`, or `blocked` card, are `409`.

### REQ-070 Solo approval

A team of one has no second person to approve, and its one GitHub account is often also the company bot. A project's Deploy settings have a Solo approval switch, off by default. While it is on, the card owner may approve their own card, and a member whose GitHub login equals the company bot login may approve, request changes, and reject. Every other approval rule stays: the job role, tribe membership, a saved GitHub login, and the current head SHA. An approval by the owner or by the bot login is stored as not independent (RFC-067).

#### Acceptance

- With Solo approval off, the owner's approval is `403` and the owner's `actions` list has no `approve`.
- With Solo approval on, the owner's `actions` list has `approve`, and the owner's approval with the bot login reaches `approved`, stored with `independent = false`.
- Saving the switch writes a `config_changed` event that names `soloApproval`.

### REQ-071 Approval shows on GitHub, and the card page shows the code changes

A new approval posts a comment on the card's pull request and a `success` commit status with context `orchestrator/approval` on the approved SHA, with the company token. Request changes or Reject on an approved card sets that status to `failure`. A failed post writes a `job_error` and leaves the card `approved`. The review job does not count `orchestrator/approval` as a required check. The card page has a Changes panel with each changed file, its added and deleted line counts, the colored diff, and a link to the GitHub compare view. Only members of the project's tribe can read the diff, and secrets in it are redacted (RFC-068).

#### Acceptance

- Approving posts one comment and one `success` status on the SHA. Approving the same SHA again posts nothing.
- When GitHub refuses the post, the card is still `approved` and a `job_error` names the failed post.
- Request changes on an approved card posts a `failure` status on the approved SHA.
- A pending `orchestrator/approval` status does not keep a card out of `human_review`.
- A member reads the card's diff with per-file counts and hunks. A non-member gets `404`. A planted model key shows as `[redacted]`.

### REQ-072 The pull request checks decide when the AI review gives no verdict

A card whose pull request checks are green, or which has no checks, reaches `human_review` even when the AI review gives no verdict. That covers no routable review model, an unparseable verdict after the fallback model, a provider failure, and a review job that stopped. The attempt stores the review as `failed`, the pull request gets an "AI review: Not done" comment, and the card page shows that the AI review failed. A failed check still moves the card to `changes_requested`, and a `request_changes` verdict still sends it back. An empty check list counts as no checks only 2 minutes after the card entered `pr_open`, because GitHub needs time to register the runs (RFC-069). While the card waits in `agent_review` with a `failed` review, a person who may run cards may retry the AI review, which sends the card back to `pr_open` for a new review job without using an attempt (RFC-076).

#### Acceptance

- A review with no verdict and no checks reaches `human_review` with `ci_required = false` and posts one "AI review: Not done" comment.
- A review with no verdict and a red check moves the card to `changes_requested` with a `ci` note.
- Right after `pr_open`, an empty check list keeps the card in `agent_review`. After 2 minutes it reaches `human_review`.
- A card left in `agent_review` without a verdict by a stopped review reaches `human_review` on a refresh once no job holds its lock.
- Retry AI review on an `agent_review` card with a `failed` review moves it to `pr_open`, and the next review job records a new verdict. `attempts_used` does not change.

### REQ-073 Self review on a requirement

A Lead Developer or an admin may turn Self review on for one requirement, when adding it on the New requirement page or later on its Reviewers panel. Nobody else may change it. A card whose requirements all have Self review on is approved as if its project had Solo approval on (REQ-070): the owner may approve it, and so may the bot login. Every other approval rule stays, and such an approval is stored as not independent. A card that also lists a requirement without Self review keeps the owner rule (RFC-070).

#### Acceptance

- A Developer who adds a requirement with Self review on gets `403`, and so does the requirement's creator, as a Developer, who tries to turn it on later. A Lead Developer's or an admin's change is stored.
- With Solo approval off, the owner of a card whose only requirement has Self review has `approve` in `actions`, and the owner's approval reaches `approved` with `independent = false`.
- When the card also lists a requirement without Self review, the owner's `actions` has no `approve` and the owner's approval is `403`.

### REQ-074 A failed attempt retries, and a requirement chooses what an AI request for changes does

After a person runs a card, an attempt that makes no commit or fails its local check queues the next attempt while attempts remain, so nobody presses Run again. A card out of attempts is blocked with `attempt_cap` and is not offered Run (RFC-071). A red pull request check also queues the next attempt, except on a Manual card, which waits for Run, and any card sent back to `changes_requested` with no attempts left is blocked with `attempt_cap` (RFC-077).

A Lead Developer or an admin sets each requirement's AI review mode: Continue, the default, or Manual. Nobody else may choose Manual. When the AI review asks for changes on a card whose requirements are all Continue, the card goes back with the review note and the next attempt is queued. When any requirement is Manual, the card waits for its pull request checks and reaches Human review with the review note shown, so an approver may approve the head as it is or ask for changes (RFC-072).

#### Acceptance

- An attempt with no commit and attempts left leaves the card queued. At the cap the card is blocked with `attempt_cap` and Run is not listed.
- An AI request for changes on a Continue card reaches `changes_requested` with the next attempt queued.
- An AI request for changes on a Manual card with green checks reaches `human_review`, and the card's notes hold the review body.
- A Developer who asks for Manual gets `403`. A Lead Developer's change shows on the requirement and on its card.
- A red check with attempts left reaches `changes_requested` with the next attempt queued. Out of attempts, the card is blocked with `attempt_cap`.
- Request changes on a card out of attempts blocks it with `attempt_cap`, and a `changes_requested` card out of attempts is blocked on the worker's next refresh.

### REQ-075 A person writes instructions that later attempts of a card read

Before the next attempt of a card starts, a person who may run it can write instructions: in the box that Run opens on the card page, or in the card's Instructions panel, also while the card is queued. Every later attempt reads the newest instructions next to the newest failure note, such as the AI review's, until a person writes new ones or clears them (RFC-073).

#### Acceptance

- After an AI request for changes and saved instructions, the next attempt's prompt has the review note and the instructions.
- Instructions over 4000 characters are refused, and Run with them queues nothing.
- A role that cannot run the card gets `403`, and the card page shows no Instructions panel to it.

### REQ-076 A person follows an attempt live on the Trace page

While a card works, its Trace page shows new events without a reload. Each model call also records the model's reply text, and each tool call records the arguments a person needs to follow it, such as the file path, the search text, the commit message, or the check command. Both are redacted and capped. This amends REQ-041: a model call event stores the five usage fields and the reply text (RFC-074). A Terminal view on the same page prints every stored event with its full text, oldest first, like terminal output (RFC-078). While the card works, the page holds a streamed connection: stored events arrive as they commit, and the model's reply appears as it is written, before its event is stored (RFC-088).

#### Acceptance

- During an attempt, a new tool call appears on an open Trace page within a few seconds, and the page stops refreshing once the card stops working.
- A read file row names the file, and an edit file row stores its size and not the file text.
- A configured secret in the model's reply is stored as `[redacted]`.
- In the Terminal view, a tool call shows its whole stored output under its command, and new output appears within about a second while the card works.
- In the Terminal view, the model's reply text appears while the model is still writing it, and the stored model call then replaces it.
- A configured secret never appears in the streamed reply text, even when it arrives split across pieces.

### REQ-077 An admin sets thinking and effort per model

On an LLM family's page, an admin sets each model's thinking (provider default, on, off, or adaptive on the anthropic protocol), the thinking budget when it is on, and the effort. Every request to that model sends them, a fallback sends its own model's settings, and a model with none set sends the same request as before. The model's reasoning goes back to it between tool turns of an attempt and is not stored (RFC-079).

#### Acceptance

- A model with no settings sends no `thinking`, `output_config`, or `reasoning_effort`.
- Thinking on with a budget sends that budget, and the reply keeps its own token limit.
- `adaptive` on an openai family, a budget outside 1,024 to 32,768, or an effort the protocol does not take is `422` on `modelOptions`.
- With thinking on, a tool turn after a reply that thought succeeds on the same model, and the trace holds the reply text only.

### REQ-078 The implementer changes files without destroying them

The implementer reads a file by line range, replaces exact text in it, writes, deletes, and moves files, and searches by text or pattern. A write that would empty a file or copy a history placeholder into it is refused. A tool error is marked as an error, and every tool result is redacted before the model sees it (RFC-080).

#### Acceptance

- `replace_in_file` with text that occurs twice, or not at all, changes nothing and returns an error.
- `edit_file` with the history placeholder, or with empty text over a non-empty file, changes nothing.
- A read with an offset and a limit returns those lines numbered, with the total line count.
- A configured secret in a file the model reads reaches the model as `[redacted]`.

### REQ-079 An implementer attempt keeps its history and finishes

Each model sets how much earlier tool output an attempt sends back. Output leaves the history oldest first and does not come back. A reply without a tool call before any commit is nudged to continue at most twice. When the last local check passed on the same files, the harness commits them. The default is 40 model turns per attempt, and the turn and token caps block the card (RFC-081).

#### Acceptance

- An attempt whose last local check exited 0 and that ends without calling `commit` ends with a commit on that tree.
- An attempt that changes a file after its last passing check is not committed by the harness.
- A fixture that uses every turn without a commit blocks with `turn_cap`, and one over the card's token cap blocks with `token_cap`, each with a `budget_stop` event.

### REQ-080 The implementer pack maps the repository and carries its agent notes

The implementer pack holds a path map of the repository, the repository's `AGENTS.md` and `CLAUDE.md` as data inside the markers, each requirement's heading and acceptance lines, and the decisions that name the card's requirements. The implementer keeps a todo list that every request carries. Read-only tool calls in one reply run at the same time (RFC-082).

#### Acceptance

- The pack fixture contains the path map and `AGENTS.md` between the markers, and the trace records the file's hash.
- An `AGENTS.md` over 16 KiB is cut.
- A todo list written on one turn appears in every later request of the attempt.

### REQ-081 Repeated prompt text is cached

Requests on the anthropic protocol mark the system prompt, the tools, and the earlier turns as cacheable. Cached input tokens are recorded per attempt and count at a tenth toward the card's token cap (RFC-083).

#### Acceptance

- An anthropic request body has three cache breakpoints.
- Cached input tokens reported by either protocol are stored on the attempt.

### REQ-082 The implementer runs allowlisted commands on a copy, and infrastructure failures keep the attempt

A project may list commands the implementer may run. `run_command` runs one argv from that list in the sandbox on a throwaway copy of the worktree, with no network, and returns its exit code and output tail. A stopped sandbox engine, a provider outage, or a failed publish blocks the card without using up an attempt. A branch that already holds the change goes to the gating check (RFC-084, RFC-085).

#### Acceptance

- With no allowed commands, `run_command` runs nothing.
- A file a command writes is not in the worktree afterwards.
- A gating check whose engine exits 125 blocks with `sandbox_config` and leaves `attemptsUsed`.
- An attempt that makes no commit on a branch already ahead of the base branch goes to the gating check.

### REQ-083 A member can refine a requirement with AI and compare it before applying

On the new-requirement form and the edit page of a manual requirement, a member can ask the model of the `implement` route to refine the statement or the acceptance lines, each from its own button, optionally saying what to improve. The reply appears as the model writes it. The page then shows it against the current text of that field, and the member applies it to the form or discards it (RFC-086, RFC-087).

#### Acceptance

- A refinement never changes a saved requirement on its own.
- Refining the statement leaves the acceptance lines alone, and refining the acceptance lines leaves the statement alone.
- Text appears on the page before the whole reply is finished, and Stop ends the request.
- The page shows that field's changes before Apply.
- Apply puts the proposal in that field of the form, and nothing is saved until the member presses Save.
- Saving a refined requirement clears its acceptance.
- With no model on the `implement` route, the page says so and nothing is sent.

### REQ-084 An admin sees token usage across projects

An admin opens one Token usage page for the whole company. It counts the tokens of every stored model call, from the implementer, the reviewer, and requirement refine, over a chosen number of days. The admin filters and groups the usage by LLM, model, project, user, and task type, and sorts the groups by any column. A call counts against the card's owner, and a refine against the member who ran it (RFC-089).

#### Acceptance

- Only an admin sees the page and its data. Every other user gets no usage.
- Implementer, reviewer, and refine calls are all counted, each under its own task type.
- Filtering by LLM, model, project, user, or task type changes every total on the page.
- The groups sort by name, calls, input, output, cached, or total, in either direction.
- The page never shows a model's reply text or a secret.

### REQ-085 A tribe admin sees their tribe's token usage, and a member sees their own

A tribe admin opens the Token usage page for their tribe and sees the calls of that tribe's projects only. Every member opens My token usage and sees the calls of the cards they own and the refines they ran. Both pages filter, group, and sort as the admin page does (RFC-090).

#### Acceptance

- A tribe's page opens for a company admin and for that tribe's admins. Other users get no usage for the tribe.
- A tribe's page counts only calls in that tribe's projects.
- My token usage counts only the member's own calls, and never shows another user.
- A call on a card counts in the card's project.
- Neither page shows a model's reply text or a secret.

### REQ-086 Token usage filters by source and card

Every Token usage page filters and groups the usage by source, Intake for requirement refines and Card for the implement and review jobs on cards, and by card, so a person sees what intake costs next to card work and which card used the most (RFC-091).

#### Acceptance

- Source has two values: Intake and Card. Filtering by one changes every total on the page.
- Card lists each card that made calls, by title, and refines as "Intake". Two cards with one title stay apart.
- Source and Card are on the company, tribe, and My token usage pages.

### REQ-087 Tribe MCP servers

A tribe admin registers the tribe's Model Context Protocol servers, over HTTPS or as a stdio server in the sandbox image, and allowlists their read-only tools. The implementer and requirement refine of the tribe's projects can call those tools, and their results are data (RFC-092 to RFC-095).

#### Acceptance

- A company admin and the tribe's admins manage the tribe's servers. Other members, and admins of another tribe, cannot see them.
- Header and environment secrets are never returned by the API and are redacted from every event.
- Only allowlisted tools whose schema has not changed are offered, as `mcp_<server>_<tool>`.
- An HTTP server on a private, loopback, or metadata address is refused. A stdio server has no network.
- An MCP result cannot move a card, pass a check, or add a tool.
- A tribe with no server sends the same implementer and refine requests as before.

### REQ-088 MCP catalog and database connectors

A tribe admin picks an MCP server from a catalog, a featured remote server, the built-in Postgres and SQL Server connectors, or a search of the official MCP Registry, fills in its credentials, and saves it. The implementer and refine read the tribe's databases through read-only tools (RFC-096 to RFC-100).

#### Acceptance

- The catalog lists featured servers and Postgres and SQL Server at once, then registry servers when the registry answers. A server the sandbox cannot run is listed with the reason and cannot be set up.
- When the registry is slow or cannot be reached, the featured and built-in entries still show, and typing a new query stops the previous search.
- A database connector reaches any host, private and loopback included, except the board database.
- A database connector runs one read-only statement per call and returns at most 200 rows. The password is never returned and is redacted from every event.
- A stdio registry server is pulled and pinned by digest before it is saved.

### REQ-089 MCP mentions in requirements

A member writing a requirement names the tribe's MCP servers it needs with `@slug`, picked from a list of the tribe's servers. The implementer of its cards, and refine of its text, offer only those servers. A requirement that names none keeps every enabled server (RFC-101, RFC-102).

#### Acceptance

- Typing `@` in the statement or an acceptance line lists the tribe's server slugs. Each mention shows under the field, with a warning for an unknown or disabled server.
- An email, an npm scope, a decorator, or text in code is not a mention.
- A card whose requirements mention servers offers only the enabled ones they name. A card whose requirements mention none offers every enabled server.
- A mention that names no enabled server, or a requirement edited since acceptance, offers no MCP tools, and the trace says why.
- A member sees only the tribe's slugs, whether each is enabled, and its tool count.
- A mention in the pull request body does not notify anyone on GitHub.

### REQ-090 Reviewers before GitHub verification

The creator of a requirement, or a Lead Developer member, names any enabled member of the tribe as a reviewer, with or without a verified GitHub login. GitHub is asked for a reviewer's review once they have a verified login, and the trace lists the reviewers still waiting (RFC-103).

#### Acceptance

- The reviewer picker lists every enabled tribe member except the creator, and labels one without a verified login "No verified GitHub login".
- Saving an unverified member as a reviewer succeeds. A non-member, a disabled user, or the creator is rejected.
- The reviewer request on the pull request names only verified logins, and the `reviewers_requested` event lists the others as waiting.
- A reviewer who verifies later is asked on the next push.

### REQ-091 Tribe and Project sections in the sidebar

The sidebar names the active tribe and the active project. The Tribe section lists Projects and Conventions for every member, and MCP servers and Members for a tribe admin or an admin. The Project section lists Repository, Linked projects, Deploy, and Conventions of the selected project, and hides when no project is selected.

#### Acceptance

- The section headings show the tribe name and the project name.
- A member who cannot manage the tribe sees no MCP servers or Members link.
- On an MCP server page, MCP servers is the current link, not Members.

### REQ-092 Layered conventions

A tribe admin or an admin picks stacks and skills for every project of the tribe and switches company rules off for the tribe. A Lead Developer member, a tribe admin, or an admin switches each stack, skill, and rule on or off for one project, over the tribe. Every item shows where it comes from (RFC-104).

#### Acceptance

- A skill the tribe enables reaches every project of the tribe, and a project can switch it off.
- A rule the tribe switches off leaves its projects' packs, and a project can switch it back on.
- A disabled catalog item never applies.
- A member reads the tribe's conventions and cannot save them.
- Skills enabled and stacks added by hand before this change stay in place.

### REQ-093 Card context

A person who may run a card attaches or detaches its skills and rules, and detaches its MCP servers. A Lead Developer member, a tribe admin, or an admin also attaches an enabled server of the tribe. The next attempt reads the change, and the trace records it (RFC-105).

#### Acceptance

- A Developer attaches a skill and detaches an MCP server; attaching a server as a Developer is rejected.
- The next attempt's pack carries an attached skill and leaves out a detached rule.
- An attached server is offered with its allowlisted tools, a detached one is not, and a requirement edited since acceptance still offers none.
- A `done` card's context does not change, and the card status never changes.

### REQ-094 Remote runners

An admin mints a registration token, and an operator installs a runner on a machine with one script. The runner claims implement jobs over HTTPS while the control plane keeps the board, every status change, and every credential that writes to GitHub or calls a model. The Runners page shows each runner as idle, busy, draining, or offline (RFC-106 to RFC-109).

#### Acceptance

- A registration token works once and within one hour, and the database stores only hashes of tokens and runner secrets.
- A runner secret passes only `/api/runner/*`, and a runner reaches only the leases it holds.
- Two runners never claim the same run request, and a project's open leases stay under its implementation cap.
- A runner that stops heartbeating goes offline, and its card's open attempt is cancelled and its run request queued again.
- A remote attempt reaches `pr_open` through the same transitions and gates as a local one, and a runner cannot move a card.
- No response to a runner carries `databaseUrl`, `tokenKey`, a model key, or the owner's GitHub token.

### REQ-095 Push notifications

The server pushes changes to open pages instead of the pages polling, and tells the people who must act on a card: reviewers when a card waits for review, the people who can ship an approved card, and the owner when the card is blocked, needs changes, or is finished. A notification shows in the app's bell and, for people who turned it on, as a browser notification even when the tab is closed (RFC-113).

#### Acceptance

- A status change written by any process reaches an open board and card page without a reload, with the viewer's own actions.
- The board does not poll while its stream is open.
- A card moved to `human_review` notifies its requirement reviewers, or the members who may approve when it has none, and never the person who moved it.
- One event never notifies the same person twice, even with two `serve` processes.
- A browser notification carries only a title, the card title and project name, and a link. It never carries a reason, note, code, log, or token.
- A subscription endpoint that is not `https` on a known push service is refused, and the VAPID private key is never returned by the API.

### REQ-096 Feedback where the person acts

A person sees the result of what they did next to where they did it. A form that fails says why at the top of the form and under each field it blames, and moves focus there. A button shows that its request is running. An outcome with no form, such as a saved setting, a queued run, or a failed deploy, shows as a toast that closes by itself. The top bar carries no status text.

#### Acceptance

- A failed form submit shows the message in an alert at the top of the form, each field error under its field, and focuses the first invalid field, else the alert.
- Every field error the API returns is shown, under its field or listed in the form's alert.
- A wrong email or password on the login page says "Email or password is wrong." and clears the password.
- A wrong current password on the Account page is a field error and does not sign the person out.
- A success or an error from an action with no form shows as a toast, announced to screen readers, that the person can dismiss.
- Every control and toast button is at least 44px tall, and an icon on a button never changes its accessible name.

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

Project settings live in that project's `config.yaml` and secrets (REQ-045). A Lead Developer member, a tribe admin, or an admin edits them from the settings pages (REQ-044, REQ-065). An admin edits the company GitHub account (REQ-052). Each user edits their own GitHub settings (REQ-051). The full shape is in [tech/0002-data-and-api.md](../tech/0002-data-and-api.md). The defaults that shape behavior:

| Setting | v1 default |
| --- | --- |
| Attempts per card | 3 |
| Model turns per attempt | 40 |
| Tokens per card | 2000000 |
| Wall minutes per attempt | 30 |
| Parallel implementation cap | 2 |
| PRD and RFC paths | `docs/prd`, `docs/rfc` |
| Review independence | On. The reviewer is another family than the author when one is configured |
| Sandbox engine | Podman |
| Environment | `dev` only |

Model ids, the project repository, dev hosts, deploy and rollback scripts, the sandbox image digest, and check commands have no default. Each is chosen per project. A card whose job needs a missing setting blocks with a reason that names the section.

Git host default is GitHub. The card store is PostgreSQL, reached by `databaseUrl` in the data directory (RFC-044). The git host stores the pull request. It does not store card status as the source of truth.

## Flows

### Happy path

A member runs `ingest` from the CLI or the Intake page, or adds requirements by hand. A Product Owner member reads the draft and accepts it. The planner has written two cards, B depending on A. `run-next` locks A, routes `implement` or `implement_hard`, and creates `orch/A`. The inner loop commits and the local check exits 0. One pull request opens. CI goes green. The reviewer model recommends approval. A permitted member approves the head SHA from the CLI or the card page. A Lead Developer member starts deploy. The deployer ships that SHA, health passes, the dev check exits 0, and A is `done`. A later `run-next` may start B.

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
| Project | One repository in the company GitHub organization, and its cards, settings, and worker. |
| Linked project | Another project in the same tribe whose requirements, decisions, and files a project reads as context. |
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
13. The company GitHub account, project repositories, and linked projects (REQ-052 through REQ-054).
14. Manual requirements and senior accept (REQ-055).
15. Card owner, owner pull requests, and intake reviewers (REQ-056 through REQ-058).
16. Coding rules, stacks, and integration skills (REQ-059 through REQ-062).
17. Tribe admins, links at project create, and reviewers on the board (REQ-063, REQ-054, REQ-058).
18. User display names (REQ-064).
19. Admins and tribe admins edit project settings (REQ-065).
20. GitHub confirms a project's repository (REQ-066).
21. Run one ready card (REQ-067).
22. Accepted requirements become cards (REQ-068).
23. A queued card runs with a real model up to a pull request (REQ-069).
24. Solo approval (REQ-070).
25. Approval on GitHub and the Changes panel (REQ-071).
26. Checks decide when the AI review gives no verdict (REQ-072).
27. Self review on a requirement (REQ-073).
28. Retry after a failed attempt, and the AI review mode (REQ-074).
29. Instructions for the next attempt (REQ-075).
30. A live, verbose trace (REQ-076).
31. Thinking and effort per model (REQ-077).
32. The Claude Code-style agent loop (REQ-078 to REQ-082).
