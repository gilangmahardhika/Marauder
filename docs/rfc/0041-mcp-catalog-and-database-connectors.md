# RFC-0041: MCP catalog and database connectors

Status: Accepted for implementation. Date: 2026-10-13.

This RFC adds decisions `RFC-096` to `RFC-098` under the new REQ-088. It supersedes earlier statements only where it names the id. Those statements stay in their files.

Sprints 60 to 62 in [../plan/sprints.md](../plan/sprints.md) implement this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-096 | A tribe admin adds an MCP server from a catalog: the built-in database connectors first, then a live search of the official MCP Registry | None |
| RFC-097 | Postgres and SQL Server are built-in connectors in the harness, read-only, to any host except the board database | RFC-092 "a transport `http` or `stdio`", which gains `postgres` and `sqlserver`; RFC-093 is restated as applying to `http` servers only; NG-001 "any credential that can reach production", for a read-only database login of a tribe connector only |
| RFC-098 | A registry entry maps to an `http` or `stdio` server, and the harness pulls and pins a stdio image | None |

## RFC-096 The MCP catalog

### Problem

Adding a server by hand means knowing its URL or image digest, its argv, and its secret names. A database server cannot be added at all: a stdio server has no network, and an HTTP server cannot reach a private address (RFC-093).

### Decision

- The tribe MCP page opens a catalog. A company admin and the tribe's admins search it, checked like the other tribe routes: `403`, then `404`.
- The built-in entries, Postgres and SQL Server, are listed first when they match the query.
- Then the harness searches the official MCP Registry at `https://registry.modelcontextprotocol.io/v0/servers`. The browser never calls the registry. The harness uses the `netguard` client, reads at most 512 KiB in 10 seconds, keeps only the latest version of each server, and caches each query and cursor for 10 minutes.
- When the registry cannot be reached, the catalog still lists the built-in entries with the fixed notice "The MCP Registry could not be reached".
- `Set up` opens the server form filled from the entry. The admin still enters the secrets, saves, discovers, and allowlists the read-only tools (RFC-092). A built-in entry fills its fixed tools.
- A registry description is shown to the admin only. It is never sent to a model.

### Why

A catalog turns setup into picking a server and typing its credentials. Keeping the registry call on the harness means one network path with one guard, and the allowlist stays the admin's review of what the model may call.

## RFC-097 Database connectors

### Decision

- `postgres` and `sqlserver` are transports. The harness connects itself; no container and no MCP process run. The connector answers the same list and call requests as an MCP server, so the allowlist, schema hash, redaction, call caps, trace, and refine rules of RFC-092 to RFC-095 apply unchanged.
- A connector has a host, a port, a database, a user, a TLS mode (`disable`, `require`, or `verify-full`), and a write-only `PASSWORD` secret. The host is a name or an IP, not a socket path and not a list. The admin confirms that the login can only read; a save without it is refused.
- The host may be any address, private and loopback included. `netguard` does not apply to a connector. A connector cannot reach the board database: at dial time the harness refuses an address, port, and database name that match `databaseUrl`. When the board is on this machine, every loopback and local interface address counts as the board's.
- Each connector offers three fixed tools with constant schemas: `query` with one SQL statement of at most 4000 characters, `list_tables` with an optional schema, and `describe_table` with a table and an optional schema.
- Postgres: the session sets `default_transaction_read_only`. Each call runs in `BEGIN READ ONLY` with a 15 second `statement_timeout` over the extended protocol, so one statement runs, and always ends with `ROLLBACK`.
- SQL Server: each call runs in a transaction that always ends with `ROLLBACK`. Before it runs, the statement is checked with strings and comments skipped: it is one `SELECT` or `WITH` statement, with no `INTO`, `EXEC`, `EXECUTE`, `OPENROWSET`, `OPENQUERY`, or `OPENDATASOURCE`. SQL Server has no read-only transaction, so the read-only login is the guard and the check is a second one.
- A result is a text table of at most 200 rows with each cell cut at 1 KiB, then the 64 KiB history cap of RFC-094. A database error is the tool's error text, cut at 300 bytes and redacted. A connect failure is the fixed text `mcp <slug>: the server is unreachable`.
- The password joins the job's redaction list, as every MCP secret does.
- An `http` server keeps the RFC-093 guard. A `stdio` server keeps no network.

### Why

Databases live on private networks, which the HTTP guard refuses on purpose. A connector in the harness can read a database without giving any container a network, and its own read-only transaction and fixed tools are easier to trust than a third-party server. The board refusal keeps one tribe from reading another tribe's cards through the harness's own database.

## RFC-098 Registry entries and image pull

### Decision

- A registry server with a `streamable-http` remote becomes an `http` entry. Its headers become secret names with their descriptions.
- Otherwise a server with an `oci` package and the `stdio` transport becomes a `stdio` entry, marked "Runs with no network" (RFC-093). Its environment variables become secret names, and its positional package arguments become the argv.
- `npm` and `pypi` packages, `sse` remotes, and URLs with template variables are listed but cannot be set up, with the reason.
- `Pull and pin image` asks the harness to pull an image reference with the sandbox engine, within 5 minutes, and returns it pinned by its `@sha256:` digest. The reference is checked against an image-reference pattern first. Fixture mode and a machine with no engine refuse it. Jobs still run the image with `--pull=never`.

### Why

The registry lists many servers the sandbox cannot run. Showing them with a reason is clearer than hiding them, and pinning the digest at setup keeps RFC-093's rule that a job runs exactly the image the admin chose.

## Set aside

- Other database engines, and write statements.
- Network for stdio servers.
- A company-wide catalog of approved servers.
- Installing `npm` or `pypi` packages.

## Tests

Sprint 60: `TestDBConfigValidation`, `TestPostgresToolReadOnly`, `TestPostgresSingleStatement`, `TestPostgresRowCap`, `TestDBConnectorRefusesBoard`, `TestDBPasswordRedacted`, `TestImplementerDBTools`, `TestSQLServerStatementCheck`, and `TestSQLServerLive` when `ORCH_TEST_MSSQL` is set.

Sprint 61: `TestMCPCatalogBuiltins`, `TestMCPCatalogRegistry`, `TestMCPCatalogRegistryDown`, `TestMCPCatalogAccess`, `TestMCPImagePull`.

Sprint 62: the catalog page, database form, registry prefill, image pull, and auto-discover Vitest cases.
