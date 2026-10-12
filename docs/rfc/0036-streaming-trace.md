# RFC-0036: Stream the card trace

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-088` under REQ-076 and amends REQ-076. It supersedes earlier statements only where it names the id. Those statements stay in their files.

Sprint 52 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-088 | While a card works, the Trace page holds one streamed connection. The server pushes stored events as they commit and the model's reply text as it is generated, so the terminal view shows the reply token by token. Live text is shown only, never stored | RFC-074 "No push channel is added: a browser `EventSource` cannot send the bearer, and the bearer stays out of cookies and URLs (tech/0006)"; RFC-074 "The Trace page polls every 3 seconds while `live` is true and stops when it turns false"; RFC-074 Set aside "Server-sent events or a websocket" and "Streaming the model's reply while it is generated"; RFC-078 "While `live` is true, the terminal view polls every second. The structured view keeps 3 seconds" and the "polls every second while `live` is true" part of its index row; the token half of RFC-078 Set aside "Streaming tokens or subprocess output as they are produced"; RFC-0033 Set aside "Streaming" (RFC-080 to RFC-085); RFC-032 "API handlers time out at 30 seconds", for the trace stream route only |

## RFC-088 Stream the card trace

### Problem

The Trace page polls the stored trace every second in the terminal view and every 3 seconds otherwise, and a model call appears only after the whole reply is back. A reply can take a minute, so a person watching the terminal sees nothing and then a block of text at once.

### Decision

- `GET /api/projects/{projectId}/cards/{id}/trace/stream` takes the same `since` as the trace route and has the same checks: a member of the project's tribe, a card in that project, and `404` otherwise. It replies `200` with `Content-Type: text/event-stream`. The browser reads it with `fetch` and the bearer header, as the refine stream does (RFC-087), so the bearer stays out of cookies and URLs.
- The first frame is `trace` with the events since `since`, the attempts, and `live`, from the database. While the stream is open the server reads the database again every second and right after a model call is stored, and sends any new events in another `trace` frame. Events written by a `run-next` process therefore still arrive.
- The implementer and reviewer loops in `serve` stream their model calls. Each turn sends `turn` with the attempt, turn number, role, and model, then `delta` frames with the reply text, and `turn_end` after the `model_call` event is stored. A viewer that connects mid-reply first gets the text so far. Only reply text streams: never reasoning (RFC-079) and never tool arguments.
- Streamed text is redacted with every job secret, the same list the stored events use. The last bytes that could start a secret are held back until the next piece shows whether they do, so a secret split across pieces never reaches the page.
- In the loops, the routed provider keeps its rate-limit wait, retry, and fallback after text has streamed. Before it sends the request again it sends `reset`, and the page drops that turn's text. The RFC-087 rule that ends a refine after its first text stays for refine.
- The stored trace is unchanged and is the record. `model_call.text` stays cut at 4 KiB, so a long reply shows whole while it streams (at most 64 KiB) and cut once its event arrives. Live text is never stored and never reaches a pack or the reviewer (RFC-007, RFC-012).
- The stream ends with `done` and `live: false` when the card stops working, with `done` and `reconnect: true` after 10 minutes so the bearer and membership are checked again on reconnect, and with `resync` when the server dropped a viewer that fell behind. A viewer never slows the agent. The server keeps at most 16 viewers per card and 64 in all; another is `429`. A comment line every 15 seconds keeps proxies from closing the connection, and a server shutdown ends every stream.
- The page opens the stream while `live` is true and the tab is visible. When the stream fails it polls the trace route as before and tries the stream again after 5 seconds. The terminal view shows the reply being written as its last entry, with a blinking caret, and the stored model call replaces it. The structured view says the model is writing.

### Why

A reply that appears as it is written shows within a second or two that the agent is on track, and lets a person read along instead of waiting for the whole turn. Pushing the stored events over the same connection removes the page's polling while it is connected, and reading the database on the server keeps one source of truth for every process. Keeping the live text in memory leaves the events table insert-only (RFC-031) and the 4 KiB cap as it is.

### Set aside

- Live text for jobs that a `run-next` process runs outside `serve`. Their stored events still arrive within about a second. PostgreSQL `LISTEN`/`NOTIFY` could carry them later.
- Streaming subprocess output, reasoning, or tool arguments.
- Storing the whole reply past 4 KiB.
