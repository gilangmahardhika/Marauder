# RFC-0032: Thinking and effort per model

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-079` under REQ-077. It takes up one item that RFC-075 set aside, "Turning thinking off or capping it for the reviewer", and decides it for every task class through the model's settings. RFC-075's 32,768-token reviewer limit and its one retry of a cut reply stand.

Sprint 44 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-079 | Each model in an LLM family may set `thinking`, `thinkingBudget`, and `effort`. Every request to that model sends them, an empty value sends nothing, and the model's reasoning is sent back between tool turns of the same model | RFC-075 "Set aside: turning thinking off or capping it for the reviewer" |

## RFC-079 Thinking and effort per model

### Problem

A model that thinks before it answers spends its own default effort on every request. An admin cannot turn thinking on for a model that needs it, turn it off for one that wastes tokens, or pick a lower effort for a model that is slow. A compatible server, such as GLM, DeepSeek, Kimi, or Qwen, often thinks only when asked.

### Decision

- Each model in an LLM family has three optional settings, saved in the service `config.yaml` with the model id:
  - `thinking`: empty (the provider's default), `enabled`, `disabled`, or `adaptive`. `adaptive` is allowed on the `anthropic` protocol only.
  - `thinkingBudget`: tokens the model may think with when `thinking` is `enabled` on the `anthropic` protocol, from 1,024 to 32,768. Zero means `providers.DefaultThinkingBudget`, 8,192. Any other combination saves zero.
  - `effort`: on the `anthropic` protocol `low`, `medium`, `high`, `xhigh`, or `max`. On the `openai` protocol `none`, `minimal`, `low`, `medium`, `high`, or `xhigh`. Empty sends nothing.
- `GET` and `PUT /api/settings/llm` carry them in each family's `modelOptions`, keyed by model id. `models` stays a list of ids. A key that is not one of the family's models, or a value the protocol does not take, is `422` on `modelOptions`.
- `app.RoutedProvider` sends each request with the settings of the model it is calling, so a fallback uses the fallback's settings. "Test connection" sends the family's first model with its settings.
- The `anthropic` client sends `thinking` as `{"type": "enabled", "budget_tokens": N}`, `{"type": "adaptive"}`, or `{"type": "disabled"}`, and `effort` as `output_config.effort`. With `enabled`, `max_tokens` is the request's limit plus the budget, so the reply keeps its 8,192 tokens, or 32,768 for the reviewer.
- The `openai` client sends `effort` as `reasoning_effort`, and `thinking` as `{"type": "enabled"}` or `{"type": "disabled"}` in a `thinking` body field, which the compatible servers read. OpenAI itself takes only `reasoning_effort`, so its models leave `thinking` empty.
- A reply's reasoning is kept with the turn in memory and sent back in the next request of the same attempt when the request goes to the same model: the signed `thinking` and `redacted_thinking` blocks first in the assistant message on the `anthropic` protocol, and `reasoning_content` on the assistant message on the `openai` protocol. A turn that another model answered sends none.
- Reasoning is not stored. The `model_call` event keeps the reply text only (RFC-021, RFC-074), and thinking tokens count toward the card's token budget as output tokens, as the providers report them.

### Why

The setting belongs to the model, because whether a model can think and which effort values it takes depend on the model and its server, not on the task class. An empty value sends nothing, so every saved configuration sends the same request as before. Anthropic refuses a tool turn without the earlier signed thinking blocks once thinking is on, and the compatible servers that think on the `openai` protocol expect `reasoning_content` back the same way, so the reasoning has to travel with the turn. Raising `max_tokens` by the budget keeps RFC-075's reply limit meaning what it says.

### Set aside

- Thinking or effort per route. One model used by two task classes thinks the same in both.
- Storing or showing the reasoning on the Trace page.
- Streaming, which a long budget may need on some servers. The Anthropic client already sizes its timeout from `max_tokens`.

## Tests

- `TestNoModelOptionsSendsNothing`: with no settings, the anthropic and openai request bodies have no `thinking`, `output_config`, or `reasoning_effort`.
- `TestSaveLLMModelOptions`: `adaptive` on openai, a budget under 1,024 or over 32,768, an unknown effort, and an option for a model the family does not list are refused.
- `TestLLMSettingsModelOptions`: `modelOptions` saves and reads back through `PUT` and `GET`, a bad value is `422` on `modelOptions`, and the key stays hidden.
- `TestAnthropicThinking`: `enabled` sends the budget and raises `max_tokens` by it; `adaptive` and `effort` appear in the body.
- `TestAnthropicReplaysThinking`: signed thinking blocks come first in the next assistant message for the same model, and are left out for another model.
- `TestOpenAIEffortAndThinking`: the body has `reasoning_effort` and the `thinking` field.
- `TestOpenAIReplaysReasoningContent`: `reasoning_content` from a reply is sent back on the assistant message for the same model only.
- `TestRoutedProviderModelOptions`: a request sent to the fallback carries the fallback model's settings.
- `llm` Vitest: `keepKey`, `toDraft`, and `fromDraft` keep `modelOptions`.
- `FamilyForm` Vitest: Adaptive is offered on anthropic only, the budget shows only for On on anthropic, and the effort list follows the protocol.
