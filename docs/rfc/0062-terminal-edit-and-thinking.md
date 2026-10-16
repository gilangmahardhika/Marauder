# RFC-0062: Terminal edit cards and live thinking

Status: Accepted for implementation. Date: 2026-10-16.

This RFC adds decision `RFC-132` for REQ-076. It supersedes in part the implementer `toolArgs` rule that file text is never kept on `tool_call` events, RFC-088's "Only reply text streams: never reasoning", and RFC-079's set-aside "Storing or showing the reasoning on the Trace page", only for capped edit snippets on the Trace and for live thinking display on the Trace terminal view.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-132 | Successful `edit_file` and `replace_in_file` `tool_call` events may store capped, redacted `old` and `new` text for the Trace UI; the Trace page may show live `thinking` stream frames on the terminal view without storing them | tech/0003 "File text is never kept: edit_file records its size only"; RFC-088 "Only reply text streams: never reasoning (RFC-079) and never tool arguments"; RFC-079 set-aside "Storing or showing the reasoning on the Trace page" (showing live only; still not stored) |

## RFC-132 Capped edit snippets and live thinking on Trace

### Problem

The terminal Trace view cannot show Cursor-like code diffs because `toolArgs` drops file text. Live model thinking is already published on the stream for runner logs but the Trace UI ignores it, so a person watching an attempt cannot see reasoning between edits.

### Decision

- `tool_call` args for `replace_in_file` keep path, sizes, and `replaceAll`, and add `old` and `new` clipped with `trace.Clip` at `DiffCap` (8 KiB) after secret redaction.
- `tool_call` args for `edit_file` keep path, `bytes`, and `lines`, and add `new` from the written text at `DiffCap`. When the file existed before the write, they also add `old` from that prior content at `DiffCap`. A new file has no `old`.
- Provider history and pack building still omit full edit bodies as before (`sentCall` placeholders, no pack use of these fields).
- The Trace page may render live `thinking` frames on the terminal view for the current turn. Thinking is shown only, never stored in board events, never returned into a pack, and cleared when the turn ends or live ends.
- Tool arguments still do not stream live; capped snippets appear only after the tool finishes and the `tool_call` event is stored.

### Why

A person following an attempt needs the same shape of feedback Cursor shows: what changed in each edit, and what the model is thinking between steps. Caps and redaction keep secrets and event size bounded. Leaving thinking unstored matches RFC-079's storage rule while allowing the live display that RFC-079 set aside.

### Set aside

- Structured Trace rows as edit cards.
- Persisting thinking on `model_call` or other events.
- Streaming tool argument bodies as they are generated.

## Tests

- `TestToolCallArgs`: `edit_file` and `replace_in_file` args include clipped `old`/`new` (or `new` only for a new file); secrets are redacted; oversize text is cut at `DiffCap`.
- Terminal Vitest: an entry with `old`/`new` builds an edit card; a `thinking` frame updates pending thinking on the terminal view.
- Trace page: the toolbar with Follow latest is sticky while viewing the structured (or live) trace.
