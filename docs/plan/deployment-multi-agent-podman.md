# Deployment plan: multi-agent Podman fleet (20–50 parallel)

Status: Draft operations plan. Date: 2026-10-13.

This plan describes how to run the AI Coding Orchestrator so many coding agents work at the same time, with untrusted work isolated in Podman. It follows the accepted product shape in the PRD and RFCs. It does not change an accepted RFC statement. A later RFC is required if agent loops leave the harness process.

Document precedence stays in [../README.md](../README.md). Sandbox flags and limits are locked in [../tech/0006-security-and-harness.md](../tech/0006-security-and-harness.md). Per-project workers are RFC-048.

## What “an agent” means here

| Term | Meaning in this product |
| --- | --- |
| Agent job | One implement or review job for one card, started by that project’s worker under `scheduler.implementationCap` (default 2, max 8 today) |
| Sandbox | A short-lived rootless Podman container that runs `run_local_check`, the gating check, `run_command`, or a stdio MCP server |
| Parallel agents | Distinct cards with live jobs at the same time. Two projects may run together. One card still has one lock and one worktree (RFC-005, RFC-048) |

The harness (`orchestrator serve`) owns the board, transitions, git, GitHub, model calls, and credentials. Models never write card status. Podman never mounts the worktree, the data directory, or the container socket (RFC-037 / 0006).

Today the agent **loop** runs inside the harness process as a goroutine. Podman isolates **untrusted command execution**. That is enough for a first fleet at 20–50 parallel jobs if hosts are sized for peak sandbox load. Full “one agent job = one Podman container” is Phase B below and needs a superseding RFC before implementation.

## Goals

1. Run about **20 concurrent** agent jobs in steady state, with headroom to **50** at peak.
2. Keep every check and allowlisted command inside the pinned Podman image with the flags in 0006.
3. Keep one PostgreSQL board, one company LLM/router setting, and per-project worker and deploy locks.
4. Fail closed: no floating image tags, no inherited host env, no model-owned deploy.

## Non-goals

- Production deploy of target apps (v1 stops at per-project dev deploy).
- Auto-merge to the default branch.
- More than one writer on one card.
- Temporal, Redis, NATS, or Kafka as a job bus (stack choice in 0001).
- Docker as the primary engine (fallback only when Podman is absent).

---

## Architecture

### Logical layers

```text
┌─────────────────────────────────────────────────────────────────┐
│  Clients                                                         │
│  SvelteKit static UI (adapter-static)  ·  CLI (break-glass)      │
└───────────────────────────────┬─────────────────────────────────┘
                                │ HTTPS (TLS at edge)
┌───────────────────────────────▼─────────────────────────────────┐
│  Control plane                                                   │
│  reverse proxy (TLS, Host allowlist, ORCHESTRATOR_CLOUD=1)       │
│  orchestrator serve  — JSON API, sessions, settings, wake loop     │
│  one worker loop per project (flock on worker.lock)              │
└───────┬───────────────────────────────┬─────────────────────────┘
        │ SQL                           │ spawn / schedule
┌───────▼──────────┐          ┌─────────▼─────────────────────────┐
│  PostgreSQL 16+  │          │  Agent / sandbox plane             │
│  board, events,  │          │  implement + review jobs           │
│  sessions,       │          │  rootless Podman per check/command │
│  encrypted toks  │          │  worktrees + exports on local disk │
└──────────────────┘          └───────────────────────────────────┘
        ▲                                  │
        │                                  │ git / gh (owner + company tokens)
        │                         ┌────────▼────────┐
        │                         │  GitHub + LLMs  │
        └─────────────────────────┴─────────────────┘
```

### Trust and process boundary

| Component | Trust | Runs where |
| --- | --- | --- |
| JSON API + workers | Trusted harness | Control / worker hosts |
| PostgreSQL | Trusted board | Dedicated DB (or managed) |
| Static UI | Untrusted browser; bearer only | CDN or same edge |
| Check / `run_command` / stdio MCP | Untrusted code | Rootless Podman |
| Deploy / rollback scripts | Trusted operator binaries outside repo and data dir | Same host as that project’s worker, under deploy lock |

### Concurrency model (product)

| Scope | Parallelism |
| --- | --- |
| Across projects | Workers run at the same time (RFC-048) |
| Inside one project | Implement jobs ≤ `implementationCap` (1–8); review/refresh ≤ 2 each; deploy = 1 |
| Per card | One lock, one worktree, one branch `orch/<card-id>` |
| Sandbox | Ephemeral `--rm` containers; local checks `--network=none` |

### Resource envelope per sandbox (locked in 0006)

```text
--pids-limit=256
--memory=2g
--cpus=2
--read-only + tmpfs /tmp 1g
--cap-drop=ALL --security-opt=no-new-privileges
user 65532:65532
image must include @sha256:<64 hex>
```

Those limits drive capacity planning. Do not raise them in ops without changing 0006 and the matching tests.

---

## Topology

### Recommended: split control plane and agent hosts

Use this for 20–50 parallel agents. One machine can do a small demo; it will not hold a 50-agent peak safely.

```text
                    ┌──────────────────────┐
   users ──────────►│  Edge / TLS proxy    │
                    │  UI origin allowlist │
                    └──────────┬───────────┘
                               │
              ┌────────────────┼────────────────┐
              ▼                ▼                ▼
     ┌────────────────┐ ┌────────────┐  ┌────────────────┐
     │ API host(s)    │ │ Postgres   │  │ Object / backup │
     │ serve + UI     │ │ primary    │  │ (optional)     │
     │ (1–2 nodes)    │ │ (+ replica)│  │                │
     └───────┬────────┘ └─────▲──────┘  └────────────────┘
             │                │
             │  project shard │ SQL from all workers
             ▼                │
     ┌───────────────────────────────────────────────┐
     │  Agent host pool (rootless Podman)             │
     │  Host A: projects P1–Pn   (N concurrent jobs)  │
     │  Host B: projects …       (N concurrent jobs)  │
     │  Host C: …                                     │
     │  Each: local data dir, clones, worktrees,      │
     │        exports/, worker.lock per project       │
     └───────────────────────────────────────────────┘
```

### How to shard projects today (no new queue)

v1 has no remote job broker. Each `serve` process that holds a project’s `worker.lock` owns that project’s jobs (RFC-048 / 0003). To scale:

1. Partition projects across agent hosts (fixed assignment or operator map).
2. On each agent host run `orchestrator serve` with `ORCHESTRATOR_CLOUD=1`, the shared `databaseUrl` / `tokenKey`, and that host’s slice of project data directories under the service data root.
3. Put the public API on one primary `serve` (or a small HA pair that does **not** steal project locks from agent hosts). Human gates and reads are safe through compare-and-swap; only the lock holder runs Reconcile and jobs for that project.
4. If a second process already holds `worker.lock`, that project is skipped on this host (logged) and others still run.

Practical pattern for 20–50 agents:

| Role | Processes | Holds project locks? |
| --- | --- | --- |
| API primary | 1 (or 2 for failover) | Optional: only projects you want co-located |
| Agent hosts | 3–6 | Yes, disjoint project sets |
| Postgres | 1 primary (+ replica for HA) | N/A |

Keep project data directories and git worktrees **local to the host that runs that project’s worker**. Do not NFS-mount worktrees into Podman; exports are bind-mounted with `relabel=private`.

### Single-host topology (lab / &lt; ~8 agents)

```text
[ TLS optional ] → orchestrator serve → PostgreSQL (same or beside)
                 → rootless Podman
                 → static UI on loopback or edge
```

Use for fixture demos and small tribes. Size like one agent host in the tables below.

### Network

| Path | Rule |
| --- | --- |
| Browser → API | TLS at edge; `Host` allowlist; CORS = UI origin; bearer in `Authorization` |
| API → Postgres | Private network; `databaseUrl` only in `secrets.json` |
| Harness → LLM / GitHub | Egress from harness host, not from local-check containers |
| Local check / `run_command` | `--network=none` |
| Dev check | Rootless default network; only after human-approved deploy |
| MCP HTTPS | Netguard (no loopback/private/metadata targets); stdio MCP inside sandbox |

### Storage layout per agent host

```text
<data>/                          mode 0700
  secrets.json                   databaseUrl, tokenKey, model keys (0600)
  projects/<project-id>/
    worker.lock
    config.yaml
    secrets.json                 deploy token (0600)
    repo/                        company clone
    worktrees/                   one per card
    exports/                     ephemeral check copies (deleted after run)
    logs/
```

Data directory must stay outside every target repo (RFC-036).

---

## Capacity planning (20–50 parallel agents)

### Planning assumptions

| Assumption | Value | Why |
| --- | --- | --- |
| Parallel agents | 20 steady / 50 peak | Target |
| Sandbox hard limit | 2 vCPU + 2 GiB each | 0006 |
| Concurrent sandboxes at peak | 0.25–0.40 × active agents | Most wall time is LLM I/O; bursts when many checks finish together |
| Concurrent sandboxes steady | 0.10–0.20 × active agents | Typical implement mix |
| Harness + git per active job | ~0.25 vCPU + 0.5–1 GiB | Model client, worktree, redaction, event write |
| Postgres | Shared; see DB section | One board for all projects |
| `implementationCap` | Sum across projects ≥ desired concurrency | Per project max 8 today |

**Product cap math:** with today’s max `implementationCap` of 8 per project, you need at least `ceil(desired / 8)` projects with work queued to reach that many implement jobs. Example: 50 implement jobs need ≥ 7 projects at cap 8, or a future RFC that raises the per-project ceiling.

Review jobs add up to 2 per project on top of implement slots; size CPU/RAM for implement + review + occasional refresh.

### Host sizing formulas

Let:

- `A` = target concurrent agent jobs (implement + review)
- `S_peak` = `0.35 * A` (peak concurrent Podman checks/commands)
- `S_avg` = `0.15 * A`

Then:

```text
Podman vCPU (peak)   ≈ S_peak * 2
Podman RAM (peak)    ≈ S_peak * 2 GiB
Harness vCPU         ≈ A * 0.25
Harness RAM          ≈ A * 0.75 GiB
Headroom             × 1.3 on CPU and RAM
```

### Fleet tables

#### Steady: ~20 parallel agents

| Resource | Estimate | Recommendation |
| --- | --- | --- |
| Active jobs `A` | 20 | — |
| Peak sandboxes | ~7 | — |
| Avg sandboxes | ~3 | — |
| Podman peak | ~14 vCPU / 14 GiB | — |
| Harness | ~5 vCPU / 15 GiB | — |
| **Agent pool total** | **~25 vCPU / ~38 GiB** after 1.3× | **2 hosts × 16 vCPU / 32 GiB**, or **1 host × 32 vCPU / 64 GiB** |
| Disk per host | clones + worktrees + exports | **1–2 TiB SSD**; exports churn is high |
| Projects | enough caps to sum ≥ 20 | e.g. 4–10 projects at cap 2–5 |

#### Peak: ~50 parallel agents

| Resource | Estimate | Recommendation |
| --- | --- | --- |
| Active jobs `A` | 50 | — |
| Peak sandboxes | ~18 | — |
| Avg sandboxes | ~8 | — |
| Podman peak | ~36 vCPU / 36 GiB | — |
| Harness | ~13 vCPU / 38 GiB | — |
| **Agent pool total** | **~64 vCPU / ~96 GiB** after 1.3× | **4 hosts × 16–24 vCPU / 32–48 GiB**, or **3 hosts × 32 vCPU / 64 GiB** |
| Disk fleet | worktrees × cards in flight | **4–8 TiB SSD** total |
| Projects | sum of caps ≥ 50 | ≥ 7 projects at cap 8, or raise cap via RFC |

Worst-case stress (every agent in a check at once): `A * 2` vCPU and `A * 2` GiB for Podman alone (100 vCPU / 100 GiB at 50). Do not size only for average. Either:

- keep a global host-level cgroup budget and let jobs queue when Podman is saturated, or
- size for ~35–40% coincidence as above and accept brief queueing past that.

### PostgreSQL

| Load driver | Guidance for 20–50 agents |
| --- | --- |
| Connections | Start with `max_connections` ≥ `(API hosts + agent hosts) * 20` + admin; prefer a small pool per process |
| CPU / RAM | **4 vCPU / 16 GiB** for 20 agents; **8 vCPU / 32 GiB** for 50 |
| Storage | Events and traces grow fast (RFC-074 / RFC-088). **200–500 GiB** SSD with monitoring; plan retention for `events` |
| HA | Primary + streaming replica; backups daily + WAL |

### LLM and GitHub egress

| Dependency | Parallel 20 | Parallel 50 |
| --- | --- | --- |
| Provider RPM / TPM | Size to ~20–50 concurrent streams plus retries | Same; pin budgets in router |
| GitHub API | Clone/fetch (company token) + push/PR (owner tokens) | Watch secondary rate limits per owner login |
| Trace SSE viewers | Cap already 16/card, 64 global (RFC-088) | Keep; does not scale agents |

### Podman host tuning

On each agent host:

1. Rootless Podman for the service user; `XDG_RUNTIME_DIR` and `HOME` as required by 0006’s engine env.
2. Pre-pull every project’s pinned `@sha256:` image (`--pull=never` at run time).
3. Raise user namespace / inotify / file-descriptor limits for `A` worktrees and many short containers.
4. Separate volume for `<data>/projects/*/exports` if the root filesystem is small; Reconcile must still delete leftovers.
5. cgroup v2: optional host-level ceiling so a check storm cannot starve the harness or Postgres client.

### Example assignment (50 peak)

| Host | Spec | Role | Example load |
| --- | --- | --- | --- |
| `api-1` | 4 vCPU / 8 GiB | API + static UI origin behind TLS | No heavy implement load |
| `db-1` | 8 vCPU / 32 GiB | Postgres primary | All board traffic |
| `agent-1` | 24 vCPU / 48 GiB | Projects A–D, caps summing to 16 | ~16 jobs |
| `agent-2` | 24 vCPU / 48 GiB | Projects E–H, caps summing to 16 | ~16 jobs |
| `agent-3` | 24 vCPU / 48 GiB | Projects I–L, caps summing to 18 | ~18 jobs |

Total implement capacity ≈ 50. Redistribute projects when one tribe spikes.

---

## Isolation model (Podman)

### Phase A — deploy now (matches code)

```text
Harness process
  └─ implement/review goroutine (trusted)
       ├─ model HTTP
       ├─ git / gh children (scrubbed env)
       └─ podman run … <pinned image> <argv>   ← only untrusted exec
```

Properties:

- Same security assertions as `TestSandboxFlags`, `TestEnvScrub`, `TestRunCommandFlags`.
- Parallelism = sum of per-project caps across lock-holding hosts.
- Agent crash = process/job failure; Reconcile on next lock acquire cancels open attempts.

### Phase B — agent job in Podman (future, needs RFC)

Goal the operator asked for: each agent job runs in its own isolated container.

Sketch (not accepted until an RFC supersedes the in-process job assumption where needed):

```text
API / scheduler (trusted, holds locks, Transition only here)
  └─ podman run agent-image
       mounts: worktree export or narrow bind, read-only settings snapshot
       env: job id, short-lived job token (not databaseUrl)
       network: allow LLM + GitHub only (explicit)
       still no deploy credentials, no status write API
```

Rules that must remain true:

- Only `machine.Transition` in the harness writes status.
- Sandbox for checks stays stricter (`--network=none`, digest pin).
- Card lock and project deploy lock semantics unchanged.
- Secrets stay out of packs and events.

Until Phase B lands, treat “agents in Podman” in runbooks as **Phase A**: every agent’s shell/check surface is Podman-isolated even though the loop is in-process.

### Phase C — GitHub Actions–style runners (optional scale path)

Operators **register machines** the way self-hosted GitHub Actions runners are registered: run one install script, and the machine heartbeats and claims implement jobs. [RFC-0046](../rfc/0046-remote-runners.md) (`RFC-106` to `RFC-109`, REQ-094) decides this, and Sprints 72 to 77 build it. That RFC is the source of truth; this section only maps it onto the fleet. It supersedes the RFC-043 and RFC-048 statements it names, for leased cards only, and keeps the card lock, the deploy lock, and the rule that only `machine.Transition` writes status.

#### Do not create a second product repo

| Repo | Keep? | Why |
| --- | --- | --- |
| **This orchestrator repo** | Yes — put the runner here | Same Go module, same `internal/app` / `machine.Transition` / sandbox argv / redaction. A fork that talks to the board over ad-hoc HTTP will drift and can break the harness invariant |
| **New “agent runner” application repo** | No | Duplicates providers, packs, git/`gh`, and status rules |
| **Optional infra repo** (Terraform/Ansible/Packer) | Yes, if you want | Only provision VMs, Podman, systemd units, images — no business logic |

Shape inside this repo:

```text
backend/cmd/orchestrator serve           # control plane: API, claims, reaper, Transition, publish
backend/cmd/orchestrator runner register # trade a registration token for a runner secret
backend/cmd/orchestrator runner run      # heartbeat, claim, run leased implement jobs
```

One binary, two modes. Shared packages stay under `internal/`; the runner side is `internal/runner`.

#### What the product does (RFC-0046)

1. **Leases in Postgres.** With `jobs.executor: runners`, the worker starts no implement job. `POST /api/runner/claim` takes the oldest open run request with `FOR UPDATE SKIP LOCKED` and inserts a `runner_leases` row, kept under the project's `implementationCap`. No Redis or queue service (RFC-107).
2. **Runner identity.** An admin mints a one-hour, single-use registration token on **Admin → Runners**. The runner trades it for a per-runner secret kept in a `0600` file, heartbeats every 10 seconds, and shows as idle, busy, draining, or offline. Admins drain, resume, or remove it (RFC-106).
3. **Sticky cards.** A card's first claim pins it to that runner while the runner is online, so its worktree stays on one disk. A runner that takes over a card fetches the published `orch/<card-id>` first.
4. **Who may Transition.** Only the control plane. The runner reports facts (`finished`, check exit code) on its lease, and `serve` maps them to `machine.Transition` with the same evidence as a local job. A check passes when the runner's Podman process exits 0 (RFC-108).
5. **Secrets.** The runner never receives `databaseUrl`, `tokenKey`, a model key, or the owner's GitHub token. Model and MCP calls go through the control plane, which budgets, redacts, and feeds the Live view. The runner gets the company read token only to clone. It uploads a git bundle of `orch/<card-id>`, and `serve` pushes it and opens the PR.
6. **Lost runners.** A heartbeat extends the runner's leases to 60 seconds ahead. The reaper in `serve` ends a lease past `leased_until` or held by a removed runner: it cancels the open attempt, drops the card lock and affinity, and requeues the run. It never moves the card. `TestRunnerChaos` proves that another runner then finishes the card to `pr_open`.
7. **Versions.** A claim from a runner on another protocol is `409`, and the Runners page shows **Update required** (RFC-109).

#### What you do in infra (runner fleet)

Same capacity numbers as earlier; change **how** hosts join:

```text
┌─────────────┐     register / heartbeat / claim      ┌──────────────────┐
│ API + DB    │ ◄───────────────────────────────────► │ runner host 1..N │
│ (serve)     │     job lease, settings snapshot        │ orchestrator       │
└─────────────┘                                         │ runner + Podman   │
                                                        └──────────────────┘
```

Per runner host:

1. VM or bare metal sized from the capacity tables (e.g. 24 vCPU / 48 GiB for ~16 jobs); `--slots` is the job count, 1 to 16.
2. Rootless Podman with subordinate ids for the `orchestrator` user, and pre-pulled `@sha256:` images.
3. Run the command the **Add runner** page shows: `curl -fsSL https://orch.example/runner/install.sh | sudo bash -s -- --url https://orch.example --token … --slots 4`. The script checks the binary and `orchestrator-runner.service` against `SHA256SUMS`, creates the `orchestrator` user under `/var/lib/orchestrator-runner`, checks that Podman is rootless, registers, and enables the unit. Run it again to upgrade; it keeps the registration.
4. Local disk under `/var/lib/orchestrator-runner/work` for clones and worktrees of the cards pinned to this runner.
5. Egress to the control plane over HTTPS and to GitHub for clone and fetch. No Postgres, LLM, or GitHub write access is needed, and no public inbound except admin SSH/ssm.
6. Autoscaling group optional: scale on queue depth or idle runners, like GHA larger runners. It is still just more units of the same binary. Remove a scaled-in runner on the Runners page so its secret stops working.

Control plane infra adds:

- `scripts/release.sh <version>` on each release, with `ORCHESTRATOR_RUNNER_RELEASES` pointing `serve` at its output, so `/runner/install.sh` and `/runner/download/…` serve pinned binaries.
- Set **Implement jobs run on** to runners on the Runners page (`jobs.executor: runners`) once at least one runner is online.
- Watch the Runners page for offline runners and **Update required**. The reaper runs inside `serve`, on each worker tick.

#### When to stay on Phase A sharding instead

Use fixed project→host maps (section “How to shard projects today”) if:

- one team operates fewer than ~3 agent hosts, and
- you do not need self-service “add a machine with a token.”

Move to Phase C when hosts churn, tribes bring their own capacity, or you want GHA-like labels without editing a runbook map.

---

## Rollout

### 1. Bootstrap

1. Provision Postgres 16+; set `databaseUrl` and `tokenKey` in service `secrets.json` (0600).
2. `ORCHESTRATOR_CLOUD=1`, bind behind TLS, configure Host and UI origin.
3. Bootstrap first admin; company GitHub; LLM APIs; router.
4. Install Podman rootless; pull pinned images; set `sandboxEngine: podman` and digest on each project.

### 2. Scale to 20

1. Set each active project’s `implementationCap` so the sum is ~20 (respect 1–8).
2. Split projects across two agent hosts if one host is under 32 GiB.
3. Load-test: queue 20 ready cards; watch Podman `ps`, host CPU/RAM, Postgres connections, provider errors.
4. Confirm local checks never see `GH_TOKEN` / model keys (`TestEnvScrub` contract).

### 3. Scale to 50

1. Add agent hosts; rebalance project locks.
2. Raise Postgres to the 50-agent row; enable replica if not already.
3. Chaos: kill an agent host; confirm other projects continue; on restart, Reconcile cancels stale attempts and clears that host’s locks.
4. Document owner GitHub rate-limit playbooks (many cards, one owner).

### 4. Optional Phase B

1. Write RFC: job runtime boundary, credential minting, network policy, who may call Transition.
2. Implement behind a settings flag; keep Phase A path for fixture and small installs.
3. Add verification tests named in 0005/0006 for the new boundary.

### 5. Optional Phase C (GHA-style runners)

1. Build the release directory with `scripts/release.sh` and set `ORCHESTRATOR_RUNNER_RELEASES` on `serve` ([RFC-0046](../rfc/0046-remote-runners.md), RFC-109).
2. Provision runner VMs from an infra repo or packer image, and run the install command from **Admin → Runners → Add runner** on each.
3. Switch **Implement jobs run on** to runners (`jobs.executor: runners`) once the runners show idle. Chaos: stop one runner mid-attempt and confirm another finishes its card after the reaper requeues it.
4. Keep Phase A sharding as the small-fleet path.

---

## Observability and SLOs

| Signal | Target |
| --- | --- |
| Time from Run to first `model_call` event | &lt; 15 s when under cap |
| Check container start failure rate | &lt; 1% (image missing → `sandbox_config`) |
| Host memory with `A` at cap | &lt; 85% | 
| Postgres disk growth | alert on 7-day projection |
| Stuck `worker.lock` after crash | detect via heartbeat/metrics; operator restart |

Prefer metrics from the harness (job start/end, sandbox exit, transition counts) over scraping model tokens into a second store; token usage already has admin/tribe pages (RFC-089–091).

---

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Check storm exhausts CPU/RAM | Host cgroup ceiling; queue via existing caps; size for 35% coincidence |
| Per-project cap 8 blocks 50-on-few-projects | Spread work across projects or RFC to raise cap |
| NFS / shared worktrees | Do not share; keep project data on the lock holder |
| Floating tags | Refuse images without digest (0006) |
| Lock split brain | Disjoint project assignment; never two hosts for one project |
| Event table growth | Retention job; separate tablespace; cap trace payloads already apply |
| LLM provider outage | Cards block without burning attempts where RFC-085 says so; multi-family router |

---

## Checklist before calling the fleet ready

- [ ] Postgres 16+ reachable only from control/agent hosts; `databaseUrl` never returned by API
- [ ] `ORCHESTRATOR_CLOUD=1` and TLS edge; loopback-only still works for local fixture
- [ ] Rootless Podman on every agent host; pinned digests pre-pulled
- [ ] Sum of `implementationCap` across lock-holding projects ≥ target concurrency
- [ ] Project → host map documented; each `worker.lock` has one owner
- [ ] Deploy scripts outside repo and data dir; one deploy lock per project
- [ ] Disk and memory headroom measured at target `A` with real check images
- [ ] Backup + restore drill for Postgres and a sample project data dir

---

## Summary

| Topic | Decision |
| --- | --- |
| Arch | Go harness + Postgres board + static UI; Podman for untrusted exec; per-project workers |
| Topology | API/DB control plane + sharded agent hosts with rootless Podman and local project data |
| 20 agents | ~25 vCPU / ~38 GiB agent capacity (often 1–2 hosts) |
| 50 agents | ~64 vCPU / ~96 GiB agent capacity (often 3–4 hosts) + stronger Postgres |
| Isolation today | Phase A: in-process agent loop, Podman sandboxes |
| Isolation next | Phase B: per-job Podman runtime via new RFC |
| GHA-like runners | Phase C: [RFC-0046](../rfc/0046-remote-runners.md); same binary (`orchestrator runner`), Postgres leases, HTTPS only, sticky cards; optional infra-only second repo |
