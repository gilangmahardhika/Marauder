# RFC-0056: Choose the model of a refine

Status: Accepted for implementation. Date: 2026-10-15.

This RFC adds decision `RFC-124` under REQ-083. It supersedes RFC-086 only where a refine must use the implement route's model. The stream, the save rules, and the comparison of RFC-087, and the refine box of RFC-123, are unchanged.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-124 | A refine box chooses the model, thinking, and effort of its request. It starts on the implement route's model with that model's settings, and the reply streams as before | RFC-086 "A member can ask the routed implement model to refine" (only that the model is always the implement route's) |

## RFC-124 Choose the model of a refine

### Decision

- The refine box shows Model, Thinking, and Effort above "What to improve". Model lists every model in the LLM settings (`GET /api/settings/llm`, which any signed-in user may read), and starts on the implement route's model (`GET /api/settings/router`). Thinking and Effort offer the values the selected model's protocol takes (RFC-079), with "Model default" first. Both refine boxes of a form share the three choices, and changing the model drops a thinking or effort value its protocol does not take.
- `POST /api/projects/{projectId}/requirements/refine` takes optional `model`, `thinking`, and `effort`. Empty values send the request RFC-087 describes: the implement route's model, then its fallback, each with its own saved settings.
- A `model` other than the implement route's, or any `thinking` or `effort`, calls that one model with no fallback. The rate-limit wait and the one retry still apply until the first text. A chosen thinking or effort replaces the model's saved value for this request only, and "Model default" keeps the saved one. Thinking On keeps the model's saved budget, or 8,192 when it has none.
- A model that is not in the LLM settings, or whose family has no usable key, is `422` on `model`. A thinking or effort value the model's protocol does not take is `422` on `modelOptions`. Both are JSON replies before anything streams. With no `model` and no implement route, the reply is still `409 router_config`.
- The `requirement_refined` event's `modelId` names the model that answered, as before.

### Why

A refine is a short, interactive request. A member who wants a faster or a more careful answer for one draft should not need an admin to change the implement route, which every card's implementer also uses. Starting on the route's model with its saved settings keeps the request a member gets without touching anything the same as before.

### Set aside

- A thinking budget control in the refine box.
- Remembering the choices between page loads.
- Showing the model's reasoning. Only the reply text streams (RFC-079, RFC-087).
