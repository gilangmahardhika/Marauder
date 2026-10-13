# Implementation plan: GitHub Actions–style agent runners

Status: Draft plan only. Date: 2026-10-13. **No code in this change.**

This plan describes how to add self-hosted runners that register with the control plane, report idle/busy status, and claim queued card jobs. It builds on [deployment-multi-agent-podman.md](deployment-multi-agent-podman.md) Phase C. It does not change an accepted RFC statement; a new RFC must land before implementation starts.

Document precedence: [../README.md](../README.md). Workers today: RFC-048, `docs/tech/0003-backend.md`. Sandbox: `docs/tech/0006-security-and-harness.md`.

---

## 1. Decision: install script (like GHA), not “runner = one container”

### Recommendation

**Ship an install script + systemd (or Windows service) that runs the `orchestrator runner` binary on the host**, the same pattern as GitHub’s self-hosted runner (`config.sh` / `run.sh` + svc).

Do **not** make the primary install model “one container that is the whole agent.”

| Option | Verdict | Why |
| --- | --- | --- |
| **Host install script + binary + systemd** | **Choose this** | Matches GHA ops muscle memory; rootless Podman for checks runs as a **sibling** on the host; no nested engine; fits 0006 (“never mount the container socket”) |
| Runner as a single long-lived container | Avoid as default | Checks need Podman/Docker; nested containers or mounting the host socket weaken isolation and fight the sandbox contract |
| VM/AMI/Packer image that already ran the script | Optional | Fine as an infra convenience on top of the script; still host-native Podman |
| Ephemeral “job pod” per card | Later stretch | Useful after sticky worktrees are solved; not the first milestone |

### What the script does (product behavior, not code yet)

1. Download or copy the `orchestrator` binary (version-pinned).
2. Prompt or flags: control-plane URL, registration token, runner name, labels, work directory, slot count.
3. Write a local runner config (mode `0600`) under the work directory — **no** service `databaseUrl` in that file if the runner talks HTTPS to the API only.
4. Register once; store runner id + runner auth token.
5. Install a user systemd unit (or print the unit for the operator).
6. `orchestrator runner` loops: heartbeat → claim → run job → report → idle.

Podman stays a **host** dependency the script checks for (`podman version`), same as today for real repos.

### Container use that *is* correct

- **Sandbox containers** for `run_local_check` / `run_command` / stdio MCP (existing 0006 shape).
- Optional **control-plane** compose: API + Postgres + static UI in containers.
- Not: wrapping the runner agent itself in a container that must also spawn sandboxes.

---

## 2. Goals and non-goals

### Goals

1. Operators add capacity like GHA: install script → runner appears in UI as **Online / Idle** or **Online / Busy**.
2. Admins (and later tribe admins if desired) see which runners are idle, busy (with current card/project), offline, or disabled.
3. A **Run** on a card enqueues work; an **idle** runner with matching labels and free slots **claims** it; if none are free, the card stays **Queued** until one is.
4. Harness invariants hold: only `machine.Transition` writes status; one card one lock; deploy lock per project; Podman sandbox flags unchanged.
5. No second product repository; runner is a mode of this binary.

### Non-goals (this plan)

- Implementing the code, migrations, or UI in the same change as this doc.
- Replacing Postgres with Redis/Temporal/NATS/Kafka (keep one board DB; use row leases).
- Auto-scaling cloud APIs (may come later; queue depth metrics are enough to hang ASG on).
- Production deploy of target apps.
- Nested “runner-in-container” as the supported path.

---

## 3. Target behavior (operator story)

```text
1. Admin creates a registration token (org-wide or tribe-scoped).
2. On a VM: curl …/install-runner.sh | bash
      → ./config.sh --url https://orch.example --token XXX --name agent-3 --labels podman,linux
      → sudo ./svc.sh install && sudo ./svc.sh start
3. UI → Runners: agent-3  Online  Idle  slots 0/4
4. Member presses Run on card_abc (or Run ready cards).
5. Card shows Queued until a runner claims it.
6. Idle runner claims → Busy (card_abc, project X) → implement job → Idle again.
7. If all runners Busy or Offline, new Runs stay Queued (existing runs row semantics, plus runner assignment).
```

Board copy stays honest: **Queued** means not yet claimed by a runner (or waiting for a slot), not “running.”

---

## 4. Architecture after the change

```text
┌──────────────────────────────────────────────────────────┐
│ Control plane: orchestrator serve                        │
│  - sessions, settings, Transition for human gates        │
│  - runner registry, heartbeats, lease reaper             │
│  - enqueue runs (unchanged entry: POST …/run)            │
│  - optional: in-process worker OFF when runner mode on   │
└─────────────────────┬────────────────────────────────────┘
                      │ HTTPS (runner auth) + Postgres board
        ┌─────────────┼─────────────┐
        ▼             ▼             ▼
   runner host A  runner host B  runner host C
   (script+systemd)  …              …
        │
        ├─ orchestrator runner (trusted harness code)
        │     claim job → git/gh/LLM → Transition
        └─ podman run … (sandbox only)
```

**Who writes card status for jobs:** the runner process, using the same `internal/app` + `machine.Transition` paths as today’s worker (preferred), with a DB role or job-scoped connection. Alternative: runner posts evidence to a narrow internal API that calls `Transition` on `serve` — slightly cleaner secret split, more moving parts. **Plan default: runner runs harness job code; serve owns registry + human HTTP.**

**Project `worker.lock`:** superseded for *job execution* by runner leases. Keep a project-level **coordinator** concern for Reconcile and deploy (one deploy at a time). The RFC must say whether Reconcile runs on `serve` only, or on a designated “project leader” runner.

**Recommendation for v1 of runners:** `serve` remains the only Reconcile/deploy starter; runners only execute implement/review (and refresh if assigned). That minimizes lock redesign.

---

## 5. Runner status model

### States (UI + API)

| Status | Meaning | How set |
| --- | --- | --- |
| `offline` | No successful heartbeat within TTL (e.g. 30s × 3) | Reaper |
| `idle` | Online, `busy_slots == 0` | Heartbeat |
| `busy` | Online, `busy_slots > 0` | Heartbeat / claim |
| `disabled` | Admin disabled; must not claim | Admin API |

Derived display: `Online · Idle`, `Online · Busy (2/4)`, `Offline`, `Disabled`. Optional detail: current job list `{ projectId, cardId, jobKind, startedAt }`.

### Capacity

Each runner advertises:

- `name`, `labels[]`
- `max_slots` (concurrent jobs on that host; operator-set, capped by policy e.g. 1–16)
- `busy_slots` (from live leases)
- `version`, `hostname`, `last_heartbeat_at`

Global scheduling: a job is claimable only if some online, non-disabled runner has `busy_slots < max_slots` and label match. Otherwise it stays queued.

Per-project `implementationCap` remains a **project** fairness cap (how many of that project’s implements may run at once across the fleet). Effective concurrency = min(project cap remaining, free runner slots with matching labels).

---

## 6. Queue and assignment

### Enqueue (unchanged UX)

- `POST /api/projects/{id}/cards/{card}/run` and `POST /api/projects/{id}/runs` still insert `runs` rows (RFC-061).
- Card JSON keeps `queued: true` until a runner has claimed and the job has started (or define `queued` until `processed_at` as today, and add `runnerId` when claimed).

### Claim (new)

```text
idle/busy runner loop:
  heartbeat
  while busy_slots < max_slots:
    claim next eligible run (SKIP LOCKED)
      filters: labels, project allowlist (optional), implementationCap, card lock free, deps done
    start job goroutine/process
    on finish: release lease, heartbeat
```

Eligibility (implement):

1. Open `runs` row with `card_id`, `processed_at` null.
2. Card `ready` / `changes_requested` / retryable `in_progress`, deps done, under attempt cap.
3. Project’s running implement count &lt; `implementationCap`.
4. Runner labels satisfy job requirements (default: `podman` required for real repos).
5. Optional sticky label `project:<id>` if the operator pinned worktrees to that host.

If no runner claims within N minutes, UI still shows Queued; optional admin alert “stuck queue.”

### Fairness

- Oldest `runs.created_at` first (same as today).
- Do not starve small projects: optional round-robin by `project_id` in a later iteration; not required for first ship.

### Failure

| Event | Result |
| --- | --- |
| Runner crash mid-job | Heartbeat TTL expires → lease released → attempt cancelled / Reconcile path on serve → card can be re-queued or left for retry policy |
| Sandbox missing | Job blocks `sandbox_config` as today |
| Runner disabled while busy | Finish current jobs; no new claims |

---

## 7. Data and API contract (to specify in RFC + 0002)

### New tables (sketch)

```text
runners
  id, name, labels[], max_slots, disabled,
  token_hash, created_at, last_heartbeat_at,
  version, hostname, tribe_id null|set

runner_jobs  (or columns on runs)
  run_id, runner_id, card_id, project_id,
  job_kind (implement|review|…),
  claimed_at, heartbeat_at, finished_at, lease_until
```

Registration tokens: one-time or reusable hashed secrets, scope `company` or `tribe`, expiry.

### New routes (sketch)

| Route | Who | Purpose |
| --- | --- | --- |
| `POST /api/runners/register` | registration token | Create runner, return runner auth |
| `POST /api/runners/heartbeat` | runner auth | Status, slots, optional job progress |
| `POST /api/runners/claim` | runner auth | Atomically claim 0..N jobs |
| `POST /api/runners/jobs/{id}/finish` | runner auth | If evidence-API variant only |
| `GET /api/runners` | admin (tribe admin later) | List status for monitor UI |
| `POST /api/runners/{id}/disable` | admin | Drain |
| `DELETE /api/runners/{id}` | admin | Remove |
| `POST /api/runner-tokens` | admin | Mint install token |

Human board routes unchanged. Card payload may add `runnerId`, `runnerName` when claimed.

### UI

- New **Runners** page (admin): table of name, status pill, slots, labels, current cards, last seen, Disable.
- Board/card: keep Queued; when claimed show “On runner {name}” in details or trace header (optional first ship).

---

## 8. Security plan

1. Runner auth ≠ user bearer; hashed at rest; rotatable.
2. Registration token shown once; scoped; expiry.
3. Runner host config `0600`; script refuses world-writable work dir.
4. Sandbox rules unchanged: digest pin, `--network=none` for local checks, no socket mount, no worktree mount.
5. Redaction list still applied on every pack/event.
6. Deploy credentials stay off implement runners if deploy remains on `serve` only (recommended).
7. Label spoofing: labels set only at register/admin update, not free-form on every heartbeat from an untrusted field without auth.

---

## 9. Docs and RFC sequence (before code)

| Step | Deliverable |
| --- | --- |
| R1 | New RFC: “Self-hosted agent runners” — supersedes RFC-048 / 0001 “no job bus” **only where named**; keeps card lock, deploy lock, Transition, Postgres board |
| R2 | PRD acceptance checks for runner register, status, queue→idle claim |
| R3 | Update `tech/0002` (schema + routes), `0003` (runner package + claim loop), `0004` (Runners page), `0005`/`0006` (named tests), `0001` (allow runner mode; still no Redis) |
| R4 | Sprint slice in `plan/sprints.md` + checklist rows |
| R5 | Then implement in code order below |

Do not start coding until R1–R4 are accepted for implementation.

---

## 10. Implementation phases (code later)

### Phase 0 — Spec freeze

- [ ] Land RFC + tech contract updates.
- [ ] Choose secret model: runner uses DB vs evidence API (default: runner runs jobs with harness code).
- [ ] Choose Reconcile/deploy owner: `serve` only (default).

### Phase 1 — Registry and status (no job move yet)

- [ ] Migrations: `runners`, tokens, heartbeats.
- [ ] Register / heartbeat / list / disable APIs.
- [ ] Admin Runners UI with Idle / Busy / Offline / Disabled.
- [ ] Reaper marks offline.
- [ ] Tests: `TestRunnerRegister`, `TestRunnerHeartbeat`, `TestRunnerOffline`, `TestRunnerDisable`.
- [ ] **Still:** existing in-process project workers run jobs (feature flag `runners.execute=false`).

### Phase 2 — Install script and packaging

- [ ] `scripts/install-runner.sh` (or `dist/runner/`) mirroring GHA flow: config, svc install, uninstall.
- [ ] Document host prerequisites: Go binary, git, gh, Podman, disk paths.
- [ ] Version check: runner refuses if control plane major mismatch.
- [ ] Tests: script dry-run in CI where possible; unit-test config parsing.

### Phase 3 — Claim path and queue visibility

- [ ] `claim` with `SKIP LOCKED`; write `runner_id` on lease; set `processed_at` when job starts (preserve RFC-061 meaning).
- [ ] Runner loop in `orchestrator runner`.
- [ ] Board: Queued until claim; optional runner name when busy.
- [ ] Honor `implementationCap` fleet-wide for that project.
- [ ] Tests: `TestClaimIdleRunner`, `TestQueueWaitsWhenBusy`, `TestClaimSkipLocked`, `TestLabelMatch`, `TestProjectCapWithRunners`.

### Phase 4 — Move implement (then review) off in-process worker

- [ ] Feature flag `runners.execute=true`: `serve` does not start implement jobs; runners do.
- [ ] Review jobs claimable (cap 2 per project still).
- [ ] Sticky labels for projects that need local worktrees; document “one runner set per heavy project” for first cut.
- [ ] Crash/lease expiry integration with Reconcile on `serve`.
- [ ] Tests: `TestRunnerImplementsCard`, `TestRunnerCrashReleasesLease`, `TestParallelRunners`, `TestDeployStillOnServe`.

### Phase 5 — Hardening and ops

- [ ] Metrics: idle count, busy slots, queue depth, claim latency, heartbeat loss.
- [ ] Drain/disable; graceful shutdown finishes current job.
- [ ] Token rotation; audit `config_changed` for runner admin actions.
- [ ] Load test: 20 then 50 concurrent jobs per [deployment-multi-agent-podman.md](deployment-multi-agent-podman.md).
- [ ] Fixture mode: runners optional; local `serve` worker remains for REQ-043.

---

## 11. Suggested package layout (when coding starts)

```text
backend/internal/runner/     # register, heartbeat, claim, reaper (no HTTP)
backend/internal/app/        # RunnerRegister, RunnerHeartbeat, ClaimJobs, …
backend/internal/httpapi/    # /api/runners*
backend/internal/cli/        # orchestrator runner, runner-tokens
frontend/src/routes/runners/ # admin monitor
scripts/install-runner.sh
docs/rfc/00xx-self-hosted-runners.md
```

No new top-level Go module. No separate agent repository.

---

## 12. Mapping to GHA concepts

| GitHub Actions | This product |
| --- | --- |
| `config.sh` + `run.sh` / svc | install script + `orchestrator runner` + systemd |
| Runner idle / busy | `idle` / `busy` + slot counts |
| Offline | heartbeat TTL |
| Labels | `labels[]` on runner; job requirements |
| Job queued in Actions | `runs` row + Queued on card |
| Job assigned to runner | claim lease + `runner_id` |
| `_work` directory | runner work dir + per-project data paths |
| Nested container jobs | **Not used**; Podman only for sandbox argv |

---

## 13. Risks

| Risk | Plan mitigation |
| --- | --- |
| Nested container install temptation | Docs + script default to host binary; refuse “runner image” as supported if it mounts engine socket |
| Worktree on wrong host | Sticky `project:` labels for v1; no NFS into Podman |
| Double execution | `SKIP LOCKED` + card lock + single open `runs` row per card |
| Split brain with old worker | Feature flag; only one execute path on |
| Secret sprawl on runners | Prefer job-scoped decrypt; deploy stays on serve |
| Queue never drains | Alert on oldest queued age; offline reaper |

---

## 14. Exit criteria for “done”

1. Admin can mint a token, run the install script on a clean VM with Podman, and see the runner **Online · Idle**.
2. With one runner `max_slots=1`, two Runs → one **Busy** with a card, one card **Queued**; when the first finishes, the second claims without a second Run click.
3. Runner kill → within TTL, status **Offline**, lease cleared, no stuck permanent Busy; board consistent with Reconcile rules.
4. REQ-043 fixture still passes with runners disabled.
5. Named tests in 0005/0006 for register, heartbeat, claim, queue, crash, and sandbox flags still green.
6. No Redis/Temporal introduced; board remains PostgreSQL.

---

## 15. Summary answers

| Question | Answer |
| --- | --- |
| Single container for the agent? | **No** as the default. Use a **GHA-like install script** on the host; Podman only for sandboxes. |
| New repo? | **No** product repo. Optional infra repo for VMs only. |
| Runner status? | First-class `runners` rows + admin UI: Idle / Busy / Offline / Disabled + slots. |
| Card → idle runner? | Run enqueues; idle runner **claims**; else stays Queued. |
| Next action | Write the superseding RFC (section 9 R1), then Phase 1 — **no feature code before that**. |
