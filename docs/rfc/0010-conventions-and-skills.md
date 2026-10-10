# RFC-0010: Coding conventions and integration skills

Status: Accepted for implementation. Date: 2026-10-10.

This RFC adds decisions `RFC-055` and `RFC-056`. It supersedes earlier statements only where it names the id. Those statements stay in their files.

Sprint 18 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-055 | An admin keeps one catalog of coding rules, and a project's stacks pick the rules that apply | RFC-013, fixed pack contents |
| RFC-056 | An admin writes integration skills, a project enables them, and the planner attaches them to cards | RFC-013, fixed pack contents; the `create_card` arguments in REQ-017 |

## RFC-055 An admin keeps one catalog of coding rules, and a project's stacks pick the rules that apply

The company has one catalog of coding rules. A rule is global, such as naming, database tables and columns, or general style, or it belongs to one stack, such as Go, TypeScript, SvelteKit, or Django. A stack is a language or framework with marker files and dependencies that identify it in a repository. Only an admin edits the catalog. Every signed-in user may read it.

A project's stacks are detected from its clone on each Sync: marker files such as `go.mod`, and dependencies in `package.json` or `pyproject.toml`. A Lead Developer member may add stacks by hand, for example for a new repository that has no files yet. A Sync replaces the detected stacks and keeps the ones added by hand. The rules that apply to a card are the global rules plus the rules of the project's stacks.

Each rule save increments its version, stores its content hash, and appends a revision row. A job reads the rules once at the start of an attempt and keeps them for that attempt. The attempt's trace lists every rule by id, version, and hash, so the pack can be rebuilt.

The implementer and reviewer packs carry the rules inside the untrusted markers, after the card block and before the requirements. Each rule is capped, and the rule section has its own cap inside the pack cap. A rule that does not fit is replaced by a note. The rule text is data: it cannot add a tool, name a command, select a credential, or change a card status. The tool allowlist stays the boundary (RFC-024). The system constant names the rule and skill blocks, which changes the system hash once, as a deliberate release.

A rule in each developer's editor or in the repository drifts per person and per repository, and the harness cannot show which rules built a card.

Set aside: reading `AGENTS.md`, `.cursor/rules`, or similar files from the target repository. Those files are repository data under RFC-024. Also set aside: rule overrides per tribe or per project, and putting rule text in the system prompt, which would make the system hash change with every edit.

Consequences: `stacks`, `convention_rules`, `convention_revisions`, and `project_stacks` are new tables. The routes are `/api/conventions` and `/api/projects/{projectId}/settings/conventions`. A catalog save appends `config_changed` with section `conventions`. A Sync appends `stacks_detected`. An attempt appends a `tool_call` event named `conventions_context`. The reviewer reads the rules as part of its checklist. A breach is `request_changes` with a review note. The verdict format does not change.

Requirements: REQ-016, REQ-059, REQ-060, REQ-062.

## RFC-056 An admin writes integration skills, a project enables them, and the planner attaches them to cards

A skill is a written procedure for one kind of integration, such as OAuth 2.0: when to use it, the steps, the configuration it needs, its security rules, and its test checklist. Only an admin edits skills, with the same versions and revisions as rules. A Lead Developer member enables skills for a project.

The planner sees the project's enabled skills by slug and "use when" text. `create_card` accepts `skillIds`. Each id must be a skill enabled for the project, or the tool call fails and no card is stored. The card stores its skills. The implementer and reviewer packs carry the card's skills after the rules, under the same caps, markers, and trace event as rules.

A skill names steps and checks. It does not add a tool or a command. Check commands still come from the project settings (RFC-026).

A team that integrates OAuth twice writes the same security checklist twice, and the second card misses the step the first one learned.

Set aside: a `read_skill` tool that reads a skill on demand. It changes the implementer tool list. Also set aside: the planner choosing any skill in the catalog. A Lead Developer decides which integrations a project uses.

Consequences: `skills`, `project_skills`, and `card_skills` are new tables. The card JSON adds `skills`. The project conventions route also saves enabled skills.

Requirements: REQ-016, REQ-017, REQ-061, REQ-062.
