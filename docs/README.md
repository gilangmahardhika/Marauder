# Documents

This folder specifies the AI Coding Orchestrator v1. Read it in the order below.

| Document | Role |
| --- | --- |
| [prd/coding-orchestrator.md](prd/coding-orchestrator.md) | Requirements `REQ-001` to `REQ-097`, acceptance checks, non-goals |
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
| [rfc/0033-agent-loop.md](rfc/0033-agent-loop.md) | Decisions `RFC-080` to `RFC-085`. Ranged reads and exact replacements, a stable per-model history, a loop that finishes, a path map and `AGENTS.md` in the pack, a todo list, prompt caching, a sandboxed `run_command`, and fair attempt accounting |
| [rfc/0034-refine-requirement.md](rfc/0034-refine-requirement.md) | Decision `RFC-086`. A member asks the implement model to refine a manual requirement, compares the proposal with the current text, and applies it to the form before saving |
| [rfc/0035-streaming-field-refine.md](rfc/0035-streaming-field-refine.md) | Decision `RFC-087`. The statement and the acceptance lines refine apart, each from its own spark button, and the reply streams into the page as it is written |
| [rfc/0036-streaming-trace.md](rfc/0036-streaming-trace.md) | Decision `RFC-088`. While a card works, the Trace page holds one streamed connection that pushes stored events and the model's reply token by token |
| [rfc/0037-token-usage.md](rfc/0037-token-usage.md) | Decision `RFC-089`. An admin-only Token usage page totals stored model calls across projects, by LLM, model, project, user, and task type |
| [rfc/0038-scoped-token-usage.md](rfc/0038-scoped-token-usage.md) | Decision `RFC-090`. Token usage for one tribe, for its admins, and My token usage for each member |
| [rfc/0039-usage-by-source-and-card.md](rfc/0039-usage-by-source-and-card.md) | Decision `RFC-091`. Token usage filters and groups by source, Intake or Card, and by card |
| [rfc/0040-tribe-mcp-servers.md](rfc/0040-tribe-mcp-servers.md) | Decisions `RFC-092` to `RFC-095`. A tribe admin registers MCP servers and allowlists read-only tools, reached over HTTPS through a network guard or over stdio in the sandbox, which the implementer and refine call as untrusted data |
| [rfc/0041-mcp-catalog-and-database-connectors.md](rfc/0041-mcp-catalog-and-database-connectors.md) | Decisions `RFC-096` to `RFC-098`. A tribe admin adds MCP servers from a catalog of built-in read-only Postgres and SQL Server connectors and the official MCP Registry |
| [rfc/0042-featured-mcp-and-fast-catalog.md](rfc/0042-featured-mcp-and-fast-catalog.md) | Decisions `RFC-099` and `RFC-100`. The catalog adds featured remote servers and lists known entries without waiting on the registry |
| [rfc/0043-mcp-mentions-in-requirements.md](rfc/0043-mcp-mentions-in-requirements.md) | Decisions `RFC-101` and `RFC-102`. A requirement names the MCP servers it needs with `@slug`, and its cards' jobs offer only those |
| [rfc/0044-reviewers-before-github.md](rfc/0044-reviewers-before-github.md) | Decision `RFC-103`. A requirement names any tribe member as a reviewer, and GitHub is asked once they verify |
| [rfc/0045-layered-conventions-and-card-context.md](rfc/0045-layered-conventions-and-card-context.md) | Decisions `RFC-104` and `RFC-105`. A tribe sets its projects' conventions, a project switches each on or off, and a person changes one card's skills, rules, and MCP servers |
| [rfc/0046-remote-runners.md](rfc/0046-remote-runners.md) | Decisions `RFC-106` to `RFC-109`. An installed runner registers, heartbeats, and claims leased implement jobs over HTTPS, and the control plane keeps every status change and credential |
| [rfc/0047-card-page-review.md](rfc/0047-card-page-review.md) | Decision `RFC-110`. Publish stores the pull request title and description, and the card page leads with the next step, the pull request, and a numbered diff |
| [rfc/0048-squash-merge.md](rfc/0048-squash-merge.md) | Decision `RFC-111`. A project turns deploy off, and an approved card is then squash-merged on GitHub by a person and moves to Done |
| [rfc/0049-wont-fix.md](rfc/0049-wont-fix.md) | Decision `RFC-112`. A blocked card is closed as won't fix with a reason, its open pull request is closed, the board hides it, and Reopen returns it to Blocked |
| [rfc/0050-push-notifications.md](rfc/0050-push-notifications.md) | Decision `RFC-113`. Board changes are pushed to open pages over one stream, and the people a card waits on get a notification in the bell and in the browser |
| [rfc/0051-agent-questions.md](rfc/0051-agent-questions.md) | Decisions `RFC-114` and `RFC-115`. The implementer asks the team questions; a Manual card waits for a person's answer and an Auto card takes the agent's suggestion |
| [rfc/0052-needs-answer-column.md](rfc/0052-needs-answer-column.md) | Decision `RFC-116`. A card waiting for an answer has its own board lane, Needs answer, between Waiting and Running |
| [rfc/0053-continue-work.md](rfc/0053-continue-work.md) | Decisions `RFC-117` to `RFC-119`. A long attempt compacts its context, an unfinished attempt hands its edits, plan, and summary to the next one, and a runner starts from the control plane's card branch |
| [rfc/0054-agent-selection.md](rfc/0054-agent-selection.md) | Decisions `RFC-120` to `RFC-122`. An admin chooses the implement agent, the built-in loop or Claude Code, and a runner runs Claude Code with the job's model key in its environment only |
| [tech/0001-implementation.md](tech/0001-implementation.md) | Stack, processes, open source tools, build order |
| [tech/0002-data-and-api.md](tech/0002-data-and-api.md) | PostgreSQL schema, JSON API, settings shape. The contract both processes implement |
| [tech/0003-backend.md](tech/0003-backend.md) | Go packages, worker, jobs, CLI |
| [tech/0004-frontend.md](tech/0004-frontend.md) | SvelteKit client and settings pages |
| [tech/0005-verification.md](tech/0005-verification.md) | Tests, fixture runner, release order |
| [tech/0006-security-and-harness.md](tech/0006-security-and-harness.md) | Security controls and harness assertions |
| [plan/sprints.md](plan/sprints.md) | Sprints, each with a goal, tasks, and the tests that close it |
| [plan/checklist.md](plan/checklist.md) | Status of each sprint task and named test |
| [plan/deployment-multi-agent-podman.md](plan/deployment-multi-agent-podman.md) | Fleet topology and capacity for 20–50 parallel agents with Podman isolation |
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
