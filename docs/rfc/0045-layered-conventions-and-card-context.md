# RFC-0045: Layered conventions and card context

Status: Accepted for implementation. Date: 2026-10-13.

This RFC adds decisions `RFC-104` and `RFC-105` under the new REQ-092 and REQ-093. A tribe sets the conventions its projects start from, a project adds to or removes from them, and a person adds to or removes from one card's skills, rules, and MCP servers. It supersedes the RFC-055, RFC-056, RFC-092, and RFC-102 statements it names. Those statements stay in their files.

Sprints 69 to 71 in [../plan/sprints.md](../plan/sprints.md) implement this file. Sprint 68 regroups the sidebar for REQ-091 and needs no decision.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-104 | A tribe picks stacks and skills for all its projects and may switch company rules off. A project switches each stack, skill, and rule on or off on top of its tribe | RFC-055 "The rules that apply to a card are the global rules plus the rules of the project's stacks."; RFC-055 set aside "rule overrides per tribe or per project"; RFC-056 "a project enables them" and "A Lead Developer member enables skills for a project", for the tribe layer |
| RFC-105 | A person who may run a card attaches or detaches its skills and rules, and detaches its MCP servers. A Lead Developer, a tribe admin, or an admin also attaches MCP servers. The change applies from the next attempt | RFC-056 "The card stores its skills", as the only source of a card's skills; RFC-092 "Only a saved tribe setting can" add a tool, for a card attach by those roles; RFC-102 "A mention never adds a server, a tool, or a disabled server. It only narrows the RFC-092 list.", for a card attach |

## RFC-104 Layered conventions

### Problem

Every company rule applies to every project, and each project enables its skills alone. A tribe whose projects share a stack and a set of integrations repeats the same choices on every project, and cannot switch off a company rule that does not fit its work.

### Decision

- A company admin still writes every rule, stack, and skill. A disabled catalog item never applies at any layer.
- A tribe's conventions are stacks that every project of the tribe has, skills every project enables, and company rules switched off for the tribe. A company admin or a tribe admin saves them. Every tribe member reads them.
- A project's conventions switch each stack, skill, and rule `on` or `off`. A project with no row for an item takes the tribe's choice. The same callers as RFC-059 save them.
- A project's stacks are its detected stacks, plus the tribe's stacks, plus its own `on` stacks, minus its own `off` stacks.
- A project's rules are the enabled global rules plus the enabled rules of its stacks. A tribe `off` removes a rule; a project `on` or `off` decides it over the tribe.
- A project's skills are the tribe's skills plus its own `on` skills, minus its own `off` skills.
- Each item the API lists carries its origin: catalog, detected, tribe, or project.
- Existing enabled skills and stacks added by hand become project `on` rows, so no project loses a convention.

### Why

The tribe is the group that shares repositories, stacks, and integrations. Setting conventions once for the tribe keeps its projects aligned, and a project still says where it differs. Rules stay on by default so a new company rule reaches every project unless someone decides it does not fit.

## RFC-105 Card context

### Problem

The planner attaches skills when it creates a card, and nobody can change them afterwards. A person who sees an attempt miss an integration step, follow a rule that does not fit this card, or call an MCP server it does not need has no way to change what the next attempt reads.

### Decision

- A card's context is its rules, skills, and MCP servers. A card row `attach` adds an item and a `detach` removes it, over the project's conventions, the planner's skills, and the RFC-102 mention scope. Removing the row returns the item to what it inherits.
- A person who may run the card attaches or detaches a skill or a rule, and detaches an MCP server. Attaching an MCP server also needs a Lead Developer member, a tribe admin of the project's tribe, or a company admin.
- An attached skill or rule is an enabled catalog item. An attached MCP server is an enabled server of the project's tribe; its tools are still the allowlisted tools whose schema hash matches.
- A requirement of the card edited since acceptance still offers no MCP tools, whatever the card attaches.
- A `done` card's context does not change.
- An attempt reads the context once when it starts, as it reads rules and skills today. An AI review that starts later reads the context as it is then.
- Each change appends `context_changed` with the kind, the item, the mode, and the person. The `mcp_context` event lists the servers the card attached and detached, and its scope is `card` when they changed the set.
- A change never moves the card. Only `machine.Transition` writes status.

### Why

The person watching the card's attempts knows what the next one is missing. Rules and skills are text inside the untrusted markers and cannot add a tool, so any person who may run the card may change them. An MCP server reaches data with a tribe-shared credential, so adding one to a card stays with the people who may manage the tribe's work.

## Set aside

- Rules or skills written by a tribe. The company catalog stays the only place they are written.
- Attaching a single MCP tool instead of a server.
- Changing a card's context during an attempt.

## Tests

Sprint 69: `TestTribeConventionsSave`, `TestEffectiveConventions`, `TestProjectConventionsMigration`, `TestPackUsesEffective`, and the updated `TestProjectConventionsLeadOnly` and `TestCreateCardSkills`.

Sprint 70: `TestCardContextRoles`, `TestCardContextDone409`, `TestCardContextScoped`, `TestCardContextPack`, `TestMCPScopeCardAttach`, `TestCardContextDisabledServer`.

Sprint 71: the `CardContext` and trace Vitest cases.
