# RFC-0035: Refine one field at a time, streamed

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-087` under REQ-083 and amends REQ-083. It supersedes earlier statements only where it names the id. Those statements stay in their files.

Sprint 51 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-087 | Each of the statement and the acceptance lines has its own refine button. The model writes the new text of that field only, and the page shows it token by token as it arrives | RFC-086 "A member can ask the routed implement model to refine a manual requirement's statement and acceptance lines" (one refine for the whole requirement); RFC-086 "The request offers one tool, `propose_refinement(statement, acceptance, notes)`" and everything it says about `notes`; RFC-086 "Set aside: Streaming the proposal" |

## RFC-087 Refine one field at a time, streamed

### Decision

- The new-requirement form and the edit page of a manual requirement show a spark icon button beside the Statement label and beside the Acceptance label. Each opens a box under its field with an optional "What to improve" input and a Refine button. The two fields refine on their own, and both may run at once.
- `POST /api/projects/{projectId}/requirements/refine` takes `field` (`statement` or `acceptance`) as well as `statement`, `acceptance`, `requirementId`, and `instructions`. Input errors are `422` and a missing implement route is `409`, as JSON, before anything streams.
- Otherwise the reply is `200` with `Content-Type: text/event-stream`: one `start` event with `modelId`, a `delta` event per piece of text, then `done` with the whole text (and, for acceptance, the lines) or `error` with a code and a message. A closed connection cancels the model request.
- The request has no tools. The model writes only the new text of the field: the statement as markdown, or one acceptance check per line. The other field is in the pack as context. The draft, the other field, and the guidance are untrusted data inside the markers.
- Both provider clients stream. The routed provider waits on a rate limit, retries, and falls back as before, but only until the first piece of text reaches the caller. After that an error ends the request, because the member has already seen part of the reply.
- Text is redacted as it streams. The last bytes that could start a model key are held back until the next piece shows whether they do, so a key split across pieces never reaches the page.
- The finished text passes the save rules for its field: a statement that is not empty and at most 10,000 characters, or at most 50 acceptance lines of at most 1,000 characters with list markers dropped. Otherwise the stream ends with `error`.
- When the text is done the box shows that field's comparison, word by word for the statement and line by line for acceptance, with Apply and Discard. Apply changes only that field in the form. Nothing is saved until the member saves, and a save clears acceptance as before (REQ-004).
- The `requirement_refined` event gains `field`. It still has no text.

### Why

A requirement's statement and its acceptance lines need different help: a statement wants clearer intent, acceptance lines want one testable check each. Refining them apart keeps each prompt small and each comparison readable. A reply that appears as it is written shows progress within a second or two instead of a blank wait, and the member can stop a reply that is going the wrong way.

### Set aside

- Refining the statement and acceptance in one request.
- Notes from the model on what it changed. The comparison shows the change.
