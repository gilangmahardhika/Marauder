# RFC-0034: Refine a requirement with AI

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-086` under REQ-083. It supersedes no earlier statement.

Sprint 50 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-086 | A member can ask the routed implement model to refine a manual requirement's statement and acceptance lines, compare the proposal with the current text, and apply it to the form. Nothing is saved until the member saves | None |

## RFC-086 Refine a requirement with AI

### Decision

- `POST /api/projects/{projectId}/requirements/refine` takes `statement`, `acceptance`, an optional `requirementId`, and optional `instructions` of at most 1,000 characters. Any member who can see the project may call it, the same rule as editing a manual requirement (RFC-053).
- The harness sends one request to the model of the `implement` route, with its fallback, through the same routed provider the implement job uses. The task class is `refine_requirement`, the reply is capped at 4,096 tokens, and the call times out after 2 minutes. When the route has no model the reply is `409 router_config`.
- The request offers one tool, `propose_refinement(statement, acceptance, notes)`. A reply without that tool call, or with another tool, is a `502`. The proposal must pass the same checks as a save (10,000-character statement, at most 50 acceptance lines of at most 1,000 characters), and `notes` is cut at 2,000 characters. Model keys are redacted from the returned text.
- The draft and the instructions are untrusted data inside the pack markers. The system prompt asks the model to keep the requirement's intent and scope, write a clear and testable statement, make each acceptance line one observable check, and say in `notes` what changed and why.
- The route writes nothing to `requirements`. It appends one project-level `requirement_refined` event with `requirementId` (or `new`), `modelId`, and token counts, and no requirement text.
- The new-requirement form and the edit page of a manual requirement have a "Refine with AI" button. The page shows the proposal against the text that was sent: the statement word by word and the acceptance lines line by line, plus the model's notes. Apply puts the proposal in the form, and the member may edit it and press Save. Discard keeps the form. Saving a refined requirement clears its acceptance like any other edit (REQ-004). PRD and RFC rows have no refine button.

### Why

Members write requirements quickly and leave statements vague and acceptance lines untestable, and the implementer then guesses. A second pass by the model the team already routes to implementation tightens the text before a card exists. The member stays the author: the model proposes, the member compares, and only a human save changes the record.

### Set aside

- Refining PRD or RFC rows into new manual requirements.
- Refining several requirements at once, or splitting one into many.
- Streaming the proposal.
