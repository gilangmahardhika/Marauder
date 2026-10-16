# RFC-0060: Execution planning before coding

Status: Accepted for implementation. Date: 2026-10-16.

This RFC adds decisions `RFC-128`, `RFC-129`, and `RFC-130` for REQ-101. When a card requires a plan, the implementer's first attempts may only explore and submit a structured step plan. The card parks on Needs answer until a person edits and accepts the plan (or skips planning). An ephemeral AI chat helps revise the steps. It supersedes no status edge. It supersedes one UI placement sentence of RFC-114: when an open plan and open questions both exist, the card page shows the plan first.

Sprints 100 to 102 in [../plan/sprints.md](../plan/sprints.md) implement this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-128 | `cards.plan_mode` is `required` (default) or `off`. When required and the card has no accepted plan, the implement attempt is a planning phase: read-only tools, `todo_write`, `ask_user`, and `submit_plan` only. `submit_plan` ends with outcome `plan` (not counted) and moves `in_progress -> waiting_answer` | none |
| RFC-129 | An open plan is a title plus an ordered checklist of steps. A person who may answer edits, reorders, adds, or removes steps, then accepts. Accept moves `waiting_answer -> ready` and queues a run. Later packs carry the approved plan | RFC-114 card-page placement when both an open plan and open questions exist: show the plan first |
| RFC-130 | `POST .../plan/chat` streams an ephemeral plan chat (ask or revise steps) with the implement route's model. Apply takes the latest revised steps into the editor. No thread is stored | none |

## RFC-128 Plan mode and the planning phase

### Problem

The implementer started editing before a person saw the approach. A wrong approach cost an attempt and a review round.

### Decision

- `cards.plan_mode` is `required` or `off`, default `required`. The card JSON carries `planMode`.
- `PUT /api/projects/{projectId}/cards/{id}/plan-mode` with `{ "planMode": "required" | "off" }`. Another value is `422`. The caller needs the card's `plan_mode` action (same people as `question_mode`). A `done` or `wont_fix` card is `409`. A `plan_mode_changed` event records the old and new mode. An attempt reads the mode once when it starts.
- When `planMode` is `required` and the card has no row in `card_plans` with `status = accepted`, the attempt is a **planning phase**.
- In a planning phase the allowed tools are: `list_files`, `search_files`, `read_file`, `read_linked_file`, `todo_write`, `ask_user`, `submit_plan`, and tribe MCP tools. `replace_in_file`, `edit_file`, `delete_file`, `move_file`, `run_local_check`, `run_command`, and `commit` reply `error: planning phase: submit a plan before editing`.
- `submit_plan` takes `{ "title", "steps": [ { "text", "detail?" } ] }`. Title is 1 to 200 characters. One to 30 steps; each `text` is 1 to 500 characters; `detail` is at most 1,000 characters. A bad argument is `error: ` to the model.
- `submit_plan` runs alone in its turn. It is refused after a `commit` (same rule as `ask_user`). It stores one open plan (replacing any previous open plan of the card), stops the loop with `stop = "plan"`, finishes the attempt with outcome `plan` without `AddAttemptUsed`, and the job moves `in_progress -> waiting_answer` with the attempt id. No check runs. The lock is released.
- A remote runner reports outcome `plan`; the control plane makes the same move. The runner writes the plan through its lease and never writes a status.
- When the company agent is Claude Code and the attempt is a planning phase, the harness runs the built-in tool loop for that attempt. The next execute attempt may use Claude Code again.
- With `planMode` `off`, or when an accepted plan already exists, the attempt is a normal execute phase with every implementer tool.
- Switching to `off` does not accept or discard an open plan by itself. The card page offers **Skip plan** (discards the open plan, moves to `ready`, queues a run) when `planMode` is `off` and the card is `waiting_answer` with an open plan and no open questions. **Accept plan** remains available while parked.

### Why

Parking on `waiting_answer` reuses Needs answer, frees the worker slot, and keeps one conversation per attempt (NG-008). Not counting the plan attempt keeps planning cheaper than guessing in code. Default `required` makes the gate the safe path; `off` keeps routine cards fast.

## RFC-129 Structured plan and accept

### Decision

Storage:

- `card_plans` keeps one row per submitted plan: `project_id`, `card_id`, `attempt_id`, `title`, `steps_json` (array of `{ "id", "text", "detail" }`), `status` (`open` or `accepted`), and when accepted `accepted_by`, `accepted_at`.
- At most one `open` plan per card. A new `submit_plan` replaces the open row.
- Plan text is redacted with the attempt's secrets before storage. It is untrusted model output until a person accepts; pages render it as text. An accepted plan that the person edited is that person's choice for the card, like answers (fed to later packs inside untrusted markers).

API and actions:

- Card JSON may include `plan`: the open plan when present, else the latest accepted plan, or null. The board list does not need full steps; the single card does.
- `accept_plan` is listed when status is `waiting_answer`, an open plan exists, and the caller may answer.
- `PUT .../cards/{id}/plan` with `{ "title", "steps": [ { "id?", "text", "detail?" } ] }` updates the open plan without changing status. Missing `id` values are assigned. Validation matches `submit_plan` caps. Not `waiting_answer` or no open plan is `409`.
- `POST .../cards/{id}/plan/accept` with the same body accepts those steps (or the stored open plan when the body omits steps and keeps the open title/steps). One transaction: update steps, set `accepted`, move `waiting_answer -> ready` with the actor, insert a card-scoped run, append `plan_accepted`.
- `POST .../cards/{id}/plan/skip` when `planMode` is `off`, status is `waiting_answer`, an open plan exists, and no open questions: discard the open plan, move to `ready`, queue a run, append `plan_skipped`.

The pack:

- Every later attempt's implementer pack has an "Approved plan" section after the answers (or after instructions when there are no answers): title and numbered steps with detail. The section is data; the model follows it as the person's plan for this card.

Notification:

- `-> waiting_answer` after outcome `plan` notifies the owner with kind `plan`, title "Agent has a plan", and a browser push. The link opens the card. An `ask_user` park keeps kind `question`.

UI:

- With `accept_plan`, `CardPlan` replaces `CardActions` in Next step (and replaces `CardQuestions` when both would show). The person reorders, edits, adds, and removes steps, then Accepts.
- `PlanModeField` sits with the answer mode in Details.

### Why

A checklist is editable without asking the model again. Accepting through `Transition` keeps one status writer. Carrying the plan in every later pack matches how answers work.

## RFC-130 Plan chat

### Decision

- `POST /api/projects/{projectId}/cards/{id}/plan/chat` with `{ "message", "title", "steps", "model?", "modelOptions?" }` streams a reply. The caller needs `accept_plan` (or the same rights while an open plan exists on `waiting_answer`).
- The harness sends one request to the chosen model (default: implement route), task class `plan_chat`, capped like refine, timeout 2 minutes. The prompt holds the card's requirements summary, the current title and steps, and the message. The model is asked to either answer in prose or return a revised plan as JSON `{ "title", "steps": [ { "text", "detail" } ] }` when the person asks to change the plan.
- Stream framing matches refine: text pieces, then a final JSON object when a revision is offered: `{ "revision": { "title", "steps" } }`. Pure Q&A has no revision object.
- No conversation is stored. A follow-up sends the latest steps from the editor and the new message (like refine chat).
- Apply on the page replaces the checklist with the revision; it does not call Accept.
- No model / no implement route is `409 router_config`. Field errors are `422` JSON before the stream. Plan text remains untrusted.

### Why

A short chat lets the person understand or reshape the steps without starting a new implement attempt. Keeping each turn a separate request matches refine chat and avoids storing threads.

### Tests

- `TestSubmitPlanParksCard`, `TestPlanModeOffSkipsGate`, `TestWriteToolsBlockedUntilPlanAccepted`, `TestAcceptPlanQueuesRun`, `TestPlanStepValidation`, `TestPackIncludesApprovedPlan`, `TestPlanChatStreams`, `TestPlanModeRights`, `TestRunnerPlanOutcome`, `TestSkipPlanWhenOff`.
- Vitest: `PlanModeField`, `CardPlan`, card page shows the plan with `accept_plan`.
- Fixture scenario `planning`.
