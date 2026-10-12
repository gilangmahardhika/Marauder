# Documents

This folder specifies the AI Coding Orchestrator v1. Read it in the order below.

| Document | Role |
| --- | --- |
| [prd/coding-orchestrator.md](prd/coding-orchestrator.md) | Requirements `REQ-001` to `REQ-077`, acceptance checks, non-goals |
| [rfc/0001-v1-architecture.md](rfc/0001-v1-architecture.md) | Decisions `RFC-001` to `RFC-023` |
| [rfc/0002-hardening.md](rfc/0002-hardening.md) | Decisions `RFC-024` to `RFC-034` |
| [rfc/0003-review-corrections.md](rfc/0003-review-corrections.md) | Decisions `RFC-035` to `RFC-043`, including the ones that supersede earlier ids |
| [rfc/0004-postgres.md](rfc/0004-postgres.md) | Decision `RFC-044`. The board is PostgreSQL |
| [rfc/0005-teams-and-cloud.md](rfc/0005-teams-and-cloud.md) | Decisions `RFC-045` to `RFC-048`. Accounts, job roles, tribes, and per-project workers |
| [rfc/0006-company-github-and-linked-projects.md](rfc/0006-company-github-and-linked-projects.md) | Decisions `RFC-049` to `RFC-051`. Company GitHub account, project repositories, linked projects |
| [rfc/0007-global-models-and-router.md](rfc/0007-global-models-and-router.md) | Decision `RFC-052`. LLM APIs and the router are one company setting |
| [rfc/0008-manual-requirements.md](rfc/0008-manual-requirements.md) | Decision `RFC-053`. Manual requirements, and every senior role accepts |
| [rfc/0009-card-owner.md](rfc/0009-card-owner.md) | Decision `RFC-054`. Card owner, owner commits and pull requests, intake reviewers |
| [rfc/0010-conventions-and-skills.md](rfc/0010-conventions-and-skills.md) | Decisions `RFC-055` and `RFC-056`. Company coding rules by stack, and integration skills |
| [rfc/0011-tribe-admin.md](rfc/0011-tribe-admin.md) | Decision `RFC-057`. A tribe admin manages members, projects, and links in one tribe |
| [rfc/0012-user-names.md](rfc/0012-user-names.md) | Decision `RFC-058`. A user has a display name, and pages show it instead of the email |
| [rfc/0013-project-settings-admins.md](rfc/0013-project-settings-admins.md) | Decision `RFC-059`. A company admin edits every project's settings, and a tribe admin edits its tribe's |
| [rfc/0014-repository-check.md](rfc/0014-repository-check.md) | Decision `RFC-060`. GitHub confirms a project's repository and base branch before they are saved |
| [rfc/0015-card-runs.md](rfc/0015-card-runs.md) | Decision `RFC-061`. A run request names one ready card, and the board shows it as queued |
| [rfc/0016-worker-first-slice.md](rfc/0016-worker-first-slice.md) | Decision `RFC-062`. `serve` runs a worker per project that makes one card per accepted requirement without a model |
| [rfc/0017-implement-job.md](rfc/0017-implement-job.md) | Decision `RFC-063`. The worker runs the implement job for queued cards, with real `openai` and `anthropic` clients, up to `pr_open` |
| [rfc/0018-implementer-finds-files.md](rfc/0018-implementer-finds-files.md) | Decision `RFC-064`. The implementer gets the read-only tools `list_files` and `search_files` |
| [rfc/0019-card-reaches-approved.md](rfc/0019-card-reaches-approved.md) | Decision `RFC-065`. A checked pull request is reviewed by a model, waits for its checks, and a person approves it |
| [rfc/0020-pull-request-text.md](rfc/0020-pull-request-text.md) | Decision `RFC-066`. The harness writes the pull request title and description, and the review verdict is a pull request comment |
| [rfc/0021-solo-approval.md](rfc/0021-solo-approval.md) | Decision `RFC-067`. A project may let the card owner and the bot login approve, recorded as not independent |
| [rfc/0022-approval-on-github-and-changes.md](rfc/0022-approval-on-github-and-changes.md) | Decision `RFC-068`. An approval posts a pull request comment and an `orchestrator/approval` status, and the card page shows the code changes |
| [rfc/0023-checks-decide-without-verdict.md](rfc/0023-checks-decide-without-verdict.md) | Decision `RFC-069`. A review with no verdict no longer blocks; the pull request checks decide, and an empty check list waits 2 minutes |
| [rfc/0024-self-review.md](rfc/0024-self-review.md) | Decision `RFC-070`. A Lead Developer or an admin may turn Self review on for a requirement, so its card owner approves alone, recorded as not independent |
| [rfc/0025-retry-and-review-mode.md](rfc/0025-retry-and-review-mode.md) | Decisions `RFC-071` and `RFC-072`. A failed attempt queues the next one while attempts remain, and a requirement's AI review mode lets a person decide after a `request_changes` verdict |
| [rfc/0026-attempt-instructions.md](rfc/0026-attempt-instructions.md) | Decision `RFC-073`. A person writes instructions for a card that every later attempt reads next to the newest failure note, until replaced |
| [rfc/0027-live-trace.md](rfc/0027-live-trace.md) | Decision `RFC-074`. The Trace page refreshes while a card works, and model and tool calls record the reply text and arguments, redacted and capped |
| [rfc/0028-reviewer-reply-limit.md](rfc/0028-reviewer-reply-limit.md) | Decision `RFC-075`. Reviewer requests may reply with 32,768 tokens, a reply cut at the limit is asked again once, and the failure names the cause |
| [rfc/0029-retry-ai-review.md](rfc/0029-retry-ai-review.md) | Decision `RFC-076`. A person may send an `agent_review` card whose AI review failed back to `pr_open`, so the review job runs again |
| [rfc/0030-red-checks-retry.md](rfc/0030-red-checks-retry.md) | Decision `RFC-077`. A red check queues the next attempt or blocks with `attempt_cap`, and a sent-back card out of attempts blocks |
| [rfc/0031-terminal-trace.md](rfc/0031-terminal-trace.md) | Decision `RFC-078`. The Trace page has a Terminal view that prints every stored event with its full text, follows new output, and polls every second while live |
| [rfc/0032-thinking-and-effort.md](rfc/0032-thinking-and-effort.md) | Decision `RFC-079`. Each model sets thinking and effort, every request to it sends them, and its reasoning goes back between tool turns |
| [tech/0001-implementation.md](tech/0001-implementation.md) | Stack, processes, open source tools, build order |
| [tech/0002-data-and-api.md](tech/0002-data-and-api.md) | PostgreSQL schema, JSON API, settings shape. The contract both processes implement |
| [tech/0003-backend.md](tech/0003-backend.md) | Go packages, worker, jobs, CLI |
| [tech/0004-frontend.md](tech/0004-frontend.md) | SvelteKit client and settings pages |
| [tech/0005-verification.md](tech/0005-verification.md) | Tests, fixture runner, release order |
| [tech/0006-security-and-harness.md](tech/0006-security-and-harness.md) | Security controls and harness assertions |
| [plan/sprints.md](plan/sprints.md) | Sprints, each with a goal, tasks, and the tests that close it |
| [plan/checklist.md](plan/checklist.md) | Status of each sprint task and named test |
| [ai-coding-orchestrator.md](ai-coding-orchestrator.md) | One-page overview and the original brief |

## When two documents disagree

1. The PRD acceptance checks.
2. RFC statements. A later RFC changes an earlier one only where it names the superseded id.
3. The contract in `tech/0002`.
4. The guides. `tech/0006` wins over `tech/0003`, `tech/0004`, and `tech/0005`, which win over `tech/0001`.
5. The overview.

The sprint plan schedules work. It does not override the documents above.

A change that would make two of these disagree updates both in the same commit. Changing the meaning of an accepted RFC statement takes a new RFC (RFC-023).

## Status words

| Word | Meaning |
| --- | --- |
| Draft | Open to edits in place |
| Accepted | Statements are stable. Changes go through a superseding entry |
| Accepted for implementation | Guides that may be refined in place as long as they stay inside the PRD and RFCs |
