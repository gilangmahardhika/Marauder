# RFC-0003: Corrections from the full review

Status: Accepted for v1. Date: 2026-10-10.

This RFC adds decisions `RFC-035` through `RFC-043`. Four of them supersede earlier statements, in whole or in part. The superseded entries stay in their files with a pointer here.

Wording fixes made in place, which do not change a decision:

- The RFC-0001 introduction now points at `docs/README.md` and the guides in `docs/tech/`, instead of `docs/ai-coding-orchestrator.md`.
- The body of RFC-022, the consequences of RFC-017 and RFC-022, and the "What stays configurable" table in RFC-0001, now name `config.yaml` in the data directory (RFC-036). They used to name `orchestrator.models.yaml` and `orchestrator.repo.yaml`.
- RFC-030 now places `secrets.json` in the data directory instead of `.orchestrator/` in the working directory.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-035 | The CLI and a local web UI share one service layer | RFC-016 |
| RFC-036 | Orchestrator state lives in a data directory outside the target repo | |
| RFC-037 | Checks run on an exported copy, and only dev checks get network | RFC-028, network and mount clauses |
| RFC-038 | Deploy and rollback commands are operator files outside the repo | |
| RFC-039 | Probes target listed dev hosts and refuse link-local and loopback | RFC-029, address clause |
| RFC-040 | A transition is a compare-and-swap on the stored status | |
| RFC-041 | Release names where the card goes and how many attempts remain | RFC-020, release sentence |
| RFC-042 | The attempt cap counts implementer attempts only | |
| RFC-043 | One process holds the worker at a time | |

## RFC-035 The CLI and a local web UI share one service layer

The operator may use the CLI or a SvelteKit web UI served on loopback. Both call the same `internal/app` functions. The UI renders only the actions the API lists for a card. GitHub, model API, router, and deploy settings are edited through the API, with secrets write-only. Neither client writes card status on its own.

Settings include tokens that should never be echoed, model lists that feed router dropdowns, and fields validated against each other (operator login against approvers, model ids against families). Editing them through a form backed by validation is safer than hand-editing yaml. Once the server computes the legal actions, a second client cannot widen them.

Set aside: a board that moves cards by drag and drop. Dragging is a status write from the client. Also set aside: SvelteKit server routes or form actions. They would be a second backend beside the Go API.

Consequences: REQ-042 covers both clients. NG-006 narrows to "a UI that writes status". The API returns `actions` and `unblockTo` for each card. The UI builds as a static site.

Requirements: REQ-042, REQ-044. Non-goal: NG-006.

## RFC-036 Orchestrator state lives in a data directory outside the target repo

Config, secrets, the SQLite board, worktrees, exports, and logs live under one data directory. The default is `~/.orchestrator/<repo-name>-<first 8 hex of sha256 of the repo path>`. Startup exits when the data directory is inside the target repo or the target repo is inside the data directory. The directory is mode `0700`. Non-secret settings are `config.yaml`. Tokens are `secrets.json`, mode `0600`.

A settings file in the target repo can be changed by a pull request. After that pull request merges, the next card would run under approvers or deploy commands that a model proposed. Secrets inside a repo tree also end up in backups and in `git add -A`.

Set aside: `orchestrator.repo.yaml` committed in the target repo. Convenient to share, and it puts the approver list inside the change surface.

Consequences: a team that wants shared settings copies `config.yaml` by hand, outside the repo. The data directory is per target repo, so two repos do not share a board.

Requirements: REQ-045.

Superseded in part by RFC-044 in [0004-postgres.md](0004-postgres.md). The words "the SQLite board" in the statement no longer apply. Config, secrets, worktrees, exports, and logs stay in the data directory. The board is PostgreSQL, named by `databaseUrl` in `secrets.json`.

## RFC-037 Checks run on an exported copy, and only dev checks get network

The sandbox never mounts the worktree. The harness copies the files to check into a fresh directory under the data directory and mounts that copy. The gating local check exports the committed SHA. The `run_local_check` tool copies the working tree, excluding `.git`, without following symlinks. The dev check exports the deployed SHA. Local checks run with network none. Dev checks run with the engine's default rootless network and `DEV_URL` set, only after a person approved that SHA. The remaining rules of RFC-028 stand: no host socket, no host home, dropped capabilities, a read-only root, a non-root user, limits, and an image pinned by digest.

A worktree contains a `.git` file that points at the git admin directory. A test that can write that file can point git at a directory it controls, with a config that runs a command the next time the harness calls git on the host. A copy removes that path. Dev tests exist to call the dev server, so they cannot run with network none.

Set aside: mounting the worktree read-only. Many test runners write caches into the tree. Also set aside: an egress proxy that allows only the dev hosts. Worth a later RFC. In v1 the dev check container has no tokens, and the code it runs was approved by a person.

Consequences: dev check egress is not restricted by host in v1. A copy costs disk and time per check. Exports are deleted after the check ends.

Requirements: REQ-018, REQ-036.

## RFC-038 Deploy and rollback commands are operator files outside the repo

`deploy.devCommand` and `deploy.rollbackCommand` are absolute paths to regular files outside the target repo and the data directory, owned by the user running the orchestrator, and not writable by group or others. The harness checks this when settings are saved and again before every exec.

A deploy script inside the repo is code a model can edit through a pull request. Approval of that pull request would then decide what the deployer runs with the deploy token.

Set aside: `./scripts/deploy-dev.sh` resolved against the main checkout. The main checkout changes on every merge.

Consequences: the operator installs the deploy and rollback scripts in an operator directory such as `/opt/orchestrator/bin/`, outside both the repo and the data directory. Changing them is an operator action, not a card.

Requirements: REQ-033.

## RFC-039 Probes target listed dev hosts and refuse link-local and loopback

`deploy.healthUrl` and `deploy.devUrl` must use a host in `deploy.devHosts`. The harness resolves the host once, refuses link-local, multicast, unspecified, and loopback addresses, and allows private ranges for a listed host. It dials the resolved address, verifies TLS against the host name, refuses redirects, and stops after 5 seconds or 64 KiB. Fixture mode allows `http://127.0.0.1`.

Most dev servers live on a private network, so a private-range ban makes the feature unusable. The address that matters for request forgery is the link-local metadata endpoint, plus the loopback services on the operator machine.

Set aside: the RFC-029 rule that refused private ranges.

Consequences: an empty `devHosts` blocks deploy with `deploy_config`. Resolving once and dialing that address also stops a DNS answer from changing between the check and the request.

Requirements: REQ-033, REQ-035.

## RFC-040 A transition is a compare-and-swap on the stored status

A transition updates the card with `WHERE id = $1 AND status = $2`, in the same database transaction as its event and gate rows. Zero rows affected means the card changed after it was read. Nothing is written, no side effect follows, and the caller receives a conflict.

The approver can request changes from the UI while the deploy job is about to deploy. The CLI and the UI can act at the same moment. A read-then-write without the status in the `WHERE` clause lets both win.

Set aside: an in-process mutex. It does not cover a second process running a human gate command.

Consequences: jobs re-check the result of every transition before the next side effect. The API maps a conflict to `409`, and the UI reloads the card.

Requirements: REQ-006.

## RFC-041 Release names where the card goes and how many attempts remain

`unblock` sends a card blocked for `health`, `deploy_failed`, or `deploy_config` to `human_review`, with approval cleared. Every other card goes to `ready`. A card blocked for `source_changed` cannot be released while any linked requirement is unaccepted. By default the release keeps the attempts used. If that count has reached the cap, the release raises the card's cap by one. With `resetAttempts`, the count returns to zero. The release event records the destination and the counter policy.

A failed deploy or health check is an infrastructure fault. Sending that card to `ready` starts an implementer attempt that edits code for no reason. Keeping the counter at the cap without raising it would leave a released card that can never start.

Set aside: the RFC-020 sentence "`unblock` returns the card to `ready`".

Consequences: the API returns `unblockTo` for blocked cards so both clients can say where the card will go. The fixture's health step re-approves the same SHA after release.

Requirements: REQ-015.

## RFC-042 The attempt cap counts implementer attempts only

An attempt is one implementer conversation. The current attempt ends, and counts, when its gating local check fails, when the agent or a person requests changes, when required CI fails, when its dev check fails, when it ends without a commit, or when it passes and moves on. A crash-cancelled attempt does not count. Review, triage, and planner calls are recorded on the attempt they serve, or as card-free events, and do not count.

The earlier documents used "attempt" for model runs of every kind. A reviewer verdict counted as an attempt would use up the cap before the implementer could respond.

Set aside: one cap shared by every model call. The token cap already limits spend across all of them.

Consequences: `cards.attempts_used` increases when an attempt ends with a counted outcome. Review columns live on the attempt row they reviewed.

Requirements: REQ-015, REQ-021, REQ-038.

## RFC-043 One process holds the worker at a time

The worker is guarded by an advisory lock on `worker.lock` in the data directory. `serve` takes it at start and exits if it is held. `run-next` takes it when free and runs until idle. When `serve` holds it, `run-next` inserts a `runs` row that the server picks up. Reconcile and lock cleanup run only in the process holding the worker lock. Human gate commands do not need it, because RFC-040 covers them.

Reconcile clears card locks left by a dead process. A second worker running Reconcile would clear locks that a live job still holds.

Set aside: letting the CLI and the server both schedule jobs and relying on card locks alone. Lock cleanup cannot tell a dead holder from a live one in another process.

Consequences: the CLI and the UI both work while `serve` runs. Only one of them executes jobs.

Requirements: REQ-008, REQ-009, REQ-040.
