# RFC-0054: Agent selection

Status: Accepted for implementation. Date: 2026-10-14.

This RFC adds decisions `RFC-120`, `RFC-121`, and `RFC-122` for REQ-099. A company admin chooses which agent writes the code of an implement job: the built-in loop, or the Claude Code CLI. The harness keeps everything around the agent: the pack, the budgets, the commit, the gating check, the handoff, and every status change. RFC-122 supersedes one statement of RFC-108 for Claude Code jobs only.

Sprints 94 to 96 in [../plan/sprints.md](../plan/sprints.md) implement this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-120 | `agent.kind` in the service `config.yaml` is `builtin` or `claude_code`. It applies to implement jobs on this server and on runners. Review, deploy, and verify stay on the built-in path. Only an admin changes it | none |
| RFC-121 | With `claude_code`, the attempt runs `claude -p` in the card worktree with the pack on stdin, a tool allowlist built from the project's executables, and a clean environment. Its stream is read into trace events, token counts, and caps. The harness commits the result and runs the gating check as for the built-in loop | none |
| RFC-122 | A Claude Code job on a runner carries the chosen model family's base URL and key. The runner passes them to the `claude` process in its environment only | RFC-046 / RFC-108: "A runner never holds ... a model key", for Claude Code jobs only |

## RFC-120 Company agent setting

### Decision

- `agent.kind` is `builtin` (the default) or `claude_code`. `agent.claudeCode` holds the Claude Code options:
  - `family`: a model family whose protocol is `anthropic` and whose key is saved. Anthropic and compatible APIs such as DeepSeek (`https://api.deepseek.com/anthropic`) both qualify.
  - `model`: one of that family's models. `smallModel` is optional; empty uses `model`.
  - `maxTurns`: from 1 to 200; zero uses the router's `maxModelTurnsPerAttempt`.
  - `auth`: `auto`, `apiKey`, or `bearer`. `auto` sends `ANTHROPIC_API_KEY` when the family has no base URL or its host is `api.anthropic.com`, and `ANTHROPIC_AUTH_TOKEN` otherwise.
  - `allowWeb`: whether `WebFetch` and `WebSearch` are offered. Off by default.
  - `instructions`: at most 8 KiB, appended to the system prompt.
  - `binary`: an absolute path to `claude` on this server; empty looks it up on `PATH`.
- `GET` and `PUT /api/settings/agent` are admin only. A save emits `config_changed`. The key is never part of this setting or its reply; it stays in `secrets.json` under the family.
- A runner reports the agents it can run in its heartbeat. While the agent is `claude_code`, a runner that does not report it gets no lease.

### Why

Teams that already trust Claude Code's editing want it behind the same board, budgets, and checks. Keeping the choice company-wide keeps one set of tool rules to review.

## RFC-121 The Claude Code engine

### Decision

- The attempt starts as a built-in one: card lock, worktree reset, handoff applied, pack built, attempt inserted with `model` set to the Claude Code model.
- The harness runs, in the worktree:

  ```text
  claude -p --output-format stream-json --verbose --model M --max-turns N
    --permission-mode acceptEdits --strict-mcp-config
    --append-system-prompt <implementer system and instructions>
    --allowedTools Read,Edit,MultiEdit,Write,Glob,Grep,LS,TodoWrite,Bash(git status:*),Bash(git diff:*),Bash(git log:*),Bash(<exe>:*)...
    --disallowedTools Bash(git push:*),Bash(git commit:*),Bash(git remote:*),Bash(git config:*),Bash(curl:*),Bash(wget:*)[,WebFetch,WebSearch]
  ```

  The pack goes on stdin. Each `<exe>` is a name from the project's `checks.allowExecutables` or `checks.commandExecutables`.
- The environment is built, not inherited: `PATH`, `HOME` and `CLAUDE_CONFIG_DIR` set to a fresh directory removed after the attempt, `ANTHROPIC_BASE_URL` when the family has one, `ANTHROPIC_MODEL`, `ANTHROPIC_SMALL_FAST_MODEL`, the key variable, `DISABLE_AUTOUPDATER=1`, `DISABLE_TELEMETRY=1`, and `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`. The host's Claude Code login and settings are never read.
- `--resume` and `--continue` are never passed. Each attempt is a new session, so NG-008 holds; the next attempt reads the handoff (RFC-118).
- The stream is read one JSON line at a time, each at most 1 MiB:
  - An assistant `tool_use` is a `tool_call` event with the tool name and clipped, redacted arguments. File contents are kept as sizes only.
  - A `TodoWrite` call replaces the attempt's todo list.
  - An assistant message is a `model_call` event, and its `usage` is added to the attempt once per message id. When the card's tokens reach the token cap, the process is killed and the attempt ends with `budget` and `token_cap`.
  - `result` gives the final text and the turn count. Subtype `error_max_turns` ends the attempt with `budget` and `turn_cap`.
  - A non-zero exit without `result` ends the attempt with `error`, with the last 2 KiB of stderr, redacted.
- After a `result`, when the worktree has changes, the harness commits them with the first line of the final text as the subject, the card trailers, and `Committed-By: harness for claude code`. No change and no new commit ends with `no_commit`. The gating check then runs as for the built-in loop.
- An ending without a commit saves a handoff. Its summary is Claude Code's final text when there is one (source `model`), else the harness summary.
- There is no `ask_user`: a card behaves as if its question mode were `auto`. MCP servers are not offered in this version.
- An `agent_run` event records `agent`, `version`, `model`, `exitCode`, and `turns`.

### Why

The harness's guarantees come from what happens before and after the agent edits files: the pack, the check exit code, the commit, and the board. Swapping only the editing step keeps them.

### Set aside

- Running Claude Code inside the Podman sandbox. It needs network access to the model API, and the sandbox runs with `--network=none`.
- Letting Claude Code commit or push. The harness commits, and the control plane pushes.

## RFC-122 Model key on a runner

### Decision

- The `start` reply of a Claude Code job carries `claudeCode` with the family's base URL, the key, the auth mode, the models, the turn cap, `allowWeb`, and the instructions.
- The runner keeps the key in memory, puts it only in the `claude` process environment, never writes it to disk, and adds it to the redaction list of every event and report.
- The runner finds `claude` at the `claudePath` in its config, or on `PATH`. It reports `claude_code` in its heartbeat only when `claude --version` succeeds.

### Why

Claude Code speaks to the model API itself; it cannot send its calls through the job API's model route. Sending the key per job, as the clone token already is, keeps the runner's disk free of model credentials.

## Tests

- `TestAgentSettingsValidate`.
- `TestClaudeCodeCommits`, `TestClaudeCodeArgsAndEnv`, `TestClaudeCodeTokenCapKills`, `TestClaudeCodeMaxTurns`, `TestClaudeCodeErrorExit`, `TestClaudeCodeHandoffSaved`, `TestClaudeCodeTraceRedactsKey`.
- `TestImplementUsesClaudeCode`, `TestImplementClaudeCodeWithoutKeyBlocks`, `TestClaimNeedsClaudeCodeRunner`, `TestRunnerRunsClaudeCode`.
- `TestAgentSettingsAdminOnly`, `TestAgentSettingsNeverReturnsKey`.
- `go run ./cmd/fixture` runs the `claudeCode` scenario with a fake `claude`.
- Vitest: the Agent page, the runners Agents column and notice, and the `agent_run` words.
