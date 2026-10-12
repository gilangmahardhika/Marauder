# RFC-0033: A Claude Code-style agent loop

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decisions `RFC-080` to `RFC-085` under REQ-078 to REQ-082, and amends REQ-017 and REQ-018. It supersedes earlier statements only where it names the id. Those statements stay in their files.

Sprints 45 to 49 in [../plan/sprints.md](../plan/sprints.md) implement this file.

## Evidence

17 real attempts on 2 cards of the live board:

- 8 of the 10 `no_commit` attempts spent every turn re-reading files, because the 16 KiB history had dropped the earlier results.
- `edit_file` replaces a whole file, and 11 times the model wrote the history placeholder `(omitted: already written to the file)` into `lint.yml`.
- 2 attempts passed the local check on their last turn and had no turn left to commit. A reply with no tool call ended the attempt.
- "nothing to commit" counted as a failure while the branch already held the fix. A podman exit 125, a provider `413`, and a malformed tool call each used up an attempt.
- `maxTokensPerCard` was never checked, no request used prompt caching, and the pack had no path map.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-080 | File tools read ranges, replace exact text, and refuse writes that destroy a file. Tool errors are flagged and tool results are redacted | RFC-064 tool list and "write each file once"; the `edit_file` history placeholder in RFC-064 "The history has a budget" |
| RFC-081 | The history budget is per model and stable, a reply without a tool call is nudged, a passing tree is committed by the harness, and the turn and token caps are enforced | RFC-064 "The history has a budget" (16 KiB fixed) and its three-turn warning; RFC-020 default of 12 turns |
| RFC-082 | The pack carries a path map, the repository's `AGENTS.md` and `CLAUDE.md` as data, each requirement's heading and acceptance lines, and the decisions that name them. A `todo_write` tool keeps a plan, and read-only calls run concurrently | RFC-055 "Set aside: reading `AGENTS.md`, `.cursor/rules`, or similar files from the target repository" for `AGENTS.md` and `CLAUDE.md` only; the RFC-064 system prompt |
| RFC-083 | Anthropic requests mark cache breakpoints, both clients report cached input tokens, and cached reads count at a tenth toward the token cap | None |
| RFC-084 | A sandboxed `run_command` runs an allowlisted argv on a throwaway copy of the worktree | RFC-014 "Text emitted during a model turn is not a command"; the RFC-026 consequence "The sandbox runs that argv, not a string parsed from the model", for `run_command` only |
| RFC-085 | Infrastructure failures do not use up an attempt, a bad tool call goes back to the model, and a branch that already holds the change goes to the gate | None |

## RFC-080 File tools that do not destroy files

### Decision

The `implementer` tool list becomes `list_files`, `search_files`, `read_file`, `read_linked_file`, `replace_in_file`, `edit_file`, `delete_file`, `move_file`, `run_local_check`, and `commit`. RFC-082 adds `todo_write` and RFC-084 adds `run_command`. Every path goes through `safePath` (0006), and every walk skips `.git` and `.orchestrator` and never follows a symlink.

- `read_file(path, offset, limit)` and `read_linked_file(project, path, offset, limit)` return lines as `N|text`. `offset` is the first line, from 1. `limit` defaults to 2,000 lines. The result ends with `lines A-B of T`, and with `more: read from line B+1` when lines remain. An offset past the end is an error. The 256 KiB read cap stays.
- `replace_in_file(path, old, new, replaceAll)` replaces the exact text `old`. Without `replaceAll`, `old` must occur exactly once. No match, more than one match, an empty `old`, or `old` equal to `new` is an error that says which. The file is written to a temporary file in the same directory and renamed over the original, keeping its mode.
- `edit_file(path, text)` still writes the whole file, now with the same rename. It refuses text that holds a history placeholder, and refuses empty text over a file that is not empty.
- `search_files(text, regex, path, glob, ignoreCase, context)` finds the exact text, or an RE2 expression when `regex` is true, under `path`, in files whose path matches `glob`. `context` adds up to 5 lines before and after each match as `path-N- text`. The 100-match cap, the 200-byte line cut, and the skips for files over 1 MiB and files with a NUL byte stay.
- `list_files(path, glob, depth)` keeps the 500-path cap. `depth` limits how many directories deep below `path` it lists. Below the start, it skips `node_modules`, `vendor`, and `dist`.
- A glob matches the whole repository-relative path, with `**` for any number of directories. A glob without `/` matches the file name.
- `delete_file(path)` removes one regular file. `move_file(from, to)` renames a regular file to a path that does not exist, making its directory.
- A tool error starts with `error: `. The `anthropic` client sends such a result with `is_error: true`. A tool outside the profile returns `error: not in profile`.
- Every tool result is redacted with the job's secrets before it goes back to the model.
- The history no longer puts a placeholder into an earlier `edit_file` call. It drops `text` from the arguments and sends `bytes` and `lines` instead.
- The `tool_call` event records `path`, `offset`, and `limit` for reads, `path`, `glob`, `regex`, and the search text for searches, `path` and the byte counts of `old` and `new` for `replace_in_file`, and `path` or `from` and `to` for the others. File text is never stored.

### Why

A placeholder written back into a file is the cost of a tool that only replaces whole files and a history that hides what was written. An exact replacement keeps the rest of the file, and a numbered range lets a model read a long file in parts.

## RFC-081 A loop that finishes

### Decision

- Each model may set `historyBudgetKiB` in its `modelOptions` (RFC-079), from 4 to 512. Zero means 16. The implement loop uses the routed model's budget for every request of the attempt.
- The newest turn's results are always sent, each cut at the result cap: 12 KiB, or a quarter of the budget when that is larger, up to 64 KiB. Older results are sent while their total stays under the budget. When an older result has to go, the oldest results are omitted until the total is under three quarters of the budget, and an omitted result stays omitted for the rest of the attempt. So the sent history changes in chunks and its start stays the same between requests.
- A read of the same file and range whose text has not changed, while the earlier result is still sent, returns `unchanged since turn N; the earlier result is above`.
- The output of `run_local_check` and `run_command` keeps its tail when cut, because a failure is at the end.
- A reply with no tool call, or a reply cut at its token limit (`providers.Truncated`), gets a continue note while the attempt has made no commit and turns remain, at most twice per attempt. `providers.Turn` gains `Note`, which the clients send as user text after the turn's results.
- Each `run_local_check` that exits 0 records the hash of the worktree's tree. When the attempt ends with uncommitted changes and the tree hash still matches, the harness commits with the card title as the subject and the trailer `Committed-By: harness after passing local check`.
- The turns-left note starts when `max(3, cap/5)` turns remain, and it goes in the newest turn's `Note`, not in a result.
- The default `maxModelTurnsPerAttempt` is 40, and the router settings take from 1 to 200.
- Before each model call, the card's tokens are added up over its attempts. At or over `maxTokensPerCard`, the attempt stops with outcome `budget`, a `budget_stop` event with `cap: token_cap`, one more `attempts_used`, and the card blocks with `token_cap`.
- When every turn is used, the worktree is not ahead of the attempt's base, and no auto-commit applies, the attempt stops with outcome `budget`, a `budget_stop` event with `cap: turn_cap`, one more `attempts_used`, and the card blocks with `turn_cap`.
- The default `maxTokensPerCard` becomes 2,000,000, because a 40-turn attempt sends its history on every turn. A saved value is kept.

### Why

The model re-read files because their results had left the history, and it stopped because a reply without a tool call ended the attempt. A larger, stable history keeps what it read, and a stable start lets RFC-083's cache hold. A check that passed on the same tree is the same evidence a commit would gate on, so committing it loses nothing. RFC-020 and tech/0003 step 7 already required the turn and token caps.

## RFC-082 The pack, the prompt, and a todo list

### Decision

- After `DiscardChanges`, the loop builds a path map of the worktree: files and directories to at most 4 levels, skipping what `list_files` skips. A directory too deep to list shows as `dir/ (N files)`. The depth drops until the map fits in 12 KiB, and a map that still does not fit is cut with `truncated: true`. It goes inside the untrusted markers.
- `AGENTS.md` and `CLAUDE.md` in the repository root, and in each existing directory a path in the card title or its requirement statements names, go inside the markers as data: 16 KiB each, 32 KiB in total, cut with `truncated: true`. A `repo_context` event records each file's path, bytes, and sha256. They cannot add a tool, name a command, or change a card status, and the system prompt says so.
- Each requirement goes in as `REQ-x heading`, its statement, and its acceptance lines. The decisions of the project whose statement names one of the card's requirement ids go in as `RFC-x: statement`, up to 20.
- `ImplementerSystem` is rewritten on the lines of Claude Code: explore first and batch read-only calls in one reply, read a file before changing it and prefer `replace_in_file`, match the code around the change and add tests, run the local check until it passes and start from the failure note, keep the todo list current, and do not stop before `commit`. It does not contain the words deploy or verdict.
- `todo_write(items)` takes up to 30 items of `{text, status}`, with status `pending`, `in_progress`, or `done` and text up to 200 bytes. It replaces the list and touches no file. The current list goes in the newest turn's `Note` of every request, outside the history budget.
- Read-only calls in one reply (`list_files`, `search_files`, `read_file`, `read_linked_file`) run concurrently. Every other call runs alone and in order. Events and results keep the reply's order.

### Why

REQ-016 already put the path map in the implementer pack. `AGENTS.md` and `CLAUDE.md` are where a repository tells an agent how it builds and tests. Inside the markers they are data like every other repository file (RFC-024). A todo list kept outside the history survives eviction, which is where a long attempt loses its plan.

## RFC-083 Prompt caching

### Decision

- The `anthropic` client marks `cache_control: {type: ephemeral}` on the system block, the last tool, and the last block of the newest turn's results. The newest turn's `Note` comes after that block, so the todo list and the turns-left note do not move the cached prefix.
- `providers.Response` gains `CachedInputTokens`. The `anthropic` client reports `cache_read_input_tokens` there, and adds `cache_creation_input_tokens` to `InputTokens`. The `openai` client reports `prompt_tokens_details.cached_tokens` and subtracts it from `InputTokens`.
- `attempts` gains `tokens_cached`. The `model_call` event records `cachedInputTokens`.
- The token cap counts `input_tokens + output_tokens + tokens_cached / 10`.

### Why

Every turn sends the same system prompt, tools, and earlier turns. A cached read costs about a tenth of a fresh one on Anthropic and on the compatible servers that cache, so counting it in full would stop a card for tokens it barely paid for.

## RFC-084 A sandboxed run_command

### Decision

- `run_command(argv)` takes an array of 1 to 64 strings, at most 4 KiB in total. It is never a shell string.
- `argv[0]` must be a bare name on the project's new `checks.commandExecutables` list, which saves with the same rule as `allowExecutables`. Shells and wrappers (`sh`, `bash`, `zsh`, `dash`, `ksh`, `fish`, `csh`, `tcsh`, `pwsh`, `env`, `xargs`, `busybox`, `timeout`, `nice`, `sudo`, `doas`, `su`, `nohup`, `exec`, `eval`, `command`) are refused on save and at run time. An empty list replies `error: run_command is not enabled for this project` and runs nothing.
- `sandbox.ExecuteCheck` becomes a parse step and `sandbox.ExecuteArgv`, which both tools use. Each call copies the worktree to a fresh directory under `<data>/exports`, asserts that the directory is under `<data>/exports` and is not the worktree, and runs the argv in the pinned image with no network, a 5-minute timeout, and 1 MiB of output. The result is `exit N` and the last 200 lines, redacted.
- The copy is deleted after the call, so file changes are discarded, and the tool description says so. A `run_command` exit is never check evidence: it does not record a passing tree for RFC-081 and does not run the card's check.
- An attempt makes at most 30 calls. In fixture mode, where checks run on the host, the tool replies `error: run_command needs the container sandbox` and runs nothing.
- The project checks settings page has an "Allowed commands" field.

### Why

A model that can only run the card's check cannot run one test file, a type checker, or a formatter to find its own mistake. An argv on a throwaway copy in the same container as the check gives that without a shell: the card's check, its allowlist, and its exit code still decide the gate, and nothing the command does reaches the worktree.

## RFC-085 Fair attempt accounting

### Decision

- A gating check whose engine exits 125 or 127 is an infrastructure failure: a `job_error` event, the card blocks with `sandbox_config`, and `attempts_used` does not change.
- A provider failure after the router's retry and fallback blocks with `provider_error` and does not change `attempts_used`, as before. The attempt ends with outcome `error`.
- A passing attempt counts toward `attempts_used` once its pull request is published. A publish failure blocks with `publish_failed` and leaves the count, and the rerun publishes the same commit.
- A tool call whose arguments are not valid JSON goes back to the model as `error: the arguments were not valid JSON`. `providers.ToolCall` gains `BadArgs`.
- An attempt that made no commit, while the worktree is clean and its HEAD is ahead of the base branch, ends with outcome `no_change` and goes to the gating check on HEAD. `attempts` takes outcome `no_change`.

### Why

An attempt is the model's try at the change. A stopped Podman machine, a provider outage, or a failed push says nothing about the change, and counting them blocked cards that had done the work.

## Set aside

- Streaming.
- Summarising old turns with a second model.
- An explore subagent on the `plan` route.
- CI log tails in the CI failure note.
- Applying `run_command`'s file changes back to the worktree.
- Reading `.cursor/rules` and other agent files besides `AGENTS.md` and `CLAUDE.md`.

## Tests

Sprint 45: `TestReadFileRange`, `TestReplaceInFileUnique`, `TestReplaceInFileNotFound`, `TestEditRefusesPlaceholder`, `TestHistoryDropsEditText`, `TestSearchRegexGlobContext`, `TestListGlobDepth`, `TestDeleteMoveSafePath`, `TestToolErrorsFlagged`, `TestToolResultsRedacted`, and the updated `TestImplementerTools`, `TestToolCallArgs`, and `TestHistoryKeepsNewestResults`, with trace and terminal Vitest cases.

Sprint 46: `TestHistoryBudgetPerModel`, `TestHistoryStableEviction`, `TestRereadUnchanged`, `TestCheckOutputKeepsTail`, `TestTextReplyNudged`, `TestTruncatedReplyNudged`, `TestAutoCommitAfterPassingCheck`, `TestNoAutoCommitOnDirtyTree`, `TestTurnCapStops`, `TestTokenCapStops`, `TestInfraErrorKeepsAttempt`, `TestBadToolJSONReturnedToModel`, `TestNoChangeGoesToGate`, and the `FamilyForm` history budget Vitest case.

Sprint 47: `TestPackPathMap`, `TestPackRepoMemory`, `TestPackAcceptanceLines`, `TestPromptMarkers`, `TestTodoWriteEchoed`, `TestTodoWriteCap`, `TestReadOnlyCallsConcurrent`.

Sprint 48: `TestAnthropicCacheBreakpoints`, `TestCacheUsageRecorded`, `TestTokenCapCountsCachedAtTenth`, `TestHistoryPrefixStable`.

Sprint 49: `TestRunCommandParse`, `TestRunCommandRefusesShell`, `TestRunCommandFlags`, `TestRunCommandOnCopy`, `TestRunCommandDisabled`, `TestRunCommandBudget`, `TestRunCommandCannotPassGate`, `TestRunCommandRedactsOutput`, `TestRunCommandExportAssertion`, `TestSaveDeployCommandExecutables`, and the project settings Vitest case.
