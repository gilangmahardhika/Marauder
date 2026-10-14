# Security and harness hardening

Status: Accepted for implementation. Date: 2026-10-10. Revised after review.

Decisions are in [../rfc/0002-hardening.md](../rfc/0002-hardening.md) and [../rfc/0003-review-corrections.md](../rfc/0003-review-corrections.md). This file is how to build them. The worker shape is in [0003-backend.md](0003-backend.md). Where 0003 is looser, this file wins. Schema and field rules are in [0002-data-and-api.md](0002-data-and-api.md).

## Trust boundary

| Source | Trust | May influence |
| --- | --- | --- |
| Project settings and the service secrets file | Trusted to name commands, hosts, images, and tokens | Deploy argv, dev hosts, check argv, image digest |
| CLI that can read the service data directory | Trusted as break-glass for bootstrap and the fixture | Bootstrap, and a gate only with `--as` |
| Loopback or cloud API after a live bearer | Trusted as that user, and only for the actions their role and membership allow | The gates in RFC-046 |
| Target repo files, diffs, PR comments, CI output, logs | Untrusted | Text inside a pack |
| Model output | Untrusted | Tool name and arguments, which the harness then filters |
| A tribe's MCP server settings | Trusted to name a URL, an image, an argv, a database host and login, and secrets, as saved by a tribe admin (RFC-092, RFC-097) | Which allowlisted tools the implementer and refine offer |
| `@slug` mentions in requirement text, written by members or in the repository | Trusted only to narrow: a mention picks among the tribe's enabled, allowlisted servers and never adds a server, a tool, or a disabled server. A requirement edited since acceptance offers no MCP tools (RFC-101, RFC-102) | Which of the tribe's servers a card's attempt and a draft's refine offer |
| A card's context, set by a person (RFC-105) | Trusted as that person's choice: any role that may run the card adds or removes rule and skill text, which stays inside the untrusted markers and adds no tool. Adding an MCP server is trusted only from a Lead Developer member, a tribe admin of the project's tribe, or an admin, and only for an enabled server of that tribe | Which catalog rules and skills the next attempt reads, and which of the tribe's enabled servers it opens. Never a disabled server, a tool off the allowlist, or a server of another tribe |
| MCP server responses and database rows | Untrusted | Text inside the untrusted markers after `MCP RESULT`, and the tool list only where its schema hash matches the allowlist |
| `ask_user` questions, options, and suggestions (RFC-114) | Untrusted model output, redacted with the attempt's secrets and capped before storage | Text a person reads as plain text on the card page. On an Auto card the suggestion becomes an answer the next attempt reads inside the untrusted markers. Never a status, a route, a tool, or a setting |
| Answers to agent questions, written by a person who may run the card (RFC-114) | Trusted as that person's choice for the card, like instructions | Text the next attempt reads inside the untrusted markers. Never a status beyond `waiting_answer -> ready`, which the answers route makes through `Transition` |
| MCP Registry entries | Untrusted | What the setup form is filled with, which the tribe admin reviews before saving. A registry description is never sent to a model (RFC-096) |
| Featured MCP entries | Trusted as code: a fixed list compiled into the harness and reviewed with it. Registry data never sets `source` to `featured` | What the setup form is filled with. The URL still passes the check on save and at dial, and the admin still allowlists each tool (RFC-099) |
| Check processes, local and dev | Untrusted code | Their own exit code and output, from inside the sandbox |
| Browser push subscriptions (RFC-113) | Untrusted: any signed-in person can post an endpoint and keys | Where an encrypted `{title, body, url, tag}` is posted, and only to an allowlisted push host over HTTPS |
| Browser push services (FCM, Mozilla, Windows, Apple) | Outside the boundary. They see the encrypted payload, the endpoint, and the VAPID public key and contact | Nothing; their reply only deletes or keeps a subscription |
| Fixture mode | Trusted only when the resolved repo ends with `/testdata/fixture-repo` | `sandbox` off, `http://127.0.0.1` for health and dev URL |

The model is outside the boundary. Agreement from the model does not cross it.

## Fail closed

| Condition | Result |
| --- | --- |
| Data directory inside the repo, or the reverse | Process exits before opening the database |
| Tool name not in the profile | Error to the model, no exec |
| `ask_user` after `commit`, past its cap, or with a bad argument | Error to the model, nothing stored, no status change (RFC-114) |
| Path escapes the root, or a symlink does | Tool error, no write |
| Check command fails parsing, or its first word is not allowed | `sandbox_config`, check does not run |
| Card check differs from settings | Card is blocked with `source_changed` |
| `execChild` argv names a shell | Process refuses to start |
| Sandbox image has no digest on a real repo | `sandbox_config`, check does not start |
| Deploy or rollback command fails the path rule | `deploy_config`, no exec |
| `devHosts` empty, or a URL host not listed | `deploy_config`, no probe |
| Resolved health address is loopback, link-local, multicast, or unspecified | Probe fails, rollback runs |
| Secret appears in a pack or event | Write the redacted bytes, or refuse the pack if redaction cannot run |
| `--addr` is not loopback, and `ORCHESTRATOR_CLOUD` is unset | Process exits before listen |
| `Host` header not allowed | `403` |
| Push refspec destination is not the card branch | No network call |
| Card owner missing, disabled, unverified, or without a token | `owner_github`, no push and no pull request call. The company token is never used instead (RFC-054) |
| Approver is the card owner | `403`, no approval row |
| Rule or skill text asks for a tool, a command, or a status | Nothing changes. The text sits inside the untrusted markers, and the tool list is the profile constant (RFC-055) |
| A requirement draft or a refined text asks to accept the requirement, change a status, or run something | Nothing changes. The draft and guidance sit inside the markers, the refine request has no tools, and the refine route writes no requirement, acceptance, or card. The text reaches the record only when a member applies it and saves, and that save clears acceptance (RFC-086, RFC-087) |
| A model key appears in streamed refine text, split across pieces | The redact buffer holds back the bytes that could start a key until the next piece decides, so the stream carries `[redacted]` and never part of the key (RFC-087) |
| A secret appears in a streamed agent reply, split across pieces | The loops redact live text with every job secret, the same list the stored events use, through `trace.NewStreamRedactor`, so a trace stream carries `[redacted]` and never part of a secret. Reasoning and tool arguments are never streamed (RFC-088) |
| Someone outside the project watches a card's live reply | The trace stream has the trace route's checks: a bearer in the header, membership of the project's tribe, and a card in that project. The bearer never goes in the URL. A stream ends after 10 minutes, so membership is checked again on reconnect (RFC-088) |
| A member who is not an admin reads token usage | `GET /api/usage` is `403` before the dates are read. The response has counts, ids, names, and emails only: no reply text, prompt, key, or `databaseUrl`. The CLI twin is for the operator, who can already read the board (RFC-089) |
| A tribe admin or a member reads usage beyond their scope | `/api/tribes/{id}/usage` is `403` unless the caller is an admin or a tribe admin of that tribe, and reads only that tribe's projects. `/api/me/usage` takes no user parameter: it reads the caller's own calls only. Neither returns reply text or a secret (RFC-090) |
| An MCP URL is not https, or names or resolves to a loopback, private, link-local, CGNAT, metadata, unspecified, multicast, or IPv4-in-IPv6 address | `422` on save; at connect the guarded client refuses the dial, checked again on the socket, and the tool is not offered (RFC-093) |
| An MCP server redirects, or a proxy variable is set | The client does not follow the redirect and ignores proxy variables |
| An MCP server changes a tool's schema after it was allowlisted | The tool is left out of the attempt and `assertion_failed` `mcp_changed` is written. The card is not blocked |
| An MCP result asks for a tool, a command, a commit, or a status | Nothing changes. The result sits inside the markers with marker lines dropped, the tool list was frozen at the start, and a result is never evidence (RFC-094) |
| An MCP server sends a sampling, elicitation, or roots request | It gets method-not-found; the client declares no capabilities |
| An MCP reply is over 256 KiB, or a stdio line over 1 MiB | The call fails with a fixed reason; the stdio server is closed |
| An MCP error carries server text | The model, events, and API get only a fixed reason such as `the server is unreachable` |
| A stdio MCP server in fixture mode, or with no engine | Not started; `mcp_context` records `stdio servers need the container sandbox` |
| A card's requirements mention servers, but none is an enabled server of the tribe, or a requirement's text changed since acceptance | No MCP server is opened and no MCP tool is offered. `mcp_context` records scope `none`, the `missing` slugs, and the reason (RFC-102) |
| A card attaches a server, but a requirement of the card changed since acceptance | No MCP server is opened. The acceptance check runs before the card step (RFC-105) |
| A card attaches a server that is disabled or belongs to another tribe, or a Developer attaches any server | The change is refused, and no row is written (RFC-105) |
| A requirement's mention would notify a GitHub user or team in the pull request body | The body wraps each mention in backticks (RFC-101) |
| A member who is not a tribe admin reads the intake | The intake lists the tribe's servers by slug, enabled, and tool count only; the URL, image, argv, connection, and secret names stay on the admin routes (RFC-101) |
| A stdio MCP argv names a shell or wrapper, or the image has no digest | `422` on save, and `sandbox.StdioArgv` refuses again before start |
| A database connector names the board database, by any name that resolves to its address, with the same port and database name in any case; when the board is on this machine, any loopback or local interface address | The guarded dialer refuses before the socket opens, and the tool is not offered (RFC-097). The host check on save does not apply `netguard`: private and loopback hosts are allowed on purpose |
| A database tool call tries to write, run two statements, or leave the transaction | Postgres: the session and the transaction are read-only, the extended protocol runs one statement, and every call ends with `ROLLBACK`. SQL Server: `CheckSelect` refuses anything but one `SELECT` or `WITH`, and the call runs in a transaction that always rolls back |
| A SQL Server login that can write | SQL Server has no read-only transaction, so a function or a statement the check does not know could still write. The admin must confirm a read-only login (for example `db_datareader` only) before saving, and the check is a second guard, not the first |
| A database password, or a database error that echoes it | The password is a write-only `PASSWORD` secret sealed under `tokenKey`, never in `config_json`, joins the job's redaction list, and is set after the DSN is parsed so no `.pgpass` or service file is read. An error is cut at 300 bytes and redacted |
| A database result is large or slow | At most 200 rows, each cell cut at 1 KiB, then the 64 KiB history cap; a 5-second connect and a 15-second statement timeout |
| The MCP Registry is down, slow, or returns a large or bad body | The featured and built-in entries come from `source=local`, which never calls the registry. The registry page has the fixed notice. The harness reads through a copy of the netguard transport, at most 512 KiB in 60 seconds, at most 4 fetches at once, and remembers a failure for 30 seconds; the browser never calls the registry (RFC-096, RFC-100) |
| A person types quickly while the registry is slow | Each new query aborts the previous request, the abort cancels the registry fetch, and an abandoned fetch is neither cached nor remembered as a failure, so it frees its slot (RFC-100) |
| A featured server offers write tools, or its URL moves | GitHub and Linear use their read-only endpoints, and only tools the admin allowlists are offered. A header value is a tribe-shared credential, never a card owner's token (RFC-054). A redirect is refused at dial, and `TestMCPCatalogFeatured` refuses a URL that fails the check (RFC-099) |
| A pull is asked for a reference that is not an image name, in fixture mode, or with no engine | `422` on `image`, and no engine runs. A failed pull is the fixed `the engine could not pull the image`, never the engine's output. Jobs still run the pinned digest with `--pull=never` (RFC-098) |
| Many viewers or a slow viewer hold up the agent | Publishing never blocks: a viewer that falls 256 frames behind is dropped with `resync`, and the server allows 16 viewers per card and 64 in all (RFC-088) |
| Rules and skills exceed the 64 KiB section cap | The blocks that do not fit become `omitted` notes. The pack never exceeds its cap |
| `create_card` names a skill the project has not enabled | No card is stored (RFC-056) |
| Stack detection meets a symlink or a file over 1 MiB | The link is not followed, and the file is read only to 1 MiB |
| Event `UPDATE` or `DELETE` | Postgres trigger aborts the statement |
| Deploy SHA is not 40 or 64 hex digits | `deploy_config`, no exec |
| Transition status changed after read | `409`, no side effect |
| Worker lock held by another process | `serve` exits, `run-next` only queues |

## Data directory

Startup resolves both paths with `filepath.EvalSymlinks` and compares them with a separator-aware prefix check. Equal paths count as containment. The directory is created `0700`. An existing directory with group or world bits exits with a message naming `chmod 700`.

`config.yaml` and `secrets.json` are `0600`. Both are written by temp file, `fsync`, and rename in the same directory.

## Untrusted packs

`contextpack.Build` returns one system string and one user string.

The system string is a constant in the binary, one per profile. Its SHA-256 is stored on the attempt as `system_hash`. A test fails if the constant names a tool that the profile does not have.

The user string is sections in this order:

```text
CARD
<card fields from the board>

UNTRUSTED START
<company rules and the card's skills>
<requirements, diff, files, log tail, comments>
UNTRUSTED END
```

Company rules and skills are written by an admin, not by the target repo, and they still sit inside the markers (RFC-055). They are guidance for the code the model writes. They cannot widen the tool list, name a check command, or move a card. The system string says so in one fixed sentence. Each rule or skill block is capped at 16 KiB, and the blocks together at 64 KiB, inside the 256 KiB untrusted cap.

The system string tells the model that text between the markers is data. One exception is named in the system string (RFC-073): the block from `INSTRUCTIONS FROM THE TEAM` to `END OF INSTRUCTIONS`, written after the failure note inside the markers, comes from a person on the team, and the model follows it where it fits the requirements. Like the rules, it cannot add a tool, name a command, or change a card status. Only the implementer builder writes that block, and only when the card has instructions. Builders strip any copy of the marker lines, and any line equal to either instructions line ignoring case and outer spaces, from untrusted text before wrapping it. They refuse a section over the caps below. They run `redact` on the user string before return. They do not append model text from an older attempt.

The provider client drops any tool call whose name is not in the profile snapshot for that request. A dropped call is a `tool_call` event with `rejected: true` and no dispatch.

## Path rule

```text
func safePath(root, requested string) (string, error)
```

1. Reject an empty path, a NUL byte, and an absolute path.
2. Clean with `path/filepath.Clean`.
3. Join under `root`. The result must have `root` as a prefix after both are cleaned.
4. Resolve symlinks on the deepest existing ancestor. The resolved path must still have the resolved `root` as a prefix.
5. Reject a final path whose relative form is `.git` or `.orchestrator`, or starts with `.git/` or `.orchestrator/`.
6. `read_doc` uses the configured PRD and RFC directories as its only roots.
7. `read_linked_file` uses the export of one linked project's base SHA as its only root (RFC-051). The model names the linked project by id, and an id that is not a link of the card's project is an error. Shared paths saved on a link pass steps 1, 2, and 5 when they are saved, and the full check when they are read.

`edit_file` and `replace_in_file` refuse a target that exists and is not a regular file, refuse writes over 1 MiB, refuse a history placeholder, and write through a temporary file and a rename. `edit_file` refuses empty text over a non-empty file. `delete_file` and `move_file` take regular files only, and `move_file` does not overwrite. `read_file` opens files up to 8 MiB and returns at most 256 KiB per call, with the next line to read. Every tool result is redacted with the job's secrets before it goes back to the model (RFC-080). The data directory is outside the repo, so it is outside every root.

## Checks

### Source

On card insert, copy `checks.local` and `checks.dev` from settings onto the card. `create_card` has no command field. A command field in the payload is ignored and logged as a rejected `tool_call`.

Changing `checks.local` or `checks.dev` in settings updates cards in `backlog` and blocks cards past `backlog`, except `done`, with `source_changed`.

### Parsing

A check string is split on runs of ASCII spaces. Refuse the string when it contains a newline, a tab, or any of these characters:

```text
; & | $ ` < > ( ) { } [ ] * ? ! ~ " ' \
```

The first word must be in `checks.allowExecutables`. It is a bare name resolved inside the image, not a host path. The result is the argv passed after the image in the container command. An empty string means no check: health-only for `devCheck`, and `sandbox_config` for `localCheck` on a non-manual card.

The implementer's `run_command` (RFC-084) takes an argv array, never a string, so there is no parse step and no shell syntax to refuse. `argv[0]` must be a bare name in `checks.commandExecutables`. That list refuses shells and programs that run another program (`sh`, `bash`, `zsh`, `dash`, `ksh`, `fish`, `csh`, `tcsh`, `pwsh`, `env`, `xargs`, `busybox`, `timeout`, `nice`, `sudo`, `doas`, `su`, `nohup`, `exec`, `eval`, `command`) on save, and the tool refuses them again at run time. The argv runs in the same container as the check, on a throwaway copy, and its exit is never check evidence.

### Exports

The sandbox never mounts the worktree (RFC-037).

| Check | Export |
| --- | --- |
| `run_command` tool | A fresh copy of the working tree per call, made like the `run_local_check` copy in a new directory under `<data>/exports`, deleted after the call (RFC-084) |
| `run_local_check` tool | Copy of the working tree. Skip `.git`. Copy regular files and directories only. Do not follow symlinks; recreate them as symlinks only when the target stays inside the tree, otherwise skip them |
| Gating local check | `git archive --output=<export>.tar <commit_sha>`, read by an in-process tar reader that refuses absolute names, `..`, and links outside the export. The `.tar` file is deleted after the read (RFC-065) |
| Dev check | Same as gating, for `deployed_sha` |

Exports live under `<data>/exports/<card-id>-<sha-or-attempt>/` and are deleted when the check ends. `Reconcile` deletes leftovers.

## execChild

```text
type Child struct {
    Argv    []string      // Argv[0] is an absolute path
    Env     []string      // full environment, not merged with os.Environ
    Dir     string
    Timeout time.Duration
    MaxOut  int
}
```

`Argv[0]` must be absolute and must be one of: the `git`, `gh`, or sandbox engine path resolved at startup, the deploy or rollback command from the settings snapshot, or the orchestrator binary for askpass. A basename of `sh`, `bash`, `zsh`, `dash`, `fish`, `cmd`, or `powershell` is refused. In fixture mode with the sandbox off, the check's first word is resolved once against `/usr/local/bin:/usr/bin:/bin`.

Do not pass `os.Environ()` into `Env`. Build it.

| Child | Env | Timeout | MaxOut |
| --- | --- | --- | --- |
| `git` | `PATH` with only the binary's directory, `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`, `GIT_TERMINAL_PROMPT=0`, `HOME=<data>/empty-gh`. For network calls also `GIT_ASKPASS`, `ORCH_ASKPASS=1`, `GH_TOKEN` | 2 min | 1 MiB |
| `gh` | `PATH` as above, `GH_TOKEN`, `GH_HOST`, `GH_CONFIG_DIR=<data>/empty-gh`, `GH_PROMPT_DISABLED=1`, `NO_COLOR=1` | 2 min | 1 MiB |
| Sandbox engine | `PATH` with only the engine's directory, `HOME` and `XDG_RUNTIME_DIR` from the operator session so rootless Podman works | 10 min | 1 MiB |
| Inside the container, local check | `PATH=/usr/local/bin:/usr/bin:/bin`, `HOME=/tmp`, `CI=1` | | |
| Inside the container, dev check | Same, plus `DEV_URL` and `ORCH_SHA` | | |
| Deploy and rollback | `PATH=/usr/bin:/bin`, `DEPLOY_TOKEN`, `DEV_URL`, `HOME` an empty dir under the data directory | 10 min | 1 MiB |

Container env is passed with `--env NAME=value` flags. Tokens never appear in container env. The one exception is a stdio MCP server's own environment secrets (RFC-093): the argv names them as `--env NAME`, the value sits only in the engine child's environment, and no harness, GitHub, deploy, or model secret is added. On timeout or oversize, kill the process group (`Setpgid` at start, negative pid on kill). Store exit `124` for timeout and `125` for oversize. Those are failed checks, not passes.

## Sandbox

Real repo targets use this Podman shape. Docker uses the same flags, minus `relabel`.

```text
podman run --rm
  --network=none                      # local checks
  --cap-drop=ALL
  --security-opt=no-new-privileges
  --pids-limit=256
  --memory=2g
  --cpus=2
  --read-only
  --tmpfs /tmp:rw,nosuid,size=256m
  --mount type=bind,src=<export>,dst=/work,relabel=private
  --workdir /work
  --user 65532:65532
  --env CI=1
  <image>@sha256:<digest>
  <check argv>
```

Dev checks replace `--network=none` with the engine's default rootless network and add `--env DEV_URL=<devUrl> --env ORCH_SHA=<sha>`. They run only for a SHA a person approved and the harness deployed. Egress is not restricted by host in v1 (RFC-037).

`sandbox.image` must contain `@sha256:` and 64 hex chars. The harness passes `--pull=never` once the image is present, and runs `podman pull <image>` only from the settings test. It never mounts the home directory, the data directory, the worktree, or the container socket.

The sandbox off mode is accepted only in fixture mode.

What runs today (RFC-063): `sandbox.ExecuteCheck` builds this argv for `run_local_check` and the gating check, always with `--pull=never`, and runs it with the engine environment in the table above, a 10 minute timeout, and a 1 MiB cap. The engine must be an absolute `podman` or `docker` path resolved at startup. Anything else, or an image without a digest, runs nothing and returns an error the job maps to `sandbox_config`. Pull the image by hand before the first run, because there is no settings test yet.

## Deploy commands

Checked at settings save and again inside the deploy lock before exec (RFC-038):

1. Absolute path. `filepath.EvalSymlinks` result is outside the resolved repo and data directory.
2. `Lstat` after resolving: a regular file, owned by the current uid, mode has an execute bit, and no group or world write bit.
3. Its parent directory has no group or world write bit.

Argv is `[command, sha]` for deploy and `[command, previousSha]` or `[command]` for rollback. `sha` matches `^[0-9a-f]{40}$` or `^[0-9a-f]{64}$`.

## Health probe

```text
func probe(raw string, hosts []string, fixture bool) error
```

1. Parse the URL. Reject userinfo and a scheme other than `https`. In fixture mode also allow `http` with host `127.0.0.1`.
2. The lowercased host must equal an entry in `deploy.devHosts`.
3. Resolve once. Reject loopback, link-local (`169.254.0.0/16`, `fe80::/10`), multicast, and unspecified addresses, including IPv4-mapped IPv6 forms. Private ranges are allowed for a listed host. Fixture mode allows `127.0.0.1` only.
4. Dial the resolved address with a custom `DialContext`, so DNS cannot change between the check and the request. TLS `ServerName` is the host, and the certificate is verified.
5. 5 second timeout. Redirects are refused (`CheckRedirect` returns an error).
6. Read at most 64 KiB. Success is HTTP 200. Store the status and byte count, not the body. The body never goes to a model.

`devUrl` passes steps 1 to 3 at save time. The dev check container reaches it on its own network, outside this function.

## Secrets

Load secrets once per job into that project's snapshot, never into a global that HTTP handlers can format. The list is `databaseUrl`, `tokenKey`, the caller's GitHub token if it was decrypted, the card owner's GitHub token when the job decrypted it (RFC-054), the company GitHub token, each model key, and the deploy token. Password hashes are not redacted by substring search. They are never written to a pack, an event, or a log. `redact(s string, secrets []string) string` replaces each non-empty secret with `[redacted]`. Apply it to:

- event payloads, and before that each `model_call.text` and `tool_call.args` value, which `trace.Clip` redacts before it cuts (RFC-074). JSON escapes characters such as `&`, so redacting the marshalled payload alone can miss a secret that holds one
- pack user strings
- tool results before they re-enter the model
- HTTP error messages
- child output before it reaches the log file
- failure notes before they are stored, since the card page returns them to every project member

The implement and review jobs get `databaseUrl` and `tokenKey` through `JobDeps.Secrets`, which `serve` and `run-next` fill from `secrets.json`, with `vapidPrivateKey` when set. A job started where the file cannot be read still redacts its model keys and GitHub tokens.

`vapidPrivateKey` signs every push request, so whoever holds it can send this site's subscribers notifications through their push service. It lives only in `secrets.json` (mode `0600`); `GET /api/push/key` derives and returns the public key, and no route, event, or log carries the private one. Losing it means every browser must turn notifications off and on again; deleting it from the file makes `serve` generate a new one, which invalidates every subscription the same way.

A secret shorter than 8 characters is refused at save, and a job refuses to start if one is loaded. Short values make redaction collide with ordinary text.

Children receive a secret only in `Env`, never in `Argv`.

## Event log

The insert-only triggers are part of `00001_init.sql` (0002). `trace.Append` is the only writer. Resume reads. Nothing repairs history by editing a row.

## Locks and transitions

Card lock acquire:

```sql
INSERT INTO locks (card_id, holder, acquired_at)
VALUES ($1, $2, $3)
ON CONFLICT (card_id) DO NOTHING;
```

Proceed only when `RowsAffected` is `1`. `holder` is a new random id for that job. Release:

```sql
DELETE FROM locks WHERE card_id = $1 AND holder = $2;
```

A release that matches zero rows appends `assertion_failed`. It does not delete someone else's lock. The deploy lock:

```sql
UPDATE deploy_lock
SET card_id = $1, holder = $2, acquired_at = $3
WHERE project_id = $4 AND card_id IS NULL;
```

Release sets the three columns to null `WHERE project_id = $4 AND holder = $5`. Another project's row is not updated.

Transitions are compare-and-swap on `id` and `status`, and the `WHERE` also includes `project_id` (0003). Only the holder of that project's worker lock runs `Reconcile` for that project, which is the only code that clears locks it does not hold (RFC-048).

## API session

| Control | Value |
| --- | --- |
| Listen | `127.0.0.1` or `::1` unless `ORCHESTRATOR_CLOUD=1` |
| Cloud bind | Configured address. TLS terminates in front of the process |
| `Host` header | Loopback names, or the configured allowlist in cloud mode, else `403` |
| Password | argon2id hash. Minimum 12 characters. Never logged |
| Compare | `subtle.ConstantTimeCompare` on bearer SHA-256, and a password verifier that does not branch on secret length in the HTTP response |
| Failures | 5 bad passwords for one email, then `429` for 1 minute |
| Bearer | 32 bytes from `crypto/rand`. The row stores SHA-256. 12 hour absolute expiry, checked on every request |
| GitHub tokens | AES-256-GCM with `tokenKey` from the service secrets file. GET returns `tokenSet` only |
| Body cap | 1 MiB, then `413` |
| Header timeout | 30 seconds on the HTTP server |
| CORS | Exact configured origin, no `*`. A mismatched `Origin` is `403` |
| Hidden project | `404` for a non-member and for an unknown project id |

The worker uses a context that is not the HTTP request context, so the 30 second cap does not kill a check.

## Git

Every git argv starts with:

```text
git -c core.hooksPath=<data>/empty-hooks
    -c core.fsmonitor=false
    -c protocol.file.allow=never
    -c credential.helper=
    -c commit.gpgSign=false
```

Calls on a worktree add `--git-dir=<worktree_gitdir> --work-tree=<worktree_path>` from the card row. The harness never lets git discover the admin directory by reading the worktree's `.git` file after creation. `<data>/empty-hooks` is an empty directory created at startup.

Network calls (`clone`, `fetch`, `push`) use an explicit URL built from settings: `https://<host>/<org>/<repo>.git`, where `host` and `org` come from the company GitHub account and `repo` from the project (RFC-049, RFC-050). `owner` and `repo` match `^[A-Za-z0-9._-]+$`. Remotes configured in the repo are not used. A remote a model added cannot redirect a push.

Authentication uses askpass. `GIT_ASKPASS` is the absolute path of the orchestrator binary. With `ORCH_ASKPASS=1`, the binary prints `x-access-token` for a prompt starting with `Username` and the value of `GH_TOKEN` for a prompt starting with `Password`, then exits. `credential.helper=` empties the helper list so the operator's keychain is not consulted.

Push:

```text
git push <url> refs/heads/orch/<card-id>:refs/heads/orch/<card-id>
```

The card id matches `^card_[0-9a-f]{12}$`. Build the refspec in code. Do not format it from model text.

Before and after each implement job, read local `refs/heads/<baseBranch>`. A change appends `assertion_failed` and blocks with `base_moved`. Remote base movement is normal and is not checked.

## UI

The SvelteKit build is static. It has no server code (0004).

| Control | Rule |
| --- | --- |
| CSP | `kit.csp` in hash mode: `default-src 'self'`, `script-src 'self'`, `style-src 'self'`, `img-src 'self'`, `connect-src 'self' http://127.0.0.1:8787`, `object-src 'none'`, `base-uri 'self'`, `form-action 'self'`, `frame-ancestors 'none'` where the server can send headers |
| Markup | No `{@html}`. Model text, logs, and PR bodies render as text. A requirement statement, its acceptance lines, and a card note with source `review` or `approver` are parsed by `$lib/markdown` into a token tree rendered as elements and text nodes, with links only to `https://github.com/` or `http://127.0.0.1` with an optional numeric port, so a userinfo trick like `http://127.0.0.1:1@host` stays text |
| Bearer | `sessionStorage` key `orchestrator.bearer`. Never `localStorage`, cookies, or URLs |
| Secrets | Secret inputs are `PasswordInput`, start masked, use `autocomplete="off"`, and are cleared after save. The eye toggle reveals only typed text and masks again when the value is cleared |
| Dependencies | `package-lock.json` committed. Install with `npm ci`. Versions pinned exactly. `npm audit --omit=dev` in the release order |

## Harness assertions

These run in the worker, not only in tests. A failed assertion appends `assertion_failed` with the name and blocks the card with `assertion`, except where the table names another reason.

| Assertion | When |
| --- | --- |
| Profile snapshot hash matches the binary's profile | Before the provider call |
| System prompt hash matches the constant | Before the provider call |
| No secret substring in the user pack | After `redact` ran |
| Attempt has no parent transcript bytes | At attempt start |
| `approved_sha` equals the refreshed head before deploy exec | Inside the deploy lock |
| Deploy event exists before the deploy process starts | Same function, in that order |
| Health event exists for every deploy | `Reconcile` on the next start |
| Card lock holder equals this job | Before release |
| Local base ref unchanged | End of implement job. Reason `base_moved` |
| Export path is under `<data>/exports` and is not the worktree | Before the container starts |
| Each allowlisted MCP tool's live schema hash equals its stored `schemaSha256` | When the attempt or refine opens the tribe's servers that its mentions keep (RFC-102). `mcp_changed` with the server and tool; the tool is left out and the card is not blocked (RFC-094) |

`Reconcile` stays free of model calls. It may probe health under RFC-039.

## Limits

| Limit | Value |
| --- | --- |
| Read returned to the model | 256 KiB |
| Write from `edit_file` | 1 MiB |
| Log kept for a pack | Last 200 lines, after redaction, max 64 KiB |
| Child output | 1 MiB |
| `job_error.message` | 8 KiB, head and tail kept |
| `model_call.text` | 4 KiB, head and tail kept (RFC-074) |
| Each `tool_call.args` string | 1 KiB, head and tail kept (RFC-074) |
| Trace stream | While `live` and the tab is visible: database read every 1 s and after each model call, ping every 15 s, 10 min per connection, 16 viewers per card and 64 in all, 256 frames buffered per viewer, 64 KiB of live text per turn (RFC-088) |
| Trace poll | Only while the stream is down: every 3 s, every 1 s in the terminal view; `since` reads 10 s back; the stream is tried again after 5 s |
| Model reply | 8,192 tokens, unless the request sets its own |
| Reviewer reply | 32,768 tokens; a reply cut at the limit is asked again once (RFC-075) |
| Check timeout | 10 min |
| Git and `gh` timeout | 2 min |
| Health timeout | 5 s |
| Health body | 64 KiB |
| API body | 1 MiB |
| Bearer life | 12 h |
| Session failures | 5, then 1 min pause |
| Secret minimum length | 8 |
| `implementationCap` | 1 to 8 |
| `maxAttempts` | 1 to 10 |
| MCP servers per tribe, tools per server, secrets per server | 8, 32, 8 |
| MCP tool schema and description | 8 KiB and 5 levels; 512 bytes |
| MCP calls | 30 per attempt and 8 per reply; 3 per refine over at most 3 turns |
| MCP call | 60 s, 4 KiB of arguments; a 30 s handshake and tool list per server |
| MCP reply and stdio line | 256 KiB; 1 MiB per stdio line |
| MCP result kept in the history | 64 KiB; 4 KiB in the `tool_call` event |
| MCP HTTP connect and headers | 5 s to dial and finish TLS, 15 s for response headers |

Router budgets still apply on top of this table.

## Tests to add

These sit in the suite in [0005-verification.md](0005-verification.md).

| Test | Passes when |
| --- | --- |
| `TestDataDirInsideRepo` | `--data <repo>/.orchestrator` exits non-zero before the database opens |
| `TestToolNotInProfile` | A scripted `deploy` tool call does not exec and writes `rejected: true` |
| `TestPathEscape` | `../x`, an absolute path, `.git/config`, and a symlink out of the worktree all error |
| `TestCreateCardIgnoresCommand` | A `command` field on `create_card` leaves `local_check` equal to settings |
| `TestCheckParse` | `go test ./...` parses. `go test; id` and `$(id)` are refused. `bash -c x` is refused by the allowlist |
| `TestNoShell` | `execChild` refuses an argv whose basename is `sh` or `bash`. `run_command` never reaches `execChild` with a shell: `argv[0]` must be on `commandExecutables`, which refuses shells and wrappers on save and at run time, and the child is the container engine with the argv after the image (RFC-084) |
| `TestRunCommandRefusesShell` | A shell or wrapper (`sh`, `bash`, `zsh`, `dash`, `ksh`, `fish`, `csh`, `tcsh`, `pwsh`, `env`, `xargs`, `busybox`, `timeout`, `nice`, `sudo`, `doas`, `su`, `nohup`, `exec`, `eval`, `command`, in any case) is refused at run time even when the config lists it, and nothing runs |
| `TestRunCommandRedactsOutput` | The reply is the last 200 lines of output with secrets redacted |
| `TestRunCommandExportAssertion` | `CheckExport` refuses the exports root, a path outside it, the worktree, a symlink to the worktree, and a directory inside or around the worktree, and `run_command` runs nothing when it fails |
| `TestSHAArg` | `dev;id` is refused before exec. A 40 hex SHA is accepted |
| `TestDeployCommandPath` | A command inside the repo, inside the data directory, or group writable is refused at save and at deploy |
| `TestSandboxFlags` | The local check argv has `--network=none`, `--cap-drop=ALL`, the digest, and an export mount, and has no socket or worktree path |
| `TestDevCheckNetwork` | The dev check argv has no `--network=none` and has `DEV_URL` |
| `TestCheckOnExport` | A check that writes `/work/.git` leaves the worktree's `.git` file unchanged |
| `TestTarEscape` | An archive entry named `../x` is refused |
| `TestSandboxOffRejected` | Sandbox off outside the fixture does not run the check |
| `TestHealthLinkLocal` | A host resolving to `169.254.169.254` fails the probe |
| `TestHealthLoopback` | A listed host resolving to `127.0.0.1` fails outside fixture mode |
| `TestHealthPrivateListed` | A listed host resolving to `10.0.0.5` is probed |
| `TestHealthUnlisted` | A host not in `devHosts` is `deploy_config` |
| `TestHealthRedirect` | A 302 is a failure and the second URL is not requested |
| `TestRedactPack` | The deploy token planted in a log is `[redacted]` in the pack and the event |
| `TestTraceClip` | A secret holding `&`, and one across the cut, are `[redacted]` before `trace.Clip` cuts (RFC-074) |
| `TestModelCallText` | The `databaseUrl` in a model reply is `[redacted]` in `model_call.text` |
| `TestToolCallArgs` | `edit_file` stores sizes and never the file text, and a secret in a commit message is `[redacted]` |
| `TestReviewerPackHasNoAuthorText` | The implementer's stored reply never reaches the reviewer prompt |
| `TestShortSecret` | A 4 character token PUT returns `422` |
| `TestEventsImmutable` | `UPDATE events` and `DELETE events` fail |
| `TestLockHolder` | A release with the wrong holder leaves the row and appends `assertion_failed` |
| `TestLoopbackOnly` | Without `ORCHESTRATOR_CLOUD`, `--addr 0.0.0.0:8787` exits non-zero |
| `TestCloudBind` | With `ORCHESTRATOR_CLOUD=1`, a non-loopback `--addr` listens, and a `Host` outside the allowlist is `403` |
| `TestHostHeader` | `Host: evil.example:8787` is `403` when that name is not allowed |
| `TestPasswordLogin` | A correct email and password returns a bearer that loads that user |
| `TestWrongPassword` | A wrong password is `401` and creates no session |
| `TestSessionLockout` | The 6th bad password for one email inside the pause window is `429`. A different email is not paused |
| `TestBearerExpiry` | A bearer older than 12 hours is `401` |
| `TestLastAdmin` | Disabling or removing the admin flag from the last admin is `422` |
| `TestUserGitHubOmitted` | `GET /api/me/github` has `tokenSet` and no token |
| `TestNonAdminForbidden` | A non-admin `POST /api/users` is `403` |
| `TestUserNameRequired` | `POST /api/users` without a name, with only spaces, over 80 characters, or with a control character is `422` on `name`. A trimmed name is stored and listed (RFC-058) |
| `TestAdminRenamesUser` | An admin renames a user with `PUT /api/users/{id}`. An empty name is `422`. A save without `name` keeps it. A non-admin is `403` |
| `TestOwnName` | `PUT /api/me` sets the caller's name and `GET /api/me` returns it. An empty name is `422`. Another user's name is unchanged |
| `TestMemberSeesProject` | A member lists the project and its cards |
| `TestOutsiderMissesProject` | A non-member receives `404` for the project and for a card |
| `TestReqIdsPerProject` | Two projects can both store `REQ-001` |
| `TestBranchPerProject` | Two projects can both store the same branch name |
| `TestAdminManagesMembership` | An admin adds a member. A non-admin cannot |
| `TestParallelProjects` | Two projects can implement at the same time |
| `TestLeadDeploy` | A Lead Developer member moves `approved` to `deploying_dev` |
| `TestOwnerApproveStops` | A Product Owner approval leaves the card in `approved` |
| `TestAcceptSeniorRoles` | Lead Developer, Senior Developer, QA Lead, and Product Owner members can accept. Developer and QA members get `403` (RFC-053) |
| `TestDeveloperRunOnly` | A Developer member can start a run and cannot approve or deploy |
| `TestCardRunRole` | A Product Owner member sees no `run` action, is `403` on a card run, and the board has `canRun: false`. Lead Developer, Senior Developer, Developer, and QA members may run (RFC-061) |
| `TestActorOnTransition` | A new transition event stores `actorId` and does not change older rows |
| `TestCompanyGitHubTokenNeverReturned` | `GET` and `PUT /api/settings/github` return `tokenSet` and no token, and the row holds ciphertext |
| `TestCompanyGitHubAdminOnly` | A non-admin `PUT /api/settings/github` is `403` |
| `TestRepoListUsesCompanyToken` | The repo list runs `gh` twice, for the organization list and `user/repos`, each with the company token in `GH_TOKEN` and not in argv |
| `TestRepoListIncludesCollaboratorRepos` | With an empty organization list, a private repository from `user/repos`, filtered to the organization, is listed |
| `TestCompanyTestReportsAccess` | A classic token without the `repo` scope that sees no repositories gets `tokenKind: classic`, its scopes, `repoCount: 0`, and the repo-scope hint. The token is in neither the response nor argv |
| `TestProjectRepoUnique` | A second project on the same repo is `422` on `repo` |
| `TestCloneTokenNotInArgv` | The clone and fetch argv never contain the token, and the URL is built from the company org and project repo |
| `TestCheckoutUsesBaseBranch` | A worktree branches from the project's base branch |
| `TestIntakePerProject` | Ingest and accept on one project leave another project's requirements unchanged |
| `TestProjectDeploySettingsIsolated` | Deploy settings saved on one project are not returned for another |
| `TestProjectDeployTokenNeverReturned` | The project deploy route returns `deployTokenSet` and no token, and the token is in that project's `secrets.json` |
| `TestDeployLeadOnly` | A Developer member who saves deploy settings is `403` |
| `TestProjectLinkSameTribeOnly` | A link to a project in another tribe is `422` |
| `TestProjectLinkNoSelf` | A link from a project to itself is `422` |
| `TestProjectLinkOutsiderMisses` | A non-member reading links is `404` |
| `TestTribeAdminManagesMembers` | A tribe admin who is a Developer lists candidates, adds and removes members, and cannot remove themselves (`422`) (RFC-057) |
| `TestTribeAdminGrantsTribeAdmin` | A tribe admin sets and clears another member's flag, `/api/me` lists the tribe in `tribeAdminOf`, clearing their own flag is `422`, and a non-member is `404` |
| `TestNonTribeAdminCannotManage` | A plain member is `403` on add, remove, flag, candidates, and project create. A tribe admin is `403` on another tribe's members and projects |
| `TestTribeAdminCreatesProjectWithLinks` | A tribe admin creates a project with a same-tribe link. The `201` lists the link and `project_links` holds it |
| `TestCreateProjectLinkOtherTribeRollsBack` | A create naming a project in another tribe is `422` on `links` and stores no project |
| `TestTribeAdminSavesLinks` | A tribe admin who is a Developer saves links. A plain Developer member is `403` |
| `TestAdminEditsProjectSettings` | A company admin with the Product Owner role, not in the tribe, reads and saves repository, links, deploy, and conventions with `canEdit: true` and passes the Sync role check. The same admin is `404` on the project's cards, and an unknown project is `404` (RFC-059) |
| `TestProjectRepoCheckedOnGitHub` | Creating a project on a missing, archived, or moved repository is `422` on `repo`, a missing branch is `422` on `baseBranch`, and no project is stored. A repository save with a missing repository is `422`, and one GitHub finds is saved (RFC-060) |
| `TestProjectRepoNeedsCompanyToken` | With no company token, creating a project is `422` and names the company GitHub token |
| `TestProjectRepoGitHubDown` | A GitHub call that fails with anything but 404 is `502` |
| `TestTribeAdminEditsProjectSettings` | A tribe admin who is a Developer saves all four sections of a project in their tribe and is `404` on another tribe's project. A plain Developer member reads each with `canEdit: false` and is `403` on each save |
| `TestPackIncludesLinkedRequirements` | The implementer pack holds the linked project's accepted requirements as `<project>:REQ-001` |
| `TestSharedPathConfined` | A shared path with `..`, an absolute path, or `.git` is rejected |
| `TestReadLinkedFileReadOnly` | `read_linked_file` reads the linked export, rejects an unlinked project, and there is no linked write tool |
| `TestLinkedContextIsUntrusted` | Linked content sits inside the untrusted markers and a planted secret is `[redacted]` |
| `TestHookNotRun` | A fixture `pre-commit` hook is not executed |
| `TestFsmonitorOff` | A repo config with `core.fsmonitor` set to a command does not run it |
| `TestPushURL` | A remote named `origin` pointing elsewhere is ignored. The push goes to the settings URL with the card refspec only |
| `TestAskpass` | The token is absent from every recorded git argv and present only in the askpass env |
| `TestPushUsesOwnerToken` | Push, pull request, and reviewer calls carry the owner's token in the child environment, never in argv, and never the company token (RFC-054) |
| `TestOwnerGitHubBlocks` | A card whose owner has no verified token moves to `blocked` with `owner_github`, and no push or `gh` call runs |
| `TestGitHubLoginVerified` | The account token test stores the login `gh api user` reports. A new token clears it. A second user cannot verify the same login |
| `TestBaseRefUnchanged` | Local `refs/heads/main` is unchanged after a job, and a forced change blocks with `base_moved` |
| `TestRefinePromptMarkers` | For each field the refine request has no tools, uses `RefineStatementSystem` or `RefineAcceptanceSystem`, neither of which mentions deploy, status, cards, verdicts, or tools, and has the id, the field marked "(refine this)", the other marked "(context only)", and the guidance between one pair of markers, with marker lines in the draft and guidance stripped (RFC-086, RFC-087) |
| `TestStreamRedactorEverySplit` | `trace.NewStreamRedactor` with three secrets, one a database URL, over a text with a NUL byte and a multi-byte character split into three pieces at every pair of positions: the output is the redacted text whole, valid UTF-8, without NUL, and no piece carries part of a secret. `TestStreamRedactorReset` drops held text (RFC-087, RFC-088) |
| `TestTraceStreamRedactsSplitSecret` | A reviewer reply whose secret is split across three stream pieces reaches the trace stream as `[redacted]`, and no `delta` frame carries any part of the secret (RFC-088) |
| `TestTraceStreamRequiresMember` | The trace stream without a bearer is `401`, for a user outside the tribe is not `200`, under another project is `404`, and with a bad `since` is `422` (RFC-088) |
| `TestUsageAdminOnly` | Token usage without a bearer is `401`, for a Lead Developer who is not an admin is `403` even with a bad range, and an admin's response has no reply text (RFC-089) |
| `TestTribeUsageAccess` | Tribe usage is `403` for a plain member and for a tribe admin of another tribe, and a tribe admin sees only the tribe's project. `TestMyUsage` shows a member only their own rows and no other user's email (RFC-090) |
| `TestTraceStreamCap` | With 16 viewers on the card, another trace stream is `429` `rate_limited` (RFC-088) |
| `TestPromptMarkers` | The user pack contains both markers once, untrusted copies of the markers are stripped (including from the path map and a repository `AGENTS.md`), both of those sit inside the markers, the system hash matches the rewritten `ImplementerSystem` (RFC-082), and the system names `todo_write` and `replace_in_file` and neither deploy nor verdict |
| `TestPackInstructions` | With no instructions there is no block, and a forged heading in the failure note is dropped. With instructions the block appears once after the note and inside the markers, a planted heading or marker line inside it is dropped, a planted secret is redacted, and the system string names the block (RFC-073) |
| `TestPackConventions` | Rule and skill blocks sit between the markers before the requirements, a block over 16 KiB ends with `truncated: true`, blocks past 64 KiB become `omitted` notes, and a planted secret is `[redacted]` (RFC-055) |
| `TestConventionsCannotAddTool` | A rule whose body asks for a `deploy` tool leaves the implementer tool list unchanged, and a scripted `deploy` call is rejected |
| `TestConventionsContextEvent` | An attempt appends one `conventions_context` event with each rule and skill id, version, and hash |
| `TestReviewerPackConventions` | The reviewer pack carries the card's rules and skills inside the markers |
| `TestExportLargeArchive` | A 3 MiB file exports whole, no `.tar` is left behind, and a missing SHA returns a git error under 3 KiB (RFC-065) |
| `TestRunnerPathHasOnlyGitDir` | A job's `gh` child has `PATH` set to the git directory only, and no `PATH` when no git path is given (RFC-065) |
| `TestReviewerTools` | The reviewer tools are `read_diff`, `comment_pr`, and `verdict`, with no `deploy` or `edit_file` |
| `TestLegacyCardRoutesCheckRole` | `POST /api/cards/{id}/approve` is `403` for a Developer, `404` for a lead outside the project's tribe, and `200` for a lead member |
| `TestDBConnectorRefusesBoard` | A connector to the board's address, port, and database, also through `localhost` with the name in upper case, is refused before the socket opens (RFC-097) |
| `TestPostgresToolReadOnly` | Writes through `query`, including a `DELETE` inside `WITH` and `nextval`, are refused and change no row. `TestPostgresSingleStatement` refuses `SELECT 1; DROP TABLE` and `COMMIT; SET transaction_read_only = off` |
| `TestSQLServerStatementCheck` | `INTO`, `EXEC`, `OPENROWSET`, `OPENQUERY`, writes, `WAITFOR`, `DECLARE`, and a second statement are refused; keywords inside strings, quoted names, and comments are not read as statements |
| `TestDBPasswordRedacted` | A query that rebuilds the password reaches neither the model nor any event |
| `TestMCPCatalogAccess` | The catalog and the pull are `403` for a member who is not a tribe admin and for a tribe admin of another tribe, and a bad `source` is `403` before `422` for a member (RFC-096, RFC-100) |
| `TestMCPCatalogFeatured` | Every featured URL passes the strict URL check, GitHub and Linear are read-only endpoints, and featured ids are unique (RFC-099) |
| `TestMCPCatalogRegistrySlow` | A slow registry fails within the fetch budget, a failure is not fetched again for 30 seconds, and an abandoned request is not remembered (RFC-100) |
| `TestMCPImagePull` | A flag or a reference with a space, upper case, or `;` runs no engine; fixture mode and no engine refuse; a failed pull returns the fixed text (RFC-098) |
| `TestIntakeMCPServers` | A member's intake lists the tribe's slugs, enabled flags, and tool counts, and no URL, argv, image, config, or secret name (RFC-101) |
| `TestImplementerMCPMentionMissing` | Mentions that name no enabled server open no server and offer no MCP tool (RFC-102) |
| `TestImplementerMCPUnaccepted` | A requirement edited since acceptance offers no MCP tool to its card (RFC-102) |
| `TestMCPScopeCardAttach` | A card attach widens the scope only to enabled servers of the tribe, and an unaccepted requirement still gives none (RFC-105) |
| `TestCardContextRoles` | Only a Lead Developer, a tribe admin, or an admin attaches a server to a card (RFC-105) |
| `TestRunnerTokenHashed` | Registration tokens and runner secrets are stored only as SHA-256 hex (RFC-106) |
| `TestRunnerAuthScope` | A runner secret passes only `/api/runner/*`, and a user bearer never does (RFC-106) |
| `TestRunnerNeverGetsSecrets` | A runner never receives `databaseUrl`, `tokenKey`, a model key, or the owner's GitHub token (RFC-108) |
| `TestRunnerCannotSetStatus` | No runner call moves a card or appends a `transition` or `gate` event (RFC-108) |
| `TestBundleOnlyCardBranch` | The control plane pushes only `orch/<card-id>` at the attempt commit, descending from its base (RFC-108) |

## Remote runners

A runner is a trusted operator binary on a machine the company runs. It is not a model. It still holds nothing that writes to GitHub or spends model budget, so a lost runner machine leaks at most the company read token of the jobs it was running (RFC-108).

- The registration token is single use and expires in one hour. The runner secret is per runner and revocable by removing the runner.
- The runner config file is mode `0600` under the service user's home. The install script refuses to write it anywhere else.
- Every job call checks that the lease is open and held by the caller. Ids in the body never widen the lease to another card or project.
- The bundle route is the only route with a body cap above 1 MiB, at 64 MiB. The control plane runs `git bundle list-heads` and `git bundle unbundle` in its own clone; it never runs a hook or a filter from the bundle.
- The runner redacts the company token from every event and message it sends, and the control plane redacts again with the full job list.
- The install script verifies the binary's and the unit's SHA-256 against `SHA256SUMS` from the same origin, over HTTPS; it accepts `http://` only on loopback. It passes the registration token to `runner register` in the environment, not on its command line.

## Push notifications

RFC-113 sends data out of the process in two ways. Each is limited as below.

| Threat | Control |
| --- | --- |
| Card details leak through a browser vendor's push service | The payload is only `title` (a fixed phrase and, for blocked cards, a known reason label), `body` (card title and project name, at most 120 characters), `url`, and `tag` (the card id). It never holds a reason, note, code, log, review text, or token. It is encrypted with RFC 8291 to the browser's key, so the vendor sees only its size |
| A stored endpoint turns the server into a request forger (SSRF) | An endpoint must be `https`, port 443, no user info or fragment, at most 2048 bytes, on `fcm.googleapis.com`, `*.push.services.mozilla.com`, `*.notify.windows.com`, `web.push.apple.com`, or `*.push.apple.com`. It is checked on save and again before each send, and sent through `netguard.Client`, which refuses private, loopback, link-local, and metadata addresses after DNS and follows no redirect or proxy |
| A push service that hangs holds the listener | Sends run in their own goroutine within 30 seconds; the reply body is read at most 4 KiB and discarded |
| A dead or hostile endpoint is retried forever | `404` and `410` delete the subscription at once, and five failures in a row delete it |
| A person reads another person's notifications or subscriptions | Every notification query has `user_id` = the bearer's user and joins through tribe membership, so a person who left a project's tribe no longer sees its rows. Subscription delete is scoped to the caller. An endpoint saved again by another person moves to them, since a browser has one endpoint per site |
| The stream shows cards of a project the viewer lost | The stream checks membership on open (`404`) and on every recompute, and ends with `done` `reconnect: false` |
| Many streams exhaust the server | 8 streams per person and 256 in all, else `429`; each stream ends after 10 minutes |
| A notification link sends a person off-site | The bell and the service worker follow only paths starting with a single `/`; anything else goes to `/` |
| A person is notified of their own action, or twice | The actor is dropped, and `UNIQUE (event_id, user_id, kind)` with `ON CONFLICT DO NOTHING` writes one row; only the process that inserted it pushes |

| Test | Asserts |
| --- | --- |
| `TestPushEndpointGuard` | Private, metadata, lookalike, and non-HTTPS endpoints are refused (RFC-113) |
| `TestPushGone` | The decrypted payload holds exactly `title`, `body`, `url`, and `tag`, and dead endpoints are deleted (RFC-113) |
| `TestVAPIDKeySecret` | The private key is only in `secrets.json` and never in an API response (RFC-113) |

## Operator duties

These stay outside the program.

| Duty | Why it is still theirs |
| --- | --- |
| Choose the sandbox image digest | The harness will not pull a floating tag |
| Install deploy and rollback scripts outside the repo | RFC-038 refuses any other location |
| List dev hosts | The probe refuses an empty list |
| Keep the data directory off shared or synced disks | It holds tokens and the board |
| Leave `ORCHESTRATOR_CLOUD` unset on a laptop | Loopback stays the default bind |
| Set cloud mode only behind TLS | RFC-045 refuses a public bind without that flag |
| Keep `tokenKey` in the service secrets file | It decrypts every GitHub token |
| Keep `vapidPrivateKey` with `secrets.json` in backups and moves | A new key silently ends every browser subscription (RFC-113) |
| Allow outbound HTTPS to the browser push hosts when people want browser notifications | The server posts to the vendors' push services; the in-app bell needs no egress (RFC-113) |
| Ask developers for fine-grained tokens limited to the company organization, with Contents and Pull requests write | Unattended jobs push with each owner's token (RFC-054) |
| Review what dev checks reach | Dev check egress is open in v1 |
| Give each database connector a login that can only read, scoped to the tables jobs may see | The harness enforces read-only per call, but a SQL Server login is the main guard, and rows it can read reach the model (RFC-097) |
| Ask requirement authors to mention the MCP servers a requirement needs | A requirement with no mention gives its cards every enabled server of the tribe (RFC-102) |
| Keep the board database on its own server, or at least its own database name | The connector refuses only the board's own address, port, and database |

A production credential is still out of scope (NG-001). Hardening v1 does not add a safe place to put one. RFC-097 sets NG-001 aside for one case only: a read-only database login on a tribe's connector.
