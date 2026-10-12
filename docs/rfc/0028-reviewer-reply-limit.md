# RFC-0028: The reviewer's reply limit and a cut-off reply

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-075` under REQ-072. It supersedes in part RFC-065 review job step 4 ("No verdict ... is a parse failure"), only for a reply that the provider cut at the token limit. RFC-069 stands: a review that still gives no verdict lets the pull request checks decide.

Sprint 40 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-075 | Reviewer requests may reply with up to 32,768 tokens. A reply cut at the limit with no tool call is asked again once on the same model, and a review that fails this way says so | RFC-065 step 4, for a reply cut at the token limit |

## RFC-075 The reviewer's reply limit

### Problem

Every model request without its own limit may reply with 8,192 tokens. A model that thinks before it answers can spend all of them thinking. The provider then stops with `max_tokens` (anthropic protocol) or `length` (openai protocol), and the reply has no text and no tool call. The review loop counts that as no verdict and stops at once. When the review route's fallback is the same model, nothing asks again, so one long thought costs the card its AI review. The job error and the "AI review: Not done" comment say "no valid verdict" and not why.

### Decision

- Every reviewer request sets `MaxTokens` to `loop.ReviewMaxTokens`, 32,768. Other task classes keep `providers.DefaultMaxTokens`, 8,192.
- `providers.Truncated(stopReason)` is true for `max_tokens` and `length`.
- A reviewer reply with no tool call and a truncated stop reason is asked again with the same request and model, once per review run. The retry uses one of the 8 turns. A second such reply ends the run with `loop.ErrReviewCut`, which wraps `loop.ErrReviewParse`, so the fallback rule of RFC-065 and the outcome of RFC-069 apply unchanged.
- A reply with no tool call that was not cut still ends the run with `ErrReviewParse`, as before.
- When the review fails with `ErrReviewCut`, the `job_error` reads "the reviewer gave no valid verdict: its reply hit the 32768-token limit twice", and the "AI review: Not done" comment gives the reason "The reviewer's reply hit the token limit before a verdict".

### Why

A cut reply is the provider's limit, not the model's answer, so asking again is likely to work where a reply that ended on its own would not. One retry bounds the cost: at most two cut replies, 65,536 output tokens, and both count toward the card's token budget like any model call. Wrapping `ErrReviewParse` keeps every existing branch, test, and the RFC-069 outcome.

### Set aside

- Turning thinking off or capping it for the reviewer. The protocols differ in how they take it, and not every compatible server accepts it.
- Raising the limit for every task class. The implementer's replies are tool calls with file text and have not been cut.
- Storing the hidden reasoning. RFC-021 and RFC-074 still keep only the reply text.

## Tests

- `TestReviewMaxTokens`: every reviewer request carries `MaxTokens` 32,768.
- `TestReviewRetriesCutReply`: one `max_tokens` reply, then `read_diff` and `recommend_approve`, reaches `human_review` on the route's model, with three reviewer `model_call` events, the first stopped at `max_tokens`.
- `TestReviewCutTwiceFails`: cut replies on the model and on the fallback store a `failed` verdict, one `job_error` naming the token limit, and one comment with the token-limit reason.
