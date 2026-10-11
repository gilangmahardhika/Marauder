# Documents

This folder specifies the AI Coding Orchestrator v1. Read it in the order below.

| Document | Role |
| --- | --- |
| [prd/coding-orchestrator.md](prd/coding-orchestrator.md) | Requirements `REQ-001` to `REQ-063`, acceptance checks, non-goals |
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
