# Documents

This folder specifies the AI Coding Orchestrator v1. Read it in the order below.

| Document | Role |
| --- | --- |
| [prd/coding-orchestrator.md](prd/coding-orchestrator.md) | Requirements `REQ-001` to `REQ-045`, acceptance checks, non-goals |
| [rfc/0001-v1-architecture.md](rfc/0001-v1-architecture.md) | Decisions `RFC-001` to `RFC-023` |
| [rfc/0002-hardening.md](rfc/0002-hardening.md) | Decisions `RFC-024` to `RFC-034` |
| [rfc/0003-review-corrections.md](rfc/0003-review-corrections.md) | Decisions `RFC-035` to `RFC-043`, including the ones that supersede earlier ids |
| [tech/0001-implementation.md](tech/0001-implementation.md) | Stack, processes, open source tools, build order |
| [tech/0002-data-and-api.md](tech/0002-data-and-api.md) | SQLite schema, JSON API, settings shape. The contract both processes implement |
| [tech/0003-backend.md](tech/0003-backend.md) | Go packages, worker, jobs, CLI |
| [tech/0004-frontend.md](tech/0004-frontend.md) | SvelteKit client and settings pages |
| [tech/0005-verification.md](tech/0005-verification.md) | Tests, fixture runner, release order |
| [tech/0006-security-and-harness.md](tech/0006-security-and-harness.md) | Security controls and harness assertions |
| [ai-coding-orchestrator.md](ai-coding-orchestrator.md) | One-page overview and the original brief |

## When two documents disagree

1. The PRD acceptance checks.
2. RFC statements. A later RFC changes an earlier one only where it names the superseded id.
3. The contract in `tech/0002`.
4. The guides. `tech/0006` wins over `tech/0003`, `tech/0004`, and `tech/0005`, which win over `tech/0001`.
5. The overview.

A change that would make two of these disagree updates both in the same commit. Changing the meaning of an accepted RFC statement takes a new RFC (RFC-023).

## Status words

| Word | Meaning |
| --- | --- |
| Draft | Open to edits in place |
| Accepted | Statements are stable. Changes go through a superseding entry |
| Accepted for implementation | Guides that may be refined in place as long as they stay inside the PRD and RFCs |
