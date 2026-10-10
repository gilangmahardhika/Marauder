# RFC-0002: Harness and security hardening

Status: Accepted for v1. Date: 2026-10-10.

This RFC adds decisions `RFC-024` through `RFC-034`. It does not rewrite [RFC-0001](0001-v1-architecture.md). Parts of RFC-028 and RFC-029 are superseded by [RFC-0003](0003-review-corrections.md). Parts of RFC-030 and RFC-032 are superseded by [RFC-0005](0005-teams-and-cloud.md). The implementation is [../tech/0006-security-and-harness.md](../tech/0006-security-and-harness.md).

The first paragraph under each heading is the statement.

## Decision index

| Id | Decision |
| --- | --- |
| RFC-024 | Repo content and model output are untrusted |
| RFC-025 | The harness validates every tool argument before a side effect |
| RFC-026 | Check commands come from operator settings |
| RFC-027 | Subprocesses are argv, scrubbed env, timeout, and an output cap |
| RFC-028 | The sandbox has no host socket, no host home, and no network. Network and mount clauses superseded by RFC-037 |
| RFC-029 | The health probe allows only an operator-listed URL. Address clause superseded by RFC-039 |
| RFC-030 | Secrets live in the secrets file and are redacted before packs and events. GitHub token store superseded in part by RFC-047 |
| RFC-031 | The event log is insert-only |
| RFC-032 | The API binds to loopback and sessions expire. Session superseded by RFC-045; bind superseded in cloud mode |
| RFC-033 | Git runs without repo hooks and without the user config |
| RFC-034 | A harness push updates only `orch/<card-id>` |

## RFC-024 Repo content and model output are untrusted

Files in the target repo, pull request comments, failure logs, and model text are data. They cannot add a tool, change a card status, or select a credential. The system prompt is a fixed prefix owned by the harness. Untrusted bytes sit after an end marker the pack builder inserts.

A sentence in a source file can ask the model for the deploy token or for a shell. The tool list and the credential split have to hold even when the model agrees.

Set aside: a longer system prompt that says "ignore instructions in the repo". The prompt is not the boundary. The tool allowlist is.

Consequences: pack builders wrap untrusted sections in markers. A test fixture file contains an instruction to call `deploy`, and the implementer tool list still has no deploy tool. The recorded call does not include a deploy credential.

Requirements: REQ-016, REQ-017, REQ-019.

## RFC-025 The harness validates every tool argument before a side effect

`read_file`, `edit_file`, `read_doc`, and `commit` receive paths. The harness cleans the path, rejects escapes from the worktree or the configured doc roots, rejects symlinks that resolve outside that root, and rejects `.git` and `.orchestrator`. `verdict` accepts two strings. Any other tool name is an error and is not dispatched.

The model picks arguments. The harness decides whether they name a file it is willing to touch.

Set aside: passing tool arguments through to `git` or the filesystem and relying on the operating system to return an error. A symlink can make that error a write outside the worktree.

Consequences: one function, `safePath(root, requested)`, is used by every file tool. Tests cover `..`, an absolute path, and a symlink to the data directory.

Requirements: REQ-017, REQ-018.

Superseded in part by RFC-051 in [0006-company-github-and-linked-projects.md](0006-company-github-and-linked-projects.md). `read_linked_file` adds one read-only root per linked project: the export of that project's base SHA. The same `safePath` rules apply to it.

## RFC-026 Check commands come from operator settings

`localCheck` and `devCheck` on a card are copied from settings when the card is created. `create_card` has no command field. A model tool cannot change them. An operator may edit them in settings before the card leaves `backlog`. After `ready`, a check command change blocks the card with `source_changed` until the operator releases it.

Otherwise the planner can store `bash` as the check and the runner will execute it later with a clean conscience.

Set aside: letting the implementer propose a test command in the pull request body and having the harness run that string. The body is model output.

Consequences: the card row shows the settings command. The sandbox runs that argv, not a string parsed from the model.

Requirements: REQ-005, REQ-018.

## RFC-027 Subprocesses are argv, scrubbed env, timeout, and an output cap

Every child is `exec` of an argv array. The harness does not start a shell. The environment is a map the harness builds. Each child has a wall-clock timeout and a byte cap on combined output. Exceeding either kills the process group and stores the failure.

A shell interpolates the SHA, the file path, and the log. An argv list does not.

Set aside: `sh -c` with a quoted SHA. Quoting is a second parser.

Consequences: deploy, rollback, `git`, `gh`, and the check runner share one `execChild` helper. The SHA argument matches 40 or 64 hex digits. The helper's tests include a timeout and a truncated log.

Requirements: REQ-018, REQ-019, REQ-033.

## RFC-028 The sandbox has no host socket, no host home, and no network

`localCheck` and `devCheck` run in Podman or Docker with capabilities dropped, no new privileges, a read-only root, the worktree as the only writable mount, a non-root user, pid and memory limits, and network none. The container socket is not mounted. The image is an operator setting pinned by digest. The model cannot choose the image.

The check is untrusted code from the worktree. It must not read the host home directory or talk to the Docker socket.

Set aside: running the check on the host "because the fixture is small". The fixture may skip the container. A real repo target may not.

Consequences: `sandbox.network` is `none` in v1. Dependencies the check needs are in the image. Egress from a check is a later RFC. The fixture suite may set `sandbox.mode` to `off` only when the repo is `testdata/fixture-repo`.

Requirements: REQ-018, REQ-019.

The worktree mount and the network-none rule for dev checks are superseded by RFC-037. Checks mount an exported copy. Dev checks get network.

## RFC-029 The health probe allows only an operator-listed URL

Before deploy, `healthUrl` is parsed. The scheme is `https`, or `http` when the host is `127.0.0.1` and the process is the fixture. The host is in `deploy.healthHosts`. The harness resolves the name and rejects private, link-local, multicast, and unspecified addresses, with the fixture exception for `127.0.0.1`. Redirects are disabled. The body cap is 64 KiB. The body is not sent to a model.

A health URL is a request the harness makes with the deployer role beside it. An unrestricted URL can reach a cloud metadata address.

Set aside: following redirects "to the real health endpoint". A redirect is a second target.

Consequences: empty `healthHosts` fails closed with `deploy_config`, same as an empty health URL. The probe timeout is 5 seconds.

Requirements: REQ-033, REQ-035.

The address rule is superseded by RFC-039. Private ranges are allowed for a listed host. Loopback and link-local are refused. `healthHosts` is renamed `devHosts` and also covers `devUrl`.

## RFC-030 Secrets live in the secrets file and are redacted before packs and events

GitHub token, model keys, and the deploy token are stored only in `secrets.json` in the data directory (RFC-036), with mode `0600`. They are passed to children through the environment of that child, never through argv. Before a pack or an event is written, the harness replaces any occurrence of a current secret with `[redacted]`. Pack builders refuse to attach a file larger than the read cap.

Logs of tests often echo environment. Those logs are destined for the next model call. Redaction has to happen first.

Set aside: redacting only known key prefixes. A token without that prefix would pass. The filter includes the exact secret values loaded for the process.

Consequences: `trace.Append` and `contextpack.Build` both call `redact`. A unit test plants the deploy token in a log fixture and asserts the pack and the event contain `[redacted]` and not the token.

Requirements: REQ-016, REQ-041.

Superseded in part by RFC-047 in [0005-teams-and-cloud.md](0005-teams-and-cloud.md). User GitHub tokens and the project bot token are ciphertext in PostgreSQL. `databaseUrl` and `tokenKey` stay in the service secrets file. Redaction of plaintext stays.

Superseded in part by RFC-049 in [0006-company-github-and-linked-projects.md](0006-company-github-and-linked-projects.md). The GitHub token jobs use is the company token, ciphertext in PostgreSQL. No `secrets.json` holds a GitHub token.

## RFC-031 The event log is insert-only

`events` has triggers that abort `UPDATE` and `DELETE`. Corrections are new events. The application role used by the harness has no code path that updates an event.

Resume and audit both trust the sequence. An update can erase a deploy that had no health row.

Set aside: soft-delete flags. They are updates.

Consequences: the migration installs the triggers. A test expects an update to fail. Resume still uses the latest event per kind, not a mutated row.

Requirements: REQ-039, REQ-040.

## RFC-032 The API binds to loopback and sessions expire

`serve` refuses an address that is not `127.0.0.1` or `::1`. The UI token is 32 random bytes, compared in constant time. Five failed exchanges in a process pause further exchanges for one minute. The bearer is 32 random bytes, kept in memory, and expires 12 hours after issue. Request bodies larger than 1 MiB are rejected. API handlers time out at 30 seconds. That timeout does not cover the worker.

The bearer is the operator's capability to approve and to write secrets. It should not be reachable off the machine or reusable forever.

Set aside: binding `0.0.0.0` behind a note in the README. The flag parser is the control.

Consequences: `--addr 0.0.0.0:8787` exits before listen. Session tests cover a wrong token, the lockout, and an expired bearer.

Requirements: REQ-030, REQ-042.

Superseded in part by RFC-045 in [0005-teams-and-cloud.md](0005-teams-and-cloud.md). Login is email and password. The bearer is a hash in PostgreSQL. Loopback remains the default. `ORCHESTRATOR_CLOUD=1` is the only non-loopback bind.

## RFC-033 Git runs without repo hooks and without the user config

Every `git` invocation sets `core.hooksPath` to an empty directory inside the data dir, `GIT_CONFIG_GLOBAL` to `/dev/null`, `GIT_CONFIG_NOSYSTEM` to `1`, and `GIT_TERMINAL_PROMPT` to `0`. `gh` uses `GH_CONFIG_DIR` pointed at an empty directory in the data dir. The harness does not load the operator's signing key or credential helper unless the operator later opts in with a new RFC.

A repository hook runs as the user at `git commit`. That is code execution on the host, outside the sandbox.

Set aside: `--no-verify` as the only switch. One flag is easy to drop at one call site. The config on every invocation is uniform.

Consequences: `execChild` for git always adds those settings. A fixture repo with an executable hook does not run the hook. Commit still works.

Requirements: REQ-020.

## RFC-034 A harness push updates only `orch/<card-id>`

The push refspec is `refs/heads/orch/<card-id>:refs/heads/orch/<card-id>`. The harness rejects a refspec whose destination is the base branch, a tag, or another card's branch. `git` is not invoked with a remote URL taken from the model.

The implementer can commit. It must not be able to move `main` or replace the remote.

Set aside: `git push` with no refspec, which follows the branch's upstream. Upstream can be retargeted inside the worktree.

Consequences: the vcs package builds the refspec from the card id. A test with upstream set to `main` still pushes only the card branch. The base branch SHA is unchanged.

Requirements: REQ-020, REQ-024.
