# RFC-0043: MCP mentions in requirements

Status: Accepted for implementation. Date: 2026-10-13.

This RFC adds decisions `RFC-101` and `RFC-102` under the new REQ-089. A requirement names the tribe's MCP servers it needs by writing `@slug`, and the jobs of its cards offer only those servers. It supersedes the RFC-092, RFC-094, and RFC-095 statements it names. Those statements stay in their files.

Sprints 64 and 65 in [../plan/sprints.md](../plan/sprints.md) implement this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-101 | `@slug` in a requirement's statement or acceptance lines mentions a tribe MCP server. Intake stores the mentions, lists the tribe's slugs to every member, and escapes mentions in the pull request body | RFC-092 "No other member sees the list", for the slug, the enabled flag, and the tool count only |
| RFC-102 | A card's implementer offers only the servers its requirements mention, or every enabled server when none mentions one. Refine does the same for the draft. A mention that resolves to nothing, or a requirement edited since acceptance, offers no MCP tools | RFC-092 "The implementer and refine profiles are their constant tool lists plus the project's tribe's allowlisted tools of enabled servers"; RFC-094 "At the start of an attempt the job loads the tribe's enabled servers"; RFC-095 "Otherwise refine offers the tribe's tools" |

## RFC-101 Mentions

### Problem

Every job of a tribe's projects gets every enabled MCP server. A requirement about the billing database still offers the ticket tracker and the design system, and a requirement that needs no outside data still offers a production database reader. The person writing the requirement knows which servers it needs, but has no way to say so, and cannot see the tribe's servers at all.

### Decision

- A mention is `@` followed by a slug, `[a-z][a-z0-9]{1,15}` as in RFC-092. The `@` is at the start of the text, or after whitespace or one of `( [ { " ' , ; :`. After the slug comes the end of the text, whitespace, one of `) ] } " ' , ; : ! ?`, or a `.` not followed by a letter or digit. So `a@b.co`, `@scope/pkg`, `@pytest.fixture`, and `@Override` are not mentions.
- Mentions are read from the markdown tree of the statement and of each acceptance line. Text in inline code, a fenced or indented code block, a link, an autolink, or raw HTML is not read.
- A requirement keeps its mentions in order of first use, without repeats, at most 16. They are stored with the requirement in `mentions_json`, by ingest for a repository requirement and by save for a manual one.
- The content hash adds the mention list when it is not empty. A requirement with no mention keeps its hash, so this change clears no acceptance. Wrapping a mention in backticks changes the hash and clears acceptance, as any text edit does (REQ-004, RFC-053).
- A mention of a slug the tribe does not have, or of a disabled server, is kept as written. The intake page warns about it and the save never refuses it.
- The intake response lists the tribe's servers to every member of the project as slug, enabled, and tool count. It never carries a URL, image, argv, connection, or secret name. The tribe server routes stay admin-only.
- The requirement form opens a list of the tribe's slugs when a person types `@`, and shows each mention under its field as known, disabled, or unknown.
- The pull request body wraps each mention in backticks, so a mention never notifies a GitHub user or team.

### Why

The author of a requirement is the person who knows which outside data it needs. Writing it in the text keeps it in the same reviewed, accepted, and hashed record as the rest of the requirement, so a repository requirement can carry it too. A tribe's slugs are names its admins chose; the connection details stay with the admins.

## RFC-102 Scope of a card's MCP servers

### Decision

- At the start of an attempt the job loads the tribe's enabled servers, then reads the mentions and acceptance of the card's requirements, scoped to the card's project.
- When no requirement of the card mentions a server, the attempt offers every enabled server, as RFC-094 did. A card with no requirement does the same.
- When any requirement mentions a server, the attempt offers only the enabled servers named by the union of the mentions. A requirement with no mention adds nothing to the union.
- When the mentions name no enabled server of the tribe, the attempt offers no MCP tools.
- When any requirement of the card is not accepted, because its text changed since acceptance, the attempt offers no MCP tools until it is accepted again.
- The redaction list still holds the secrets of every server of the tribe, offered or not.
- The `mcp_context` event adds `scope` (`all`, `mentioned`, or `none`), the mentioned slugs that did not resolve as `missing`, a `reason` for `none`, and each server's id. The event is written for scope `none` too, with no server.
- When the scope is `mentioned`, the implementer's card text adds one trusted line outside the untrusted markers: `This card's requirements name these MCP servers: <slugs>.` The system prompt does not change.
- Refine reads the mentions of the draft it is given, with no acceptance check, and offers the same scope. A draft whose mentions name no enabled server sends the RFC-087 request with no tools. The refine instructions add one sentence asking the model to keep each `@name` exactly as written.
- A mention never adds a server, a tool, or a disabled server. It only narrows the RFC-092 list.

### Why

Narrowing by mention keeps a job's outside access to what its requirement asked for. Failing closed when nothing resolves, or when the text changed since a person accepted it, keeps an edit or a renamed server from silently widening the scope back to every server.

## Set aside

- Mentioning a single tool, as `@slug.tool`.
- A display title for a server. Mentions use the slug.
- `@none` to mean no server at all. A requirement that needs none and shares a card with one that mentions a server already gets only the mentioned ones.
- Mentions in decision (RFC) records.

## Tests

Sprint 64: `TestMentionGrammar`, `TestIngestMentions`, `TestHashUnchangedWithoutMentions`, `TestManualRequirementMentions`, `TestIntakeMCPServers`, `TestImplementerMCPMentions`, `TestImplementerMCPUnion`, `TestImplementerMCPMentionMissing`, `TestImplementerMCPUnaccepted`, `TestRefineMCPMentions`, `TestPRBodyMentionsEscaped`, and the unchanged `TestImplementerMCPTools` and `TestMCPOtherTribe`.

Sprint 65: the mention helper, `RequirementForm` autocomplete and chips, intake pages, and trace Vitest cases.

Superseded in part by RFC-105 in [0045-layered-conventions-and-card-context.md](0045-layered-conventions-and-card-context.md). A card attach adds an enabled server of the tribe over the mention scope, and a card detach removes one.
