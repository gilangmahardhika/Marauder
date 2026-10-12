# RFC-0031: A terminal view of the trace

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-078` under REQ-076. It adds a second way to read the Trace page from RFC-074: every stored event as plain terminal output, oldest first, with the full text the event holds. It reads the same route and stores nothing new. For the terminal view only, it supersedes one sentence of RFC-074, "The Trace page polls every 3 seconds while `live` is true".

Sprint 43 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-078 | The Trace page has a Structured and a Terminal view, kept in `?view=terminal`. The terminal view prints every stored event as a head line over its full text, follows new output, and polls every second while `live` is true | RFC-074's 3-second poll, in the terminal view only |

## RFC-078 Terminal view

### Problem

The timeline (RFC-074) is a list of one-line rows. To read what the model said or what a tool returned, a person opens rows one at a time, and long text is cut at 4,000 characters. While a card works, a person wants to watch it the way they would watch a terminal: every reply and every tool output, in order, as it arrives.

### Decision

- The Trace page toolbar has a View switch, Structured or Terminal. Terminal sets `?view=terminal`; Structured removes it. Search, Kind, and Attempt filter both views.
- The terminal view is one dark, monospaced log with `role="log"`, oldest first. Each event is a head line and the text under it, from the stored payload only:
  - A model call: `model  role  modelId  N in / N out  stop reason`, then the reply text.
  - A tool call: `$ name  target`, then the tool's whole stored output. A refused call adds `(refused)`. A failed or refused call, and a local check that does not start with `exit 0`, is red; a passing check is green.
  - A status change: `transition  from -> to  evidence`. A move to `blocked` is red.
  - An error: `! job_error`, then the whole message.
  - Any other kind: its name and the description the timeline uses. A kind the page does not know shows its payload as JSON.
  - A line `--- attempt N  taskClass  modelId  ---` marks where another attempt's events begin.
- Control characters and ANSI color codes are stripped, and every value renders as text.
- Text past 200 lines folds behind "Show all N lines". The view renders the newest 300 events, and "Show earlier events" adds 300 more.
- The view follows new output. Scrolling up stops following and shows "Jump to latest", which follows again.
- While `live` is true, the terminal view polls every second. The structured view keeps 3 seconds.

### Why

The server already stores the full tool output and the model reply, so the view needs no new route, column, or event. A one-second poll with `since` returns only the overlap and new rows, which is cheap next to a model call. Folding at 200 lines and rendering 300 events keeps the page fast on a long attempt.

### Set aside

- Streaming tokens or subprocess output as they are produced. That needs a server push channel and storing partial output, and the stored events already arrive within a second.
- Storing more than the existing caps, such as the whole model reply past 4 KB.

## Tests

- `terminal` Vitest: each kind's head, text, and tone; a failed or refused tool is red; an unknown kind shows JSON; ANSI codes are stripped; the attempt line; `clipLines` folds past 200 lines.
- `TerminalView` Vitest: heads and output render; "Show all N lines"; "Show earlier events" past 300; scrolling up shows "Jump to latest", which follows again.
- Trace page Vitest: with `?view=terminal` the log renders and the Kind filter applies; a live card polls every second in the terminal view and every 3 seconds in the structured view.
