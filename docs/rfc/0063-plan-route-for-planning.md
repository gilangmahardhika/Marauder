# RFC-0063: Plan route for execution planning

Status: Accepted for implementation. Date: 2026-10-20.

This RFC adds decision `RFC-133` under REQ-101. It supersedes in part RFC-128 and RFC-130 where those used the implement route's model for the planning phase and for plan chat. Tool gates, Built-in forcing under Claude Code, accept/skip, and pack carry-forward are unchanged.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-133 | A planning-phase attempt and plan chat (default) use `router.routes.plan`. After an accepted plan (or with plan mode off), the execute attempt uses `implement` / `implement_hard` as before | RFC-128 (implied implement route for planning attempts); RFC-130 "with the implement route's model" / "default: implement route" / "No model / no implement route" |

## RFC-133 Planning uses the plan route

### Problem

The company Router page has a `plan` class, but card plan mode and plan chat called the implement route. An admin who set a cheaper or stronger planning model never saw it used.

### Decision

- When `PlanningPhase` is true, the harness resolves `router.routes.plan` (model and fallback), sets the attempt's task class to `plan` (migration `00037_plan_task_class.sql`), and runs the Built-in tool loop with the planning tool gate (RFC-128). Claude Code is still not used for that attempt.
- An empty or unusable plan route blocks the card with `router_config` and a message that names the plan route / Router page. There is no fallback to the implement route or to the Claude Code agent model.
- `POST .../plan/chat` defaults to the plan route's model. A caller may still pin another model. Missing plan route is `409 router_config`.
- After the person accepts the plan (or plan mode is off, or an accepted plan already exists), `PlanningPhase` is false and model selection is unchanged: Built-in uses `implement` / `implement_hard` from `TaskClass(risk, attempts)`; Claude Code uses the agent model when configured.

### Why

Planning and coding are different jobs. The Router already named a `plan` class; wiring execution planning to it matches the admin UI and keeps implement models for edit/commit attempts.

### Set aside

- Changing which tools the planning phase may call.
- Using the plan route for intake drafting beyond the existing `plan` task-class name.

## Tests

- `TestPlanningUsesPlanRoute`: planning calls the plan model with task class `plan`; after accept, the next attempt uses the implement model.
- `TestPlanningMissingPlanRouteBlocks`: empty plan route blocks with `router_config`.
- `TestPlanChatStreams`: default plan chat request uses the plan route model.
