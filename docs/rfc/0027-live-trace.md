# RFC-0027: A live, verbose trace

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-074` for REQ-076. It amends REQ-041's list of `model_call` fields, which gains `text`, and supersedes in part the RFC-021 Set-aside "storing the raw provider payload", only for the named fields below. RFC-007 and RFC-012 stand: stored text never goes back into a pack or to the reviewer.

Sprint 39 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-074 | The Trace page refreshes itself while a card is working, and each model call and tool call records what the agent said and what it asked for, redacted and capped | RFC-021 Set-aside on provider payloads, for `model_call.text` and `tool_call.args` only |

## RFC-074 A live, verbose trace

### Problem

A person who presses Run cannot follow the attempt. The Trace page loads once and stays still until a reload. When it is reloaded, a tool row says "Read file · 120 lines" without the file, a search row has no search text, a commit row has no message, and a model row has tokens but none of what the model said. The person cannot tell whether the agent is on track until the attempt ends.

### Decision

Every event is already committed when it is written, so the page reads the same route again while the card works. No push channel is added: a browser `EventSource` cannot send the bearer, and the bearer stays out of cookies and URLs (tech/0006).

- `GET /api/projects/{projectId}/cards/{id}/trace` takes an optional `since`, an RFC3339 time. With it, the route returns the card's events from 10 seconds before `since` onward, so an event that committed late or in the same second is not lost. The client merges events by `id`. A value that is not RFC3339 is `422` on `since`.
- Each event in the response has `seq`, its insert order. Each attempt has `maxTurns`. The response has `live`, true while the card is queued, `in_progress`, `self_check`, `agent_review`, `deploying_dev`, or `verifying`, or has an attempt with no end. The Trace page polls every 3 seconds while `live` is true and stops when it turns false.
- A `model_call` event adds `text`, the model's reply text, at most 4 KiB.
- A `tool_call` event adds `args`, the arguments a person needs to follow the step, each value at most 1 KiB: `path` for `list_files`, `read_file`, and `edit_file`; `project` and `path` for `read_linked_file`; `text` for `search_files`; `message` for `commit`; `command` for `run_local_check`; and `body` for `comment_pr` and `verdict`. `edit_file` adds `bytes` and `lines` and never the file text.
- Each new string drops NUL bytes and invalid UTF-8, is redacted with the job's secrets, then is cut to its head and tail around `... (N bytes cut) ...`. Redacting before the cut keeps a secret from surviving in part. `trace.Append` redacts the whole payload again. The job's secrets add the `databaseUrl` and `tokenKey` text, as tech/0006 already lists.
- The prompt itself is still not stored; `attempts.prompt_hash` keeps its hash. No pack builder reads `events`, so the stored text never reaches a later attempt or the reviewer.

### Why

Polling the route the page already reads keeps auth, CORS, the CSP, and project scope as they are, and it sees events from the `run-next` CLI process as well as from the server. A time cursor with an overlap is correct where a bare `seq` cursor is not, because an event written inside a transaction can commit after a larger `seq` is visible. The arguments and the reply text are what a person reads to follow the agent. A cap per field keeps one event small, and the existing `job_error` order of steps already proved redact-then-cut.

### Set aside

- Server-sent events or a websocket. They need the bearer in a cookie or a URL, a shutdown hook, and proxy settings cloud mode does not define.
- Streaming the model's reply while it is generated. The providers return whole replies, and a partial reply would need many events or an update in place, which RFC-031 forbids.
- Storing the raw provider payload or the prompt. RFC-021's reason stands for everything not named above.

## Tests

- `TestTraceClip`: a secret containing `&` is redacted, a cut keeps valid UTF-8, and the head and tail survive.
- `TestModelCallText`: a model reply is stored redacted and capped.
- `TestToolCallArgs`: tool rows carry their arguments, and `edit_file` stores sizes and no file text.
- `TestTraceSince`: `since` returns the overlap including same-second events, a bad value is `422`, and another project is `404`.
- `TestTraceLive`: `live` is true while the card works and false once it is done.
- `TestReviewerPackHasNoAuthorText`: the reviewer prompt does not carry the implementer's stored reply text.
- Vitest: events merge by id, rows show their arguments and text, the page polls while live and stops after, and new rows stay visible.
