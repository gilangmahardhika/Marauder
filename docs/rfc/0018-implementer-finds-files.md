# RFC-0018: The implementer can list and search the worktree

Status: Accepted for implementation. Date: 2026-10-11.

This RFC adds decision `RFC-064`. It supersedes in part the `implementer` tool list in RFC-001, as amended by RFC-051, and the same row in [tech/0003](../tech/0003-backend.md). Each role still has one fixed pack and one tool list.

Sprint 27 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-064 | The implementer gets the read-only tools `list_files` and `search_files`, and a rate-limited model call waits before its retry | RFC-001 and RFC-051 implementer tool list; RFC-063's immediate retry on a `429` |

## RFC-064 The implementer gets the read-only tools `list_files` and `search_files`

### Problem

The implementer pack carries the card, its requirements, conventions, and linked context, but no file map. With only `read_file`, a model has to guess paths in a repository it cannot see. On the first real card (change a label on the login page of a Laravel app), the right file was `resources/js/Pages/Auth/Login.tsx`, and nothing in the pack named it.

### Decision

The `implementer` tool list becomes `list_files`, `search_files`, `read_file`, `read_linked_file`, `edit_file`, `run_local_check`, `commit`.

- `list_files(path)` returns the regular files under a worktree directory, one repository-relative path per line, up to 500 paths, then `truncated: true`. An empty path is the root.
- `search_files(text)` returns `path:line: text` for each line that contains the exact text, up to 100 matches, then `truncated: true`. Each line is cut at 200 bytes. Files over 1 MiB and files with a NUL byte are skipped.

Both tools resolve the start directory through `safePath` (0006). They never enter `.git` or `.orchestrator`, and never follow a symlink, so a link that points outside the worktree is not read. Neither writes. Results go back to the model like any other tool result and are never logged.

The implementer system prompt names the new tools and the order of work: find the files, read them, edit, run the local check, and commit once it exits 0.

### Model base URLs

A family `baseUrl` may be pasted with its endpoint. The `openai` client drops a trailing `/chat/completions`, and the `anthropic` client drops a trailing `/messages` and `/v1`, because each SDK adds those parts. Before this, Groq saved as `https://api.groq.com/openai/v1/chat/completions` failed with 404.

### Rate limits

A `429` reply becomes `providers.RateLimited` with the wait the server asked for (`retry-after-ms`, then `retry-after`). The routed provider waits that long, 10 seconds when the server does not say, never more than 60 seconds, and sends the same call once more before the router counts it as a failure. A cancelled job stops waiting. Free tiers such as Groq's limit tokens per minute, and the implementer's growing prompt reaches that limit within a few turns.

The routed provider reports the model that answered, and the `model_call` event records it, so a call served by the fallback names the fallback.

### The model sees its own turns

The implement loop used to append each tool result to one growing user message and never sent the model's own replies back. With no record of its plan, a model reread the same files and repeated the same search until it ran out of turns. Now `providers.Request` carries `Turns`: each earlier reply with its tool calls, and the result of each call. The `openai` client sends them as an assistant message with `tool_calls` followed by one `tool` message per call. The `anthropic` client sends an assistant message with `tool_use` blocks followed by a user message with `tool_result` blocks. A call without a provider id gets `call_<turn>_<n>`.

When three or fewer model turns remain, the newest result ends with `<n> model turns left in this attempt. If the change is done, call commit now.` The system prompt also asks for the smallest change that meets the requirements, with each file written once. A model that edited the right file kept polishing it until the turn cap without committing.

### The history has a budget

Groq's free tier refuses any single request over 8000 tokens (7000 input tokens for some models). Each request sends the newest tool result in full up to 12 KiB, then older results while all results stay under 16 KiB. Older results become `(older result omitted)`, and their calls stay, so the model still sees which files it read, searched, and wrote. The file text of an earlier `edit_file` call is replaced by `(omitted: already written to the file)`. A result cut at 12 KiB ends with `truncated: true`. Known limit: a file longer than 12 KiB cannot be shown whole, and `edit_file` replaces whole files, so such a file needs a model with a larger limit or a smaller change.

### Each attempt starts from the last commit

After it takes the card lock, an attempt runs `git reset --hard HEAD` and `git clean -fd` in the card worktree, then reads the base SHA. Edits an earlier attempt left uncommitted, because it stopped on a provider error or ran out of turns, are gone. Before this, the next attempt saw those edits as the repository and could commit them without having made them. `search_files` results also repeat the searched text, so the model can tell its searches apart.

### Job errors

`job_error` joins the event kinds in [tech/0002](../tech/0002-data-and-api.md). RFC-063 already wrote it, but the trace package refused the unknown kind, so a `provider_error` block carried no message.

### Tests

`TestImplementerTools` lists the seven tools and still finds no `deploy` or `verdict`. `TestListAndSearchFiles` finds a file by path and by text, skips `.git`, does not follow a symlink out of the worktree, and rejects `..`. `TestOpenAIEndpointBaseURL` and `TestAnthropicBaseURL` cover pasted endpoints. `TestRateLimitWaitsThenRetries` waits for `retry-after` and succeeds on the same model. `TestImplementProviderErrorRecordsMessage` blocks with `provider_error` and keeps the message in a `job_error` event. `TestDiscardChangesKeepsCommits` drops an uncommitted edit and an untracked directory and keeps the commit. `TestHistoryKeepsNewestResults` keeps the newest results, cuts a long one, omits the rest, and drops `edit_file` text. `TestOpenAISendsTurns` and `TestAnthropicSendsTurns` check the message shapes.
