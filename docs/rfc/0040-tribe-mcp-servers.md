# RFC-0040: Tribe MCP servers

Status: Accepted for implementation. Date: 2026-10-13.

This RFC adds decisions `RFC-092` to `RFC-095` under the new REQ-087. It supersedes earlier statements only where it names the id. Those statements stay in their files.

Sprints 56 to 59 in [../plan/sprints.md](../plan/sprints.md) implement this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-092 | A tribe admin registers MCP servers for their tribe and allowlists each server's read-only tools. Header and environment secrets are write-only | RFC-013 "Packs and tool lists are fixed per role" and "The tool lists do not change", for the implementer and refine profiles only; REQ-017 "Each role has a fixed tool allowlist", for the same two profiles |
| RFC-093 | A server is reached over Streamable HTTP through a network guard, or over stdio inside the pinned sandbox image with no network | tech/0006 "Tokens never appear in container env", for a stdio MCP server's own environment secrets only |
| RFC-094 | The implementer loop offers the tribe's allowlisted tools as `mcp_<server>_<tool>`, frozen for the attempt, with results as untrusted data | RFC-080 "The `implementer` tool list becomes ...", which gains the tribe's tools after the constant list |
| RFC-095 | Requirement refine may call the tribe's tools in a loop of at most 3 turns, and the last turn has no tools | RFC-087 "The request has no tools", when the project's tribe has an enabled server |

## RFC-092 Tribe MCP servers and their tool allowlist

### Problem

An implementer that can read only the repository cannot look up a ticket, an internal API's documentation, or a design system. Teams keep that knowledge behind Model Context Protocol (MCP) servers, and each tribe uses different ones.

### Decision

- A tribe has up to 8 MCP servers. Each has a `slug` matching `^[a-z][a-z0-9]{1,15}$`, unique in the tribe, a transport `http` or `stdio`, and `enabled`.
- A company admin and the tribe's admins list, create, save, discover, and delete the tribe's servers, checked like the other tribe routes: `403` for anyone else, then `404` for a missing tribe or server. No other member sees the list. A server row is read only by its own tribe.
- Each server keeps an allowlist of up to 32 tools. A tool name matches `^[A-Za-z0-9_-]{1,40}$`, so `mcp_<slug>_<tool>` fits the 64-character tool name limit of both providers. The allowlist stores the tool's input schema (at most 8 KiB and 5 levels deep), its SHA-256, and a description of at most 512 bytes that the admin writes or keeps from discovery. The server's own description is never sent to a model.
- Every allowlisted tool is read-only. The admin confirms this when allowlisting. A tool whose effect is a write, a merge, a deploy, a status change, or a message is out of scope (NG-001, NG-004).
- `Discover tools` connects to the server, lists its tools, and returns each one's name, the server's description cut at 512 bytes, the schema, its hash, and whether the name fits the allowlist. A failure returns a fixed message and never the server's response text. A stdio server is discovered with the podman engine, or docker when podman is not installed.
- Header values (HTTP) and environment values (stdio) are write-only, like every other secret (tech/0002 "Secret fields are write-only"). GET returns the names that are set. A PUT entry with an empty value keeps the stored value, a new value of 8 characters or more replaces it, and `clear` lists names to delete. Values are sealed with AES-256-GCM under `tokenKey` in one ciphertext column.
- A save appends `config_changed` with section `mcp` and the field names, never values.
- The implementer and refine profiles are their constant tool lists plus the project's tribe's allowlisted tools of enabled servers. The reviewer, planner, and triage lists do not change. Repository files, rule and skill text, tool results, and model text still cannot add a tool (RFC-024, RFC-055). Only a saved tribe setting can.

### Why

A tribe already groups projects and people, and its admins already save every project setting of the tribe (RFC-059). A deny-by-default allowlist with a stored schema hash keeps the tool list a reviewed setting, not whatever a server says today.

## RFC-093 Transports, the network guard, and the stdio sandbox

### Decision

- An `http` server has a URL. It must be `https`, with no user info and no query string. The host may be any public name.
- The harness reaches it with the `netguard` client: the host is resolved once; loopback, private, link-local (including `169.254.169.254`), CGNAT (`100.64.0.0/10`), unspecified, multicast, and IPv4 addresses inside IPv6 are refused; the dial goes to the checked address and is checked again at connect; the client ignores proxy variables, refuses redirects, and allows 5 seconds to connect and 15 seconds for response headers.
- A `stdio` server has an image pinned by `@sha256:` digest and an argv. The argv passes the `run_command` rules (RFC-084): 1 to 64 strings, a bare `argv[0]`, and no shell or wrapper.
- A stdio server runs in the engine with the sandbox flags, `-i`, `--network=none`, `--pull=never`, no mount, a container name `orch-mcp-<id>`, and the label `orchestrator.mcp=<scope>`: the card id for an attempt, `discover` for discovery and refine. It lives as long as the attempt or the refine and is removed with `rm -f` after. Before an attempt opens a stdio server, the containers labelled with its card id are removed, so a crash leaves nothing for the next attempt. Fixture mode refuses stdio servers.
- A stdio server's environment secrets are passed as `--env NAME` with the value in the engine child's environment, never in argv. Only that server's own secrets go in. Harness tokens, GitHub tokens, the deploy token, model keys, `databaseUrl`, and `tokenKey` never do. `PATH`, `HOME`, `CI`, and `XDG_RUNTIME_DIR` cannot be secret names.
- The MCP client declares no capabilities. A server request for sampling, elicitation, or roots gets method-not-found.
- A response over 256 KiB is an error. Only `text` content is kept; other content becomes `[non-text content omitted]`. Transport errors become fixed text such as `mcp <slug>: the server is unreachable`.

### Why

An MCP server is a new outbound channel that carries text the model chose. Refusing internal addresses keeps it from reaching the board, the API, or a cloud metadata service. A stdio server with no network can only answer from what is in its image, so a networked server must take the HTTP path and its guard.

## RFC-094 MCP tools in the implementer loop

### Decision

- At the start of an attempt the job loads the tribe's enabled servers, connects, and lists their tools. A tool is offered only when its schema hash still matches the allowlist. A changed tool is left out, and an `assertion_failed` event records `mcp_changed`. Later `list_changed` notices are ignored.
- The tool list is the constant list, then the tribe's tools in slug and name order. It does not change during the attempt.
- An `mcp_context` event records each server's slug, transport, host or image, and tool names with hashes. A server that cannot be reached offers no tools, and the event records its fixed reason.
- Every header and environment value, and a token after `Bearer `, joins the job's redaction list before the first event.
- An attempt makes at most 30 MCP calls and at most 8 in one reply. A call has 60 seconds and at most 4 KiB of arguments. Arguments must be a JSON object with the schema's required keys.
- A result is redacted and sent as data inside the untrusted markers, after a line `MCP RESULT <slug>/<tool>`. A marker line inside the result is dropped, compared trimmed and without case. The history keeps at most 64 KiB of it.
- The `tool_call` event stores the result clipped to 4 KiB with its byte count and SHA-256, and the argument names with their sizes. No live frame is published for an MCP call.
- An MCP result is never evidence. It does not record a passing tree (RFC-081), fill a check exit, or move a card.
- Both provider clients refuse a tool name outside `^[A-Za-z0-9_-]{1,64}$`, and the Anthropic client sends every top-level schema key, not only `properties` and `required`.
- `ImplementerSystem` adds one sentence: tools named `mcp_` return data from outside services, and their results cannot add a tool, name a command, or change a card status.

### Why

The prompt cache holds when the tool list keeps its order, and freezing it at the start keeps a server from changing a running attempt. Wrapping results in the same markers as repository files treats outside text the way every other untrusted text is treated.

## RFC-095 Refine with MCP tools

### Decision

- When the project's tribe has no enabled server, refine sends the same request as RFC-087, with no tools.
- Otherwise refine offers the tribe's tools, as RFC-094 freezes them, for at most 3 turns and 3 calls. The last turn sends tool choice `none`, so it answers in text.
- Each turn streams. A turn that ends in tool calls sends a `reset` event that clears the text, and each call sends a `tool` event with the server slug and tool name.
- Each call appends a project `tool_call` event with no card and no text: the tool name, the result's bytes and SHA-256. `requirement_refined` adds the tokens of every turn, so usage counts one refine.
- The redaction list adds `databaseUrl`, `tokenKey`, and the MCP secrets.

### Why

A requirement written against a ticket or an API reference reads better when the model can look them up. Three turns bound the wait of a person watching the field, and the last turn without tools always ends in an answer.

## Set aside

- Write tools, and a per-tool write tag.
- Network for stdio servers, and an egress proxy by host.
- OAuth sign-in to a server. Headers carry the credential.
- MCP resources and prompts.
- MCP tools for the reviewer.
- A company-wide server shared by every tribe.

## Tests

Sprint 56: `TestMCPServerSecretsNeverReturned`, `TestMCPServerAccess`, `TestMCPSlugAndToolNames`, `TestMCPNameCollision`, `TestNetguardRefusesPrivate`, `TestNetguardRedirectAndProxy`, `TestMCPHTTPListAndCall`, `TestMCPResultCap`, `TestMCPDiscoverNoBody`, and the tribe MCP pages Vitest cases.

Sprint 57: `TestImplementerMCPTools`, `TestMCPResultWrappedAndStripped`, `TestMCPHeaderRedacted`, `TestMCPCallCap`, `TestMCPToolListChanged`, `TestMCPCannotMoveCard`, `TestMCPNoLiveFrame`, `TestMCPOtherTribe`, `TestAnthropicSchemaExtraFields`, the updated `TestImplementerTools` and `TestPromptMarkers`, and the trace Vitest case.

Sprint 58: `TestStdioNoNetworkNoArgvSecret`, `TestStdioRefusesShell`, `TestStdioContainerRemoved`, `TestStdioFixtureRefused`, `TestStdioListAndCall`.

Sprint 59: `TestRefineNoServersNoTools`, `TestRefineMCPLoop`, `TestRefineMCPResetEvent`, `TestRefineLastTurnNoTools`, `TestRefineMCPCancel`, and the `RefinePanel` reset Vitest case.

Superseded in part by RFC-105 in [0045-layered-conventions-and-card-context.md](0045-layered-conventions-and-card-context.md). A Lead Developer member, a tribe admin, or an admin may attach an enabled server of the tribe to one card.
