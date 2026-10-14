# RFC-0046: Remote runners

Status: Accepted for implementation. Date: 2026-10-14.

This RFC adds decisions `RFC-106` to `RFC-109` under the new REQ-094. An operator installs a runner on a machine with one script, the way a self-hosted GitHub Actions runner is installed. The runner registers with the control plane, heartbeats, and claims implement jobs over HTTPS. It supersedes the RFC-043 and RFC-048 statements it names. Those statements stay in their files.

Sprints 72 to 77 in [../plan/sprints.md](../plan/sprints.md) implement this file. The fleet plan in [../plan/deployment-multi-agent-podman.md](../plan/deployment-multi-agent-podman.md) calls this Phase C.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-106 | An admin mints a single-use registration token; a runner exchanges it for a runner secret, heartbeats, and the server derives its status | none |
| RFC-107 | With the `runners` executor, implement jobs are leased in PostgreSQL; leases, not the worker lock, protect a remote job, and a reaper releases a lost runner's work | RFC-043 "Reconcile and lock cleanup run only in the process holding the worker lock.", for leased cards; RFC-048 "implementation stays under that project's cap", counted in one process, and "Reconcile ... resets ... that project's card locks", for cards a live lease holds |
| RFC-108 | A runner reaches the board only through the job API of its lease. It reports facts and never names a status. Model and MCP calls go through the control plane, and the control plane pushes the card branch from a git bundle | none |
| RFC-109 | The control plane serves the install script and pinned binaries, and refuses claims from a runner on another protocol | none |

## RFC-106 Runners register and heartbeat

### Problem

Every implement job runs inside the `serve` process that holds the project's worker lock. Adding capacity means moving whole projects between hosts by hand, and nothing tells an admin which machine is idle.

### Decision

- A company admin mints a registration token from the Runners page. It is 32 random bytes, shown once, stored as a SHA-256 hash, valid for one hour, and used once.
- `POST /api/runner/register` takes the token, a name, labels, and a slot count from 1 to 16. It returns a runner id and a runner secret of 32 random bytes. The database stores only the secret's hash. The runner keeps the secret in a `0600` file.
- `Authorization: Runner <secret>` is accepted only on `/api/runner/*`, and a user bearer is never accepted there. A runner secret never passes a user route.
- A runner heartbeats every 10 seconds with its version and protocol number. A heartbeat extends the runner's open leases.
- The server derives each runner's status: `offline` when the last heartbeat is older than 60 seconds, `draining` when an admin drained it, `busy` when it holds a lease, and `idle` otherwise. The list shows used and total slots.
- An admin drains, resumes, or removes a runner through the `actions` list the API returns. A drained runner keeps its leases and claims nothing new. A removed runner's secret stops passing, and its open leases are released as the reaper releases them.

### Why

A token that dies after one use and one hour is safe to paste into a shell. A long-lived secret per runner names the caller on every request and can be revoked alone.

## RFC-107 Leased implement jobs

### Problem

The worker counts implement slots in memory and clears every card lock of a project when it takes that project's lock. A job on another machine is invisible to both.

### Decision

- The company setting `jobs.executor` is `local` or `runners`; the default is `local`. With `local`, nothing changes. With `runners`, the worker does not start implement jobs. It still plans cards, runs review and refresh, and keeps every human gate.
- `POST /api/runner/claim` takes the oldest open card run request, with `SELECT ... FOR UPDATE SKIP LOCKED`, and inserts a `runner_leases` row in the same transaction. A project's open leases stay under its `scheduler.implementationCap`. A drained or removed runner claims nothing.
- A card's first claim sets `cards.runner_affinity`. A later run request of that card goes only to that runner while it is not offline, so its worktree stays on that runner's disk.
- A lease lives until `leased_until`, which a heartbeat extends to 60 seconds ahead. The reaper in `serve` ends a lease that is past `leased_until` or whose runner is offline or removed: it cancels the card's open attempt, deletes the card lock the lease holds, clears the affinity, and requeues the run request. It never moves the card.
- The lease is the card lock holder of a remote attempt. `ReconcileProject` leaves out cards with an open lease, and it leaves their card locks.
- A new runner that takes over a card fetches `orch/<card-id>` from the company remote before it adds the worktree, so its next commit fast-forwards the published branch.

### Why

PostgreSQL already holds the runs table, and row locks with `SKIP LOCKED` let many runners claim without a queue service (RFC-044, tech 0001). A lease names its holder and its expiry, so cleanup can tell a dead runner from a live one, which RFC-043 could not do across processes.

## RFC-108 The job API of a lease

### Problem

The implement loop writes attempts, events, tokens, and status directly with the service database, and it holds the model keys and the owner's token. A runner on another machine must hold neither.

### Decision

- A runner never holds `databaseUrl`, `tokenKey`, a model key, the owner's GitHub token, or an MCP credential. It holds the company token for clone and fetch, sent once per job, kept in memory, and redacted from everything it reports.
- Every job call names its lease, and a runner may only call the leases it holds. A call for another runner's lease, or for an ended lease, is 404.
- The loop's board reads and writes go through one interface. The harness passes the database; the runner passes its HTTPS client. The client may insert and finish the lease card's attempt, append `model_call`, `tool_call`, `budget_stop`, `job_error`, and `assertion_failed` events for that card, add tokens, write failure notes, and take or drop the card lock as the lease. It cannot append any other event kind and cannot move the card.
- The runner reports facts: `finished` with the loop's outcome, and `check` with the gating check's exit code and tail. The control plane maps each fact to `machine.Transition` with the same evidence as a local job: a commit moves the card to `self_check`, exit 0 publishes, another exit queues the next attempt or blocks.
- `POST /api/runner/jobs/{lease}/model` runs one model request through the router and budgets of the lease's route. The model is the lease's, whatever the request names. The control plane publishes the reply to the live trace as a local job does.
- MCP tools are listed and called through the control plane, which opens the job's servers with its own credentials, network guard, and sandbox engine.
- After a check exits 0 the runner uploads a git bundle of `orch/<card-id>`. The control plane accepts it only when its one head is `refs/heads/orch/<card-id>` at the attempt's commit, and that commit descends from the attempt's base. It then pushes with the owner's token and opens or updates the pull request.
- Linked projects are not offered to a remote attempt.

### Why

The control plane already decides every status from evidence (RFC-001). Keeping the decision there, and keeping every credential that can write to GitHub or spend model budget there, means a runner machine that is lost leaks one company read token at most.

## RFC-109 Install and protocol

### Decision

- `GET /runner/install.sh`, `GET /runner/download/SHA256SUMS`, and `GET /runner/download/{file}` serve the files of the release directory named by `ORCHESTRATOR_RUNNER_RELEASES`. They need no bearer.
- The install script downloads the binary for the machine's architecture, checks its SHA-256 against `SHA256SUMS`, creates the `orchestrator` service user, checks rootless Podman, registers, and installs a systemd unit that runs `orchestrator runner run`.
- The runner sends protocol number `1`. A claim from another protocol is refused with 409 `protocol`, and the Runners page says the runner needs an update.

## Set aside

- Review, refresh, deploy, and verify jobs on runners. They stay on the control plane; deploy scripts must live on the operator host (RFC-048).
- Labels that route a project to some runners. Labels are stored and shown only.
- Stdio MCP servers inside the runner's sandbox. Their environment carries tribe secrets.
- A runner that reaches PostgreSQL directly.

## Tests

Sprint 72: `TestRunnerTokenHashed`, `TestRegistrationSingleUse`, `TestRunnerAuthScope`, `TestRunnerStatusDerived`, and the Runners page Vitest cases.

Sprint 73: `TestClaimSkipLocked`, `TestClaimRespectsCap`, `TestCardAffinity`, `TestReaperRequeues`, `TestReconcileSkipsLiveLease`.

Sprint 74: `TestLocalExecutorUnchanged`, `TestModelProxyBudgets`, `TestLiveFromRunner`, `TestRunnerNeverGetsSecrets`.

Sprint 75: `TestRunnerCannotSetStatus`, `TestCheckExitDecides`, `TestBundleOnlyCardBranch`, `TestRunnerLeaseScope`.

Sprint 76: `TestMCPProxyNetguard`, `TestProtocolMismatchRefused`, and `shellcheck` on the install script.

Sprint 77: `TestRunnerChaos`.
