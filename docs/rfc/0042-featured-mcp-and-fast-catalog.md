# RFC-0042: Featured MCP servers and a catalog that answers at once

Status: Accepted for implementation. Date: 2026-10-13.

This RFC adds decisions `RFC-099` and `RFC-100` under REQ-088. It adds a featured list of well-known remote MCP servers to the catalog of RFC-096, and splits the registry search from the entries the harness already knows, so typing a query never waits on the registry. It supersedes the RFC-096 statements it names. Those statements stay in their files.

Sprint 63 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-099 | The catalog has a featured list of remote servers that work with a static header or none, shipped with the harness | RFC-096 "The built-in entries, Postgres and SQL Server, are listed first when they match the query", which now reads built-in and featured entries |
| RFC-100 | The catalog asks for local entries and one registry page separately. The registry fetch has 60 seconds, remembers a failure for 30 seconds, runs at most 4 at once, and stops when the browser gives up. Setting up a registry entry reads it by exact name | RFC-096 "reads at most 512 KiB in 10 seconds" |

## RFC-099 Featured servers

### Problem

An admin opening the catalog sees two database connectors and then whatever the registry returns for an empty query. The servers most teams want first, such as GitHub, Linear, or a documentation server, are hard to find: the registry's search matches names only, `github` matches tens of thousands of names, and lookalike entries rank next to the official ones.

### Decision

- The catalog lists a featured section: GitHub, Linear, Stripe, Context7, Hugging Face, Exa, Firecrawl, Microsoft Learn, AWS Knowledge, Cloudflare Docs, and DeepWiki. Each is an `http` entry with id `featured:<name>` and source `featured`.
- A featured entry uses an `https` URL with no query string that answers MCP at that exact address, and a header that takes a static value, or no header. Servers that only accept an OAuth sign-in are left out.
- GitHub and Linear use their read-only endpoints. The admin still discovers the tools and allowlists the ones jobs may call (RFC-092). A featured server's own tool descriptions reach the model only after the admin saves them.
- The list ships with the harness and changes only with a release. No admin edits it, so it is not the company-wide catalog RFC-0041 set aside.
- A featured entry is saved through the same checks as any server: the URL check on save and at dial, the header rules, and the 8-character minimum for a value. A secret marked required is a label only.
- The catalog page shows featured servers as tiles with a letter mark, labelled "Token needed", "Optional token", or "No sign-in" from the entry's secrets. It loads no remote image.

### Why

A short list of servers the harness is known to reach saves the admin from searching a slow registry for the common cases. Keeping it in the release keeps it reviewed like code, and the save path does not trust it more than a registry entry.

## RFC-100 Local entries apart from the registry

### Problem

The registry's search often takes 10 to 50 seconds, uncached. The catalog returned the built-in entries only after the registry answered or the 10-second fetch gave up, and the page dropped each answer that came after a newer keystroke, so a person typing saw an empty page and then a notice.

### Decision

- `GET /api/tribes/{id}/mcp-catalog` takes `source`: `local` lists the matching built-in and featured entries and never calls the registry; `registry` lists one registry page with `nextCursor`; no `source` lists both, as before. Any other value is `422` after the access check.
- The page sends both requests for each query, 300 ms after the last keystroke, and shows the local entries as soon as they arrive. The registry section shows "Searching the MCP Registry…" until its page arrives. A new keystroke aborts both requests of the previous query.
- The registry fetch has 60 seconds, including the wait for response headers, through its own copy of the netguard transport. MCP servers keep netguard's 15-second header limit.
- A failed registry query is remembered for 30 seconds and answered with the notice without a fetch. A query the browser abandoned is not remembered as a failure.
- At most 4 registry fetches run at once across the harness; a fifth waits until one ends or its request is abandoned.
- The cache keeps 10 minutes per query and cursor and drops expired pages before it drops everything when full.
- `?id=registry:<name>@<version>` returns the entry from a cached page, or else reads `GET /v0/servers/{name}/versions/{version}` by exact name. A 404 is `404`.
- A registry header whose name the save rule would refuse is not prefilled.

### Why

The entries the harness already knows should never wait on a network call. Aborting stale requests and remembering failures keeps a slow registry from piling up fetches while a person types, and the exact-name read makes Set up take seconds instead of another search.

## Set aside

- A local index of the whole registry. It is about 43,000 servers and 39 MB, and a full crawl takes about 20 minutes.
- OAuth sign-in for remote servers.
- Logos for featured servers.

## Tests

Sprint 63: `TestMCPCatalogBuiltins`, `TestMCPCatalogFeatured`, `TestMCPCatalogRegistrySlow`, `TestMCPCatalogEntryByName`, `TestMCPCatalogRegistryDown`, `TestMCPCatalogAccess`, and the catalog page Vitest case.
