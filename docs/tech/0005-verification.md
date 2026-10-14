# Verification

Status: Accepted for implementation. Date: 2026-10-10. Revised after review.

This is how you know the backend in [0003-backend.md](0003-backend.md) and the frontend in [0004-frontend.md](0004-frontend.md) match the contract in [0002-data-and-api.md](0002-data-and-api.md). The release bar is PRD REQ-043. Live model keys are not required for that bar. A fake provider replays scripts. A real dev server is a later run of the same health and deploy steps.

## Layout

```text
backend/
  internal/machine/transition_test.go
  internal/contextpack/testdata/reviewer.json
  internal/providers/fake/fake.go
  testdata/fixture-repo/          small git repo, enables fixture mode
  testdata/scripts/               fake provider transcripts
  testdata/bin/deploy-stub        copied to a temp dir outside the repo by the runner
  testdata/prd/fixture.md
  testdata/rfc/fixture.md
  cmd/fixture/main.go             REQ-043 runner
frontend/
  src/lib/**/*.test.ts            Vitest with @testing-library/svelte
```

Run backend tests from `backend/`:

```text
go test ./...
```

The fixture command:

```text
go run ./cmd/fixture
```

It exits `0` only when all nine REQ-043 steps pass. It does not call Anthropic or OpenAI. It creates a temporary data directory outside the repo and copies the deploy stub to a temporary directory with mode `0700`, so RFC-036 and RFC-038 hold during the run.

`go test` and the fixture need `ORCHESTRATOR_TEST_DATABASE_URL`, a PostgreSQL 16 or newer database (RFC-044). Each test migrates that database and isolates its rows. A missing variable fails the test with that name before any card is written. Tests do not use testcontainers and do not create a SQLite file.

## Fake provider

`internal/providers/fake` implements the provider interface. A script is a list of responses keyed by task class and attempt number. Each response is text, a tool call, or an error.

```json
{
  "plan": [
    { "tool": "create_card", "ref": "a", "title": "Hello", "requirementIds": ["REQ-001"], "risk": "low" },
    { "tool": "create_card", "ref": "b", "title": "World", "requirementIds": ["REQ-002"], "dependsOn": ["a"], "risk": "low" }
  ],
  "implement": [
    { "tool": "edit_file", "path": "hello.txt", "text": "ok\n" },
    { "tool": "commit", "message": "Make the check pass" }
  ],
  "review": [
    { "tool": "verdict", "value": "recommend_approve" }
  ],
  "triage": [
    { "text": "hello.txt did not contain ok" }
  ]
}
```

The fake records the system and user strings it received. Tests read that record. The reviewer record must contain the diff and must not contain `author-transcript`.

Transport failure is a script entry `{ "error": "transport" }`. The router test uses two entries: primary fails twice, fallback succeeds.

## Unit tests

| Test | Asserts |
| --- | --- |
| `TestEdgesMatchPRD` | The edge map equals the REQ-006 table, and `done` has no outbound edge |
| `TestIllegalDeploySkip` | `in_progress` to `deploying_dev` returns `ErrIllegal` and writes `transition_rejected` |
| `TestEvidenceRequired` | `approved` without a SHA returns `ErrEvidence` |
| `TestTransitionCAS` | Two transitions from the same read status: one wins, the other returns `ErrConflict` and writes no event or gate |
| `TestApprovedHeadMoved` | `approved` with a new head goes to `human_review` and clears the approval row |
| `TestRequestChangesFromApproved` | Request-changes on `approved` clears approval and writes an `approver` note |
| `TestRejectFromApproved` | Reject on `approved` blocks with `rejected` and runs no deploy |
| `TestCapFromChangesRequested` | `changes_requested` at the cap goes to `blocked` with `attempt_cap` |
| `TestUnblockDestinations` | `health`, `deploy_failed`, and `deploy_config` go to `human_review`. `turn_cap` goes to `ready` |
| `TestUnblockAtCap` | Release at the cap with the default policy raises `max_attempts` by one. One attempt runs, then the card blocks again |
| `TestUnblockReset` | `resetAttempts` sets `attempts_used` to 0. `attempts.n` keeps increasing |
| `TestUnblockSourceChanged` | `unblockTo` is null and unblock is `422` while a linked id is unaccepted |
| `TestReviewNotCounted` | A review and a triage call leave `attempts_used` unchanged |
| `TestRedact` | A payload containing the fixture deploy token is stored as `[redacted]` |
| `TestReviewerPack` | Fixture JSON has the diff and lacks `author-transcript` |
| `TestImplementerTools` | Tool list has `read_linked_file` and no `deploy`, no `verdict`, no linked write tool, and no `mcp_` name in the constant list |
| `TestFallbackOnce` | Primary transport error, one retry, one fallback, then `blocked` with `provider_error` if the fallback also fails |
| `TestRouteClass` | Low risk and zero attempts select `implement`. High risk or any used attempt selects `implement_hard` |
| `TestNoDeployClass` | `Decide` rejects task class `deploy` |
| `TestRouterConfig` | An empty `implement` model blocks the card with `router_config` |
| `TestPlanCards` | Planner cards link only accepted ids, get checks from settings, and a dependency cycle is refused |
| `TestDraftIds` | A PRD without `REQ-` headings sets `draftPending`, and drafted rows are unaccepted |
| `TestManualRequirementAdd` | A Developer member adds with a blank id after `REQ-001` and gets `REQ-002`, unaccepted. A typed `REQ-010` is stored. A duplicate id is `409`. A bad id or empty statement is `422`. A non-member is `404` |
| `TestStatementLimitAndLineEndings` | A 10000-character statement is `200` and 10001 is `422` on `statement`. CRLF and CR in a statement are stored as LF on add and on edit |
| `TestAcceptanceLineMarkers` | Acceptance lines lose one leading `- `, `* `, `+ `, `1. `, `2) `, `- [ ] `, or `- [x] ` marker, keep inline markdown, and turn an inner line break into a space. A marker-only line is `422`, and a 1001-character line is `422` with "Keep each acceptance line to 1000 characters or fewer." |
| `TestPRBodyMarkdownStatement` | In the pull request body each requirement id is bold on its own line with the markdown statement under it. A statement that leaves a code fence open gets its closing line before "Acceptance criteria", so `## Changes` is not inside code; a closed `~~~` fence is left alone |
| `TestAnthropicStream` | An `httptest` SSE server's two `text_delta` events reach `onText` in order, the response has the whole text and `end_turn`, the body has `stream: true`, and usage adds cache creation to input and reports the cache read as cached (RFC-087) |
| `TestOpenAIStream` | Two content chunks reach `onText` in order, the body asks for `stream_options.include_usage`, and the final usage chunk gives input without the cached tokens, the cached tokens, and output |
| `TestRoutedStreamFallsBackBeforeFirstToken` | A primary that fails before any text is called twice, then the fallback streams, and the response names the fallback model |
| `TestRoutedStreamNoRetryAfterToken` | A stream that fails after its first text returns `ErrProvider` at once: one primary call, no fallback, and only the first text was sent |
| `TestRoutedStreamRestartAfterToken` | `StreamRestart` with a primary that fails after its first text: the primary is tried again and the fallback answers, `onRestart` comes before each new reply's text, and the response is the fallback's text and model (RFC-088) |
| `TestHubSnapshotAndSlowSubscriber` | A viewer that joins mid-reply gets the turn and its text so far; `reset` and `turn_end` change what the next viewer gets; a viewer that falls 256 frames behind is dropped and its channel closed; the live text stops at 64 KiB on a character boundary; `Close` ends every viewer and refuses new ones. `TestHubCaps` refuses the 17th viewer of a card and the 65th in all (RFC-088) |
| `TestLoopStreamsTurnText` | An implement job with a publisher sends `turn`, the redacted reply text, and `turn_end` for each model call, and each `turn_end` comes after its `model_call` event is stored (RFC-088) |
| `TestReviewStreamsText` | A review job with a publisher sends each reviewer reply as `turn` with role `reviewer`, deltas, and `turn_end` after its event is stored (RFC-088) |
| `TestTraceStreamFrames` | The trace stream replies `text/event-stream` with `no-store` and `X-Accel-Buffering: no`: a `trace` frame with the stored events, attempts, and `live`, then the hub's `turn` and text so far, then new deltas. After `turn_end` a `trace` frame has only the new `model_call`. A ping comment arrives. When the attempt ends, a `trace` frame with `live: false` and `done`, then the stream ends. A card that is not live gets `trace` and `done` at once. `TestTraceStreamReconnectAndShutdown` ends with `done` and `reconnect` after the time limit, and closing the hub ends an open stream (RFC-088) |
| `TestTraceStreamOtherProcessEvents` | An event appended straight to the database while a stream is open, as a `run-next` process would, arrives in a `trace` frame without the events already sent (RFC-088) |
| `TestUsageAggregates` | Implementer, reviewer, and refine calls on three cards in two projects and three models, read for October 1 to 3, sum to 6 calls and the right input, output, and cached tokens, with calls by day. A `tool_call` with tokens and calls a second before and after the range are left out, and grouping by model puts the largest total first (RFC-089) |
| `TestUsageTaskClass` | A payload `taskClass` wins; an old implementer row takes the attempt's `implement_hard`; an old reviewer row is `review`; a refine is `refine_requirement`; a call with no attempt is `implement` |
| `TestUsageUser` | Card calls count against the card owner and a refine against its `actor_id`, in the refine's project. A card with no owner groups as "No owner", and a user with no name by email |
| `TestUsageFamily` | Each model takes its family and protocol from `config.yaml`; a model no family lists has neither and groups as "Unknown family" |
| `TestModelCallTaskClass` | After an implement job and a review, every implementer `model_call` carries `taskClass` `implement` and every reviewer one `review` |
| `TestUsageAdminOnly` | `/api/usage` without a bearer is `401`, for a Lead Developer who is not an admin is `403` even with a bad range, and for an admin is `200` with the call's model, task, and tokens and without its reply text |
| `TestUsageRange` | No dates read the 30 days ending today with `rows: []`. `from` after `to`, 367 days, and a bad `from` are `422` on `from`; a bad `to` is `422` on `to`. 366 days is `200` |
| `TestUsageTribeScope` | With Gadget in its own tribe, the Gadget tribe reads only Gadget, including a Gadget card's event stored with `proj_default`; the default tribe reads only Default; the company view puts every card call in its card's project; an unknown tribe reads nothing (RFC-090) |
| `TestUsageSelfScope` | Ben's scope has only his rows: his card's call and two refines, in projects of both tribes. Ana's calls and the card with no owner are left out; an unknown user reads nothing |
| `TestTribeUsageAccess` | `/api/tribes/{id}/usage` without a bearer is `401`; a plain member of the tribe and a tribe admin of another tribe are `403`; an admin on a missing tribe is `404`; the tribe admin with a bad range is `422`; the tribe admin and an admin get only the tribe's project and no reply text |
| `TestMyUsage` | `/api/me/usage` without a bearer is `401`. A member who is not an admin gets their owned card call and their refine, in two projects, every row theirs, with no other user's email and no reply text. A bad `to` is `422`. An admin with no calls of their own gets no rows |
| `TestUsageSourceAndCard` | Refine rows have source `intake` and no card; card rows carry the card's id and title. By source, intake has 1 call and card 5; by card, the three cards stay apart by `title (id)` and the refine is "Intake" (RFC-091) |
| `TestRefineFieldStreams` | A statement refine replies `text/event-stream` with `start` naming the model, at least two deltas that join to the text, then `done` with the text and no acceptance. One request with task class `refine_requirement`, 4,096 max tokens, and no tools. An accepted manual row is unchanged, and one `requirement_refined` event has the id, `field`, model, and tokens, and no text (RFC-086, RFC-087) |
| `TestRefineFieldAcceptanceLines` | An acceptance refine drops list markers and blank lines in `done` and uses `RefineAcceptanceSystem`. 51 lines or no lines end with an `error` event and no `done` |
| `TestRefineFieldRedactsSplitKey` | A model key split across two deltas never reaches the stream, whole or in part, and the text reads `[redacted]`. A provider error before text is `502` JSON and one after text is an `error` event, neither with the key |
| `TestRefineFieldErrorsBeforeStream` | No field, an unknown field, an empty statement, or guidance over 1,000 characters is `422` JSON. No `implement` route is `409 router_config` JSON naming the route. A user outside the tribe gets `404` and no session `401`. None sends a request |
| `TestRefineFieldCancel` | Closing the connection after the first delta stops the provider stream, and no `requirement_refined` event is written |
| `TestMCPServerSecretsNeverReturned` | Create, save, list, and the `config_changed` event carry secret names only; an empty value keeps the stored one and `clear` drops it (RFC-092) |
| `TestMCPServerAccess` | A member who is not a tribe admin gets `403` before `404`; another tribe's server is `404`; a company admin and the tribe admin pass |
| `TestMCPSlugAndToolNames` | Bad slugs, tool names with a dot or over 40 characters, a deep or large schema, a non-https or private URL, an unpinned image, and a shell argv are `422` with field errors |
| `TestMCPNameCollision` | A second server with the same slug in the tribe is `422`; the same slug in another tribe saves; a ninth server is refused |
| `TestMCPDiscoverNoBody` | A server that answers with an error body returns `{ok:false}` with a fixed message and never the body text |
| `TestNetguardRefusesPrivate` | `127.0.0.1`, `169.254.169.254`, `10.0.0.5`, `100.64.1.1`, `::ffff:127.0.0.1`, and a name that resolves to a private address are refused at check and at dial (RFC-093) |
| `TestNetguardRedirectAndProxy` | A redirect is refused and `HTTPS_PROXY` is ignored |
| `TestMCPHTTPListAndCall` | Against an `httptest` server: initialize keeps the session id, tools page, a call returns text with non-text content as `[non-text content omitted]`, and an SSE reply answers a server request with `-32601` |
| `TestMCPResultCap` | A reply over 256 KiB is `ErrTooLarge` |
| `TestImplementerMCPTools` | A tribe's allowlisted tool is offered last as `mcp_tickets_lookup`, the system prompt has the MCP sentence, the result comes back wrapped after `MCP RESULT tickets/lookup`, and `mcp_context` and the `tool_call` event with `bytes`, `sha256`, and server and tool args are written (RFC-094) |
| `TestMCPResultWrappedAndStripped` | Marker and `MCP RESULT` lines inside a result are dropped, compared trimmed and without case; missing required or oversized arguments never reach the server; an unknown or colliding name is not offered |
| `TestMCPHeaderRedacted` | The Authorization value reaches the server, but neither the model's history nor any event holds the token |
| `TestMCPCallCap` | 8 calls per reply and 30 per attempt; the next is a tool error and the server is not called |
| `TestMCPToolListChanged` | A live schema whose hash differs from the allowlist is not offered and writes `assertion_failed` `mcp_changed`; `mcp_context` reads `1 MCP servers, 0 tools` |
| `TestMCPCannotMoveCard` | A result that claims approval and asks for commit and deploy leaves no commit, no push, and no move to `pr_open` |
| `TestMCPNoLiveFrame` | No live frame carries the MCP call or its result |
| `TestMCPOtherTribe` | Another tribe's server is never opened and its tools are not offered |
| `TestMCPStdioInFixture` | A stdio server in fixture mode offers no tools and `mcp_context` records `stdio servers need the container sandbox` |
| `TestAnthropicSchemaExtraFields` | `additionalProperties` and `$defs` reach `input_schema`; `ToolChoice` `none` sends `{"type":"none"}`; a bad tool name is refused before sending. `TestOpenAIToolChoiceNone` and `TestOpenAIRefusesBadToolName` do the same on OpenAI |
| `TestStdioNoNetworkNoArgvSecret` | The engine argv has `-i`, `--pull=never`, `--network=none`, the name and label, the sandbox limits, no mount, and `--env NAME` with the value only in the engine's environment; reserved names, an unpinned image, and a bad name or label are refused (RFC-093) |
| `TestStdioRefusesShell` | A shell, a wrapper, a path, and an empty argv are refused; `execx.Start` refuses a shell |
| `TestStdioContainerRemoved` | Close runs `rm -f <name>` once; a failed start removes it too; `SweepMCP` removes the containers its label lists |
| `TestStdioFixtureRefused` | Fixture mode refuses before anything starts |
| `TestStdioListAndCall` | Through a fake engine: initialize, list, and call over line-delimited JSON-RPC, a server request gets `-32601`, the environment secret reaches the server and not the argv, and a closed client stops answering |
| `TestStartFrames` | `execx.Start` reads one line per frame and refuses a line over the cap |
| `TestRefineNoServersNoTools` | With no server, or only a disabled one, refine sends one request with no tools and no tool choice, and writes no `tool_call` (RFC-095) |
| `TestRefineMCPLoop` | A tool turn then a text turn: `tool` names the server and tool, the result is wrapped, the header reaches the server but no event or stream holds it, `requirement_refined` sums both turns' tokens, and the project `tool_call` has no card and no text |
| `TestRefineMCPResetEvent` | Two tool turns each end with `reset` after their text, and a client that clears on `reset` shows only the final text |
| `TestRefineLastTurnNoTools` | The third turn sends tool choice `none` with the tools still listed and its text is the answer; a fourth call in one reply is refused and the next turn has tool choice `none` |
| `TestRefineMCPCancel` | Closing the connection during a call ends it, records it as failed, sends no second model request, and writes no `requirement_refined` |
| `TestMentionGrammar` | `intake.Mentions` keeps `@tickets`, `(@db)`, and `@db.` at the end of a sentence; drops `a@b.co`, `@scope/pkg`, `@pytest.fixture`, `@Override`, `@x`, a slug over 16 characters, and mentions in inline code, a fence, an indented block, link text, an autolink, and raw HTML; keeps first-use order without repeats and at most 16 (RFC-101) |
| `TestIngestMentions` | A parsed requirement stores the mentions of its statement and acceptance items; wrapping one in backticks on re-ingest drops it and clears acceptance |
| `TestHashUnchangedWithoutMentions` | A requirement with no mention has the same hash as before mentions existed, and one with a mention does not |
| `TestManualRequirementMentions` | A manual add stores the statement's and acceptance lines' mentions, an edit that removes one stores the new list and clears acceptance, and the intake response lists `mentions` |
| `TestIntakeMCPServers` | A plain member's intake lists every tribe server with `slug`, `enabled`, and `toolCount`, and the body holds no URL, argv, image, config, or secret name; another tribe's server is not listed |
| `TestImplementerMCPMentions` | With two enabled servers, a card whose requirement mentions `@tickets` is offered only `mcp_tickets_*`, and `mcp_context` has scope `mentioned` and the trusted line names `tickets`; the same card with no mention is offered both, with scope `all` (RFC-102) |
| `TestImplementerMCPUnion` | A card with two requirements, one mentioning `@tickets` and one `@docs`, is offered both servers; a third requirement with no mention adds nothing |
| `TestImplementerMCPMentionMissing` | A requirement mentioning only `@nothere` and a disabled server offers no MCP tools, opens no server, and writes `mcp_context` with scope `none` and both slugs in `missing` |
| `TestImplementerMCPUnaccepted` | A card whose requirement's acceptance was cleared offers no MCP tools and writes `mcp_context` with scope `none` and a reason naming the requirement |
| `TestRefineMCPMentions` | A draft mentioning `@docs` offers only that server's tools; a draft mentioning only an unknown slug sends the request with no tools |
| `TestPRBodyMentionsEscaped` | The pull request body writes a statement's and acceptance line's `@tickets` as `` `@tickets` `` and leaves `a@b.co` and code alone |
| `TestDBConfigValidation` | A database server with a URL, an unconfirmed read-only login, and a header secret is `422` on `transport`, `config.readOnlyLogin`, and `secrets`; one with no config is `422` on `config`; a good one saves with `PASSWORD` only, lists its config without the password, records `config` in the changed fields, and keeps the password out of `config_json` (RFC-097) |
| `TestDBConfigCheck` | A name, a private IP, loopback, `::1`, and `localhost` pass; a socket path, a list, a space, a backslash instance, and user info are refused, and each missing field is named |
| `TestPostgresToolReadOnly` | `INSERT`, `UPDATE`, a `DELETE` inside `WITH`, and `nextval` through `query` return a tool error, the insert with SQLSTATE `25006`, and change no row; a `SELECT` answers |
| `TestPostgresSingleStatement` | `SELECT 1; DROP TABLE` and `COMMIT; SET transaction_read_only = off` are refused, nothing is written, and the connection still answers |
| `TestPostgresRowCap` | A result has 200 rows and a line saying more were cut; a long cell is cut at 1 KiB; `list_tables` and `describe_table` answer; the three tools are listed |
| `TestDBConnectorRefusesBoard` | A connector to the board's address, port, and database, and to `localhost` with the database name in upper case, is `ErrBlocked` with the fixed text; a closed port is unreachable |
| `TestDBPasswordRedacted` | A query that builds the password with `chr()` returns it redacted: neither the model nor any event holds it. Skipped when the test database has no password |
| `TestImplementerDBTools` | An implement attempt is offered `mcp_pg_describe_table`, `mcp_pg_list_tables`, and `mcp_pg_query` last, reads two rows wrapped after `MCP RESULT pg/query`, gets a read-only error for a `DELETE` that leaves both rows, and `mcp_context` carries the transport, host, and database |
| `TestSQLServerStatementCheck` | One `SELECT` or `WITH`, with keywords inside strings, bracket and double-quoted names, and comments, passes; `INTO`, `EXEC`, `OPENROWSET`, `OPENQUERY`, a write, `WAITFOR`, `DECLARE`, a second statement, an unclosed string or comment, and a statement over 4000 characters are refused |
| `TestSQLServerLive` | Runs only when `ORCH_TEST_MSSQL` names a server: a `SELECT` answers, a `CREATE` is refused, and `list_tables` answers |
| `TestMCPCatalogBuiltins` | `Local("")` lists Postgres then SQL Server with their ports, `PASSWORD`, and tools, then every featured entry; `mssql`, `Postgres`, and `database` match only the database entries, `tickets` only Linear, `payments` only Stripe, and `nothing` none; no local search calls the registry; a later page has no local entry; `Entry` finds one by id (RFC-096, RFC-099) |
| `TestMCPCatalogFeatured` | Every featured entry has a unique `featured:` id, is a supported `http` entry whose URL passes `CheckURL` with the strict policy and has no template, an `https` website, non-null lists, and only an `Authorization` or `x-api-key` secret, and `LocalEntry` finds it; GitHub and Linear use `/readonly` and GitHub requires its token; `Entry` returns a featured entry while the registry fails (RFC-099) |
| `TestMCPCatalogRegistrySlow` | A registry that never answers fails at the catalog's `Timeout` with `RegistryError` and no entries; the same query then answers the notice at once without a second fetch; a query whose context was already cancelled is not remembered and fetches again later (RFC-100) |
| `TestMCPCatalogEntryByName` | `Entry` of a `registry:` id not in the cache reads `/v0/servers/io.example%2Ftickets/versions/2.0.0`, keeps the `Authorization` header and drops `API_KEY`; another name is `ErrNotFound`, and so is an unknown `featured:` id (RFC-100) |
| `TestMCPCatalogRegistry` | Against an `httptest` registry: the query carries `search`, `limit=30`, and `version=latest`, and the next cursor comes back; an old version is dropped; an http entry has its URL and header secrets; an oci stdio entry has its image, argv, environment secrets, and the no-network note; npm, sse, and a templated URL are unsupported with a reason; a page is cached for 10 minutes; `Entry` finds an entry and misses one (RFC-098) |
| `TestMCPCatalogRegistryDown` | A loopback registry the netguard policy refuses, a 500, a body over 512 KiB, and bad JSON each list the built-in and featured entries with `RegistryError`, and `Registry` alone returns no entries with the notice |
| `TestMCPCatalogAccess` | A member who is not a tribe admin and a user outside the tribe get `403` on the catalog and the pull, a tribe admin on another tribe `403`, a missing tribe `404`; a tribe admin gets the local entries then the registry entry, `source=local&q=github` lists GitHub first without a registry call, `source=registry` lists only the registry entry, another `source` is `422` for the admin and `403` for a member, `?id=featured:github` has the read-only URL, and a missing id is `404`. With no catalog the local entries come with the notice, `source=local` has no notice, `source=registry` has only the notice, and a featured id still resolves (RFC-096, RFC-100) |
| `TestMCPImagePull` | Through a fake engine the pull runs `pull` then `image inspect` and returns the digest whose repository matches; a flag, a space, upper case, a `;`, and an empty reference run nothing; a failed pull, fixture mode, and no engine are `422` on `image` |
| `TestManualRequirementEdit` | An edit of an accepted manual row clears acceptance. Editing or deleting a parsed row is `409`. Deleting a manual id a card lists is `409`. Deleting an unused manual row removes it |
| `TestIntakeLinkedCards` | A card naming `REQ-001` is listed in that requirement's `linkedCards`. A card in another project naming `REQ-001` and `REQ-002` is not listed, and `REQ-002` has `[]` |
| `TestIngestKeepsManual` | A manual `REQ-001`, then ingest of a PRD that defines `REQ-001`. The row stays manual with its text, and `Skipped` is `[REQ-001]` |
| `TestSecretOmitted` | GET of the company GitHub, LLM, and project deploy settings has no token or key substring from the secrets file |
| `TestModelSettingsAdminOnly` | A Lead Developer who is not an admin reads LLM and router settings, and gets `403` on PUT and on a family test. An admin saves them, and no response has the saved key |
| `TestSecretKeep` | A project deploy PUT without the token leaves that project's `secrets.json` bytes unchanged |
| `TestUnknownModel` | Router PUT with model id `missing` returns `422` and the yaml is unchanged |
| `TestModelIdUnique` | The same model id in two families is `422` |
| `TestBotApprover` | An approval by a user whose GitHub login is the company bot login returns `422` |
| `TestOwnerCannotApprove` | The card owner's `actions` has no `approve`, and an approval POST by the owner is `403` with no approval row (RFC-054) |
| `TestSoloOwnerApproves` | With Solo approval off the owner is `403`. After `soloApproval: true` is saved, the owner, whose login is the bot login, sees `approve` and approves, with `independent = false` (RFC-067) |
| `TestSelfReviewNeedsLead` | A Developer's add with `selfReview: true` is `403`. The creator, as a Developer, is `403` on the self-review route. A Lead Developer's add stores the flag, and an admin turns it on and off. An unknown id is `404` (RFC-070) |
| `TestSelfReviewOwnerApproves` | With Solo approval off, a card that lists a flagged and an unflagged requirement gives the owner no `approve`, and the owner's POST is `403`. With only the flagged requirement the owner sees `approve`, approves to `approved`, and the row has `independent = false` |
| `TestCardSelfReview` | `app.CardSelfReview` is true only when every listed requirement is flagged. Mixed, none flagged, no requirements, or a missing id is false |
| `TestAIReviewModeNeedsLead` | A Developer's add with `aiReviewMode: manual` is `403`, and the Developer's add without it returns `continue`. The Developer is `403` on the ai-review-mode route. An unknown value is `422` on add and on the route. A Lead Developer's add stores `manual` and the route sets it. An unknown id is `404`. The card JSON says `continue` with no requirements and `manual` once the card lists a manual one (RFC-072) |
| `TestCardAIReviewMode` | `app.CardAIReviewMode` is `manual` when any listed requirement is manual. No manual requirement, no requirements, or an unknown id is `continue` |
| `TestReviewContinueQueuesRetry` | `request_changes` on a `continue` card reaches `changes_requested` with one open `retry` row and a comment saying the card went back. At the cap it is `blocked` with `attempt_cap` and has no row (RFC-071, RFC-072) |
| `TestReviewManualReachesHumanReview` | `request_changes` on a `manual` card with green checks reaches `human_review` with the verdict, one `review` note, no `retry` row, and a comment saying a person decides. Pending checks stay in `agent_review` until a refresh sees green. A red check reaches `changes_requested` with no `retry` row |
| `TestRefreshSendsBackWhenNoLongerManual` | A `manual` card waiting in `agent_review` with `request_changes` whose requirement turns `continue` is sent to `changes_requested` with one `retry` row on the next refresh |
| `TestRetryAfterNoCommit` | An implement job that ends `no_commit` with attempts left leaves the card `in_progress` with one open `retry` row. At the cap it is `blocked` with `attempt_cap` and has no row (RFC-071) |
| `TestRetryAfterChecksFailed` | An implement job whose gating check fails with attempts left leaves the card `in_progress` with one open `retry` row |
| `TestRunRefusedAtCap` | A card at the cap is not startable, and `QueueCardRun` returns `ErrNotRunnable` without a row |
| `TestSetInstructions` | Saving trims the text and turns CRLF into LF, writes one `instructions` note and one `instructions_saved` event with the actor and `length`. The same text again writes nothing. 4001 characters is invalid on `instructions` and 4000 is not. Blank text clears them. Another project is not found and a `done` card is `ErrCardDone` (RFC-073) |
| `TestRunWithInstructions` | Run with too-long instructions writes neither the instructions nor a `runs` row. Run with instructions saves them and queues one row. Run without them leaves them. A refused run on a `backlog` card saves nothing |
| `TestAttemptReadsInstructionsAndNote` | With a review note and instructions saved, the implementer prompt carries the note and the instructions block, and the system string names the block. With the instructions cleared, the prompt keeps the note and has no block |
| `TestInstructionsRoute` | A Developer sees `instructions` in `actions` and `instructions: null`. PUT saves and returns the text with the note listed. Over 4000 characters is `422` on `instructions`. POST is `405`. Run with a non-string is `422`, and Run with text queues and saves it. The board list does not carry the text. A blank PUT returns `null`. A `done` card lists no `instructions`, and PUT is `409` |
| `TestInstructionsRole` | A Product Owner does not see `instructions`, and the PUT is `403` |
| `TestApprovalPostsCommentAndStatus` | An approval posts one comment on the pull request and one `success` status with context `orchestrator/approval` on the SHA, with the company token in `GH_TOKEN` and not in argv. A repeat approval of the same SHA posts nothing (RFC-068) |
| `TestApprovalAnnounceFailureKeepsApproved` | When `gh` fails, the approval is still `200` and `approved`, with one `job_error` for the comment and one for the status |
| `TestRequestChangesWithdrawsApprovalStatus` | Request changes on an approved card posts `failure` on the approved SHA |
| `TestRequestChangesAtCapBlocks` | Request changes on a card out of attempts is `blocked` with `attempt_cap` and keeps the approver note (RFC-077) |
| `TestCIRedQueuesRetry` | A red check with attempts left reaches `changes_requested` with one `ci` note and one open `retry` row |
| `TestReviewManualReachesHumanReview` red checks | A `manual` card with a red check reaches `changes_requested` with no `retry` row, and out of attempts it is `blocked` with `attempt_cap` |
| `TestCIRedAtCapBlocks` | A red check on refresh for a card out of attempts is `blocked` with `attempt_cap` and has no `retry` row |
| `TestRefreshCapsChangesRequested` | Refresh blocks a `changes_requested` card out of attempts with `attempt_cap` and leaves one with attempts left |
| `TestAdvancerRefreshesChangesRequested` | A worker tick starts one refresh for a `changes_requested` card |
| `TestRequestChangesReason` | A 4001-character reason is `422` with `fields.reason`, and the card stays `human_review` with no note. A reason is stored trimmed with LF line ends as the newest `approver` note with `attemptN` 0. An empty body stores `changes requested` |
| `TestReviewIgnoresApprovalStatus` | A pending `orchestrator/approval` status on the head still moves the card to `human_review`, with `ci_required = false` |
| `TestCardDiff` | On a real repository the diff lists each file with counts and hunks, redacts a planted model key, cuts a file past 1,500 lines, and returns not found for another project |
| `TestTraceOrderAndAttempts` | Three events appended in the same second, with ids that sort the other way, come back in insert order. `attemptId` is left out when empty. `attempts` lists the attempt's `n`, model, tokens, outcome, commit, and end, with `reviewIndependent` null. The card under another project is `404` |
| `TestJobErrorCapped` | A 1.5 MiB message with NUL bytes and invalid UTF-8 is stored under 10 KiB as valid UTF-8 without NUL, with its head, its tail (redacted), and the cut marker |
| `TestTraceClip` | `trace.Clip` drops NUL, redacts a secret holding `&`, keeps valid UTF-8 and the head and tail around the cut marker, and redacts a secret that spans the cut (RFC-074) |
| `TestModelCallText` | A model reply beside a tool call is stored as `model_call.text`, with the `databaseUrl` from `JobDeps.Secrets` redacted, cut under 4 KiB with its head and tail, as valid UTF-8. A reply with no text stores no `text` |
| `TestToolCallArgs` | Each tool row stores its `args`: `path` for list, read, and edit, with `glob`, `depth`, `offset`, and `limit` when set, `text` for search, byte counts for `replace_in_file`, `from` and `to` for `move_file`, `command` for the local check, and the commit `message` redacted. `edit_file` stores `bytes` and `lines`, and no event holds the file text |
| `TestReadFileRange` | `read_file` with an offset and a limit returns numbered lines, the range and the total, and the next line to read. An offset past the end is an error (RFC-080) |
| `TestReplaceInFileUnique` | Text that occurs twice is refused and leaves the file. A unique match and `replaceAll` replace, keep the file mode, and leave no temporary file |
| `TestReplaceInFileNotFound` | No match, an empty `old`, `old` equal to `new`, and a placeholder in `new` are refused, and the file is unchanged |
| `TestEditRefusesPlaceholder` | `edit_file` refuses both history placeholders and empty text over a non-empty file. An empty new file is written |
| `TestHistoryDropsEditText` | The history sends an earlier `edit_file` call with `bytes` and `lines` and no `text` |
| `TestSearchRegexGlobContext` | `search_files` matches RE2 with a glob, ignores case, adds context lines, and skips `node_modules` |
| `TestListGlobDepth` | `list_files` filters by a `**` glob and a file-name glob, stops at a depth, and skips `vendor`, `dist`, and `node_modules` below the start |
| `TestDeleteMoveSafePath` | `move_file` refuses an existing target, `..`, and `.git`. `delete_file` refuses a symlink and a path outside the worktree |
| `TestToolErrorsFlagged` | A failed tool, a tool outside the profile, and bad JSON arguments return `error: `. The `anthropic` client sends that result with `is_error: true` |
| `TestToolResultsRedacted` | A secret in a file reaches the model's next request as `[redacted]` in read and search results |
| `TestHistoryBudgetPerModel` | A model's `historyBudgetKiB` sets how much earlier tool output is sent. Zero means 16 KiB, and a value outside 4 to 512 does not save (RFC-081) |
| `TestHistoryStableEviction` | Results are replaced by a placeholder oldest first until the history fits three quarters of the budget, and a replaced result stays replaced on later turns |
| `TestCheckOutputKeepsTail` | A cut `run_local_check` result keeps its last lines |
| `TestRereadUnchanged` | A second read of an unchanged file returns `unchanged since turn 1`. A read after an edit returns the new text |
| `TestTextReplyNudged` | A reply without a tool call gets a note asking for one and the attempt goes on. Three such replies end with `no_commit` after two nudges |
| `TestTruncatedReplyNudged` | A reply cut at `max_tokens` gets a note saying it was cut at the token limit |
| `TestAutoCommitAfterPassingCheck` | A loop that ends after a passing check commits that tree with the card title, the `Committed-By` trailer, the card trailer, and a `tool_call` with `auto: true` |
| `TestNoAutoCommitOnDirtyTree` | An edit after the passing check, or a failing check, ends with `no_commit` and no harness commit |
| `TestBadToolJSONReturnedToModel` | Arguments that are not valid JSON return `error: the arguments were not valid JSON (...)` and the attempt goes on |
| `TestTurnCapStops` | Every turn spent without a commit blocks with `turn_cap`, adds one to `attempts_used`, ends the attempt with `budget`, and appends one `budget_stop` with the turns used |
| `TestTokenCapStops` | A card over `maxTokensPerCard` stops before the next model call and blocks with `token_cap` and one `budget_stop` |
| `TestInfraErrorKeepsAttempt` | A gating engine exit 125 writes a `job_error` and blocks with `sandbox_config` with `attempts_used` 0. After unblock the passed commit is gated and published without a model call, and counted once |
| `TestProviderErrorKeepsAttempt` | A provider that always fails blocks with `provider_error` with `attempts_used` 0 |
| `TestPackPathMap` | The path map lists files to 4 levels, shows a deeper directory as `dir/ (N files)`, skips `node_modules`, `.git` (a directory or a worktree's file), and `.orchestrator`, drops its depth to fit 12 KiB, and is cut with `truncated: true` when even one level does not fit. `list_files` does not list a worktree's `.git` file (RFC-082) |
| `TestPackRepoMemory` | `AGENTS.md` and `CLAUDE.md` from the root and from the directory the card title names go inside the markers, with marker lines stripped; a directory the card does not name is not read; a 22 KiB `CLAUDE.md` is cut under 16 KiB; `repo_context` records each file's path, whole size, and sha256 |
| `TestPackAcceptanceLines` | A requirement goes in as its heading, statement, and acceptance lines. A decision naming `REQ-001` goes in as `RFC-x: statement`; one naming `REQ-0010` and an unrelated one do not |
| `TestTodoWriteEchoed` | After `todo_write`, every later request carries the list in the newest turn's note only, the tool returns `todo list: 2 items, 0 done`, and the attempt still commits |
| `TestTodoWriteCap` | 30 items save. 31 items, text over 200 bytes, empty text, an unknown status, or a non-array are errors, and an error leaves the list as it was |
| `TestReadOnlyCallsConcurrent` | Three read-only calls of one reply all start before any finishes, and results keep the reply's order. A write between two reads runs after the first and before the second |
| `TestAnthropicCacheBreakpoints` | The first request marks the system block, the last tool, and the pack. A request with turns marks the system block, the last tool, and the newest turn's last tool result only, and the newest turn's note comes after that block (RFC-083) |
| `TestAnthropicCacheUsage` | `cache_creation_input_tokens` is added to `InputTokens` and `cache_read_input_tokens` is `CachedInputTokens` |
| `TestOpenAICachedTokens` | `prompt_tokens_details.cached_tokens` is `CachedInputTokens` and is subtracted from `InputTokens` |
| `TestCacheUsageRecorded` | Each call's input, output, and cached tokens add up on the attempt, `CardTokens` counts the cached ones at a tenth, and `model_call` records `cachedInputTokens` |
| `TestTokenCapCountsCachedAtTenth` | With a 100-token cap and calls of 10 input, 10 output, and 500 cached, the card stops after two calls with `token_cap` |
| `TestHistoryPrefixStable` | Over 40 requests, earlier turns are byte-identical between requests except where a chunk of two or more old results becomes the placeholder; the todo list is only on the newest turn |
| `TestRunCommandParse` | `ParseArgv` takes 1 to 64 strings of at most 4 KiB, refuses NUL, a path or spaced `argv[0]`, and a name off the list; a non-array `argv` is a tool error (RFC-084) |
| `TestRunCommandFlags` | The container argv has `--network=none`, the pinned image, `--read-only`, `--cap-drop=ALL`, the non-root user, a 5-minute timeout, 1 MiB of output, and mounts a fresh directory under `<data>/exports` |
| `TestRunCommandOnCopy` | The command sees the worktree's files on a fresh copy per call; its writes do not reach the worktree and the copy is deleted |
| `TestRunCommandDisabled` | With no allowed commands the reply is `error: run_command is not enabled for this project` and nothing runs |
| `TestRunCommandBudget` | The 31st call in an attempt is a tool error and runs nothing |
| `TestRunCommandCannotPassGate` | An exit 0 records no passing tree and runs only the command, never the card's check; fixture mode replies `error: run_command needs the container sandbox` |
| `TestSaveDeployCommandExecutables` | A path, a spaced name, a shell, or a wrapper in `commandExecutables` is a field error; a valid list saves, is listed as changed, and an unset list reads as `[]` |
| `TestNoChangeGoesToGate` | An attempt with no new commit on a clean branch already ahead of base sends that HEAD to the gate and reaches `pr_open` with the earlier commit |
| `TestReviewerPackHasNoAuthorText` | The implementer's stored reply text is in its `model_call` event and in no reviewer prompt |
| `TestTraceSince` | `since` returns events from 10 seconds before it, including two in the same second in insert order, each with a rising `seq`. Without `since`, all events. `since=yesterday` is `422` on `since`. The card under another project is `404` |
| `TestTraceLive` | `live` is false for a `human_review` card with no open attempt, true with an open attempt (with `maxTurns`), false once it ends, true while a run is queued, and true for an `in_progress` card |
| `TestCardDiffRoute` | A member gets `200` with the head SHA, and `missing: true` without a clone. A non-member gets `404` |
| `TestReviewWithoutVerdictUsesChecks` | A reviewer with no verdict and no checks reaches `human_review` with `review_verdict = 'failed'`, `ci_required = false`, one "AI review: Not done" comment, and one `job_error` (RFC-069) |
| `TestReviewMaxTokens` | Every reviewer request carries `MaxTokens` 32,768 (RFC-075) |
| `TestAnthropicReviewMaxTokens` | The Anthropic client sends a request with `max_tokens` 32,768 to the server for any model, instead of the SDK's "streaming is required" refusal |
| `TestNoModelOptionsSendsNothing` | With no model settings, the anthropic and openai request bodies have no `thinking`, `output_config`, or `reasoning_effort` (RFC-079) |
| `TestSaveLLMModelOptions` | `adaptive` on openai, a budget under 1,024 or over 32,768, an unknown effort, and an option for a model the family does not list are refused with field `modelOptions` |
| `TestLLMSettingsModelOptions` | `modelOptions` saves and reads back through `PUT` and `GET /api/settings/llm`, a bad value is `422` on `modelOptions`, and no key is returned |
| `TestAnthropicThinking` | `enabled` sends `budget_tokens` and raises `max_tokens` by it. `adaptive` and `output_config.effort` appear in the body |
| `TestAnthropicReplaysThinking` | Signed thinking blocks from a reply come first in the next assistant message for the same model, and are left out for another model |
| `TestOpenAIEffortAndThinking` | The body has `reasoning_effort` and `thinking: {"type": ...}` |
| `TestOpenAIReplaysReasoningContent` | `reasoning_content` from a reply goes back on the assistant message for the same model only |
| `TestRoutedProviderModelOptions` | A request sent to the fallback carries the fallback model's settings, and one to the primary carries the primary's |
| `TestReviewRetriesCutReply` | One `max_tokens` reply, then `read_diff` and `recommend_approve`, reaches `human_review` on the route's model, with three reviewer `model_call` events, the first stopped at `max_tokens` |
| `TestReviewCutTwiceFails` | Cut replies on the model and the fallback store `failed`, one `job_error` naming the token limit, and one "AI review: Not done" comment with the token-limit reason |
| `TestReviewWithoutVerdictRedChecks` | No verdict and a red check: `changes_requested` with one `ci` note |
| `TestReviewNoteRedacted` | A `request_changes` body that holds the company token is stored as a `review` note with `[redacted]` in its place |
| `TestReviewWithoutVerdictWaitsForChecks` | No verdict and pending checks stay in `agent_review`. Green checks on refresh: `human_review` |
| `TestNoChecksWaitsToRegister` | No checks right after `pr_open` keeps `agent_review` with `ci_required` unset, through a refresh. After `NoChecksWait`: `human_review` with `ci_required = false` |
| `TestChecksInsideWaitDecide` | Green checks inside the wait move the card to `human_review` |
| `TestAgentReviewToPROpen` | `agent_review -> pr_open` needs the attempt and the actor, and writes no gate (RFC-076) |
| `TestRetryReviewQueuesReview` | A `failed` review moves the card to `pr_open`, clears the review, keeps one `local_checks` gate, records the actor, and the next review job records its verdict with `attempts_used` unchanged |
| `TestRetryReviewNeedsFailedVerdict` | `pr_open`, a `recommend_approve` verdict, and another project are refused, and the card stays |
| `TestRetryReviewLocked` | A held card lock is `ErrLocked` and keeps the `failed` review |
| `TestRetryReviewAction` | A Developer sees `retry_review`, `GET` is `405`, `POST` returns `pr_open` without the action, and a second `POST` is `409` |
| `TestRetryReviewRole` | A Product Owner does not see `retry_review`, and `POST` is `403` |
| `TestRefreshFinishesStoppedReview` | An `agent_review` card with no verdict stays while another holder has the lock. Once the lock is free, refresh records `failed` and reaches `human_review` |
| `TestIngestRecordsCreator` | A web ingest stores the caller as `created_by` on new parsed rows. A second ingest by another user keeps the first. A CLI ingest stores null |
| `TestCardOwnerFromRequirement` | `create_card` stores the requirements' creator as `owner_id`, and an `ownerId` argument is ignored |
| `TestCreateCardMixedOwners` | `create_card` naming requirements from two creators is rejected and inserts no card |
| `TestCardReviewers` | The board and the card route list the requirement reviewers sorted by email, without the owner, with `githubLogin` `""` when unverified. A card in another project naming the same requirement id has `"reviewers": []` |
| `TestCardDetail` | The single card returns `requirements` from its own project only, `rules` labelled `global` and by stack, and `skills[].useWhen`. The board list has no `requirements` or `rules`. Unblock returns the same detail fields |
| `TestCardDetailNotes` | The single card returns `notes` newest first with source, text, and `attemptN`, `0` for an approver note. A note stored under another project is left out. The board list has no `notes` |
| `TestCardDetailPullRequest` | The single card returns `"pullRequest": null` before publish. After an attempt stores `pr_title` and `pr_body`, it returns the title, `attemptN`, `sha`, and the body without the metadata `<details>` block. The board list has no `pullRequest` (RFC-110) |
| `TestLatestPRText` | `LatestPRText` returns nothing before publish, the newest attempt with a stored title, and nothing for another project |
| `TestPublishStoresPRText` | `Publish` stores the title it sent and the body with the owner token redacted on its attempt, with the attempt's `n` and commit |
| `TestApprovedToDone` | `approved -> done` without a SHA is `ErrEvidence`. With the merge SHA it moves to `done` and writes no gate row (RFC-111) |
| `TestDeployEnabledDefault` | A project with no `deploy.enabled` key deploys. Saving `false` turns it off and lists `deployEnabled` as changed, and a later save without the key keeps it off |
| `TestActionsDeployOff` | On an approved card, a Lead Developer gets `deploy` while deploy is on. With it off, the lead gets `merge` and not `deploy`, a QA Lead gets neither, and the card owner gets `merge` only with solo approval on |
| `TestSquashMerge` | One `gh api repos/acme/widget/pulls/7/merge --method PUT` with `merge_method` `squash` and `sha` equal to the approved SHA. The caller's token is in `GH_TOKEN` and not in argv. The card is `done`, and the `merge` event has the merge SHA and login and no token |
| `TestSquashMergeNoToken` | A lead with no verified GitHub token gets `422` naming Account, no GitHub call is made, and the card stays `approved` |
| `TestSquashMergeRefused` | GitHub's "not mergeable (HTTP 405)" comes back as `422` with that text and no token, and the card stays `approved` |
| `TestDeployRefusedWhenOff` | Merge while deploy is on is `422`. Deploy while it is off is `422` "deploy is off", the card stays `approved`, and no GitHub call is made |
| `TestBlockedToWontFix` | `blocked -> wont_fix` without a reason or an actor is `ErrEvidence`. With both it keeps `blocked_reason` and writes no gate row, and `wont_fix -> blocked` restores the reason (RFC-112) |
| `TestActionsWontFix` | A Lead Developer, a Product Owner, and the card owner see `wont_fix` on a blocked card. A QA Lead does not, and the route is `403`. A `wont_fix` card is in group `closed` and offers `reopen` and no `instructions` |
| `TestWontFixClosesPR` | An empty or 501-character reason is `422` on `reason` with no GitHub call. Then one state read of `repos/acme/widget/pulls/7`, one comment "Closed as won't fix by @lead: …" on `issues/7/comments`, and one `PATCH` with `state: closed`, each with the caller's token in `GH_TOKEN` and not in argv. The card is `wont_fix`, and the `wont_fix` event has `prClosed: true` and no token |
| `TestWontFixNoPR` | A card with no pull request closes with no GitHub token and no GitHub call |
| `TestWontFixNoToken` | A pull request and no verified token give `422` naming Account, no GitHub call, and the card stays `blocked` |
| `TestWontFixClosedPR` | A pull request whose state is `closed` or `merged` gets the state read only |
| `TestWontFixRefused` | A refused `PATCH` comes back as `422` with GitHub's text and no token, and the card stays `blocked` |
| `TestReopen` | A QA Lead gets `403`. The lead's reopen moves the card to `blocked` with its old `attempt_cap` reason, and a second reopen is `409` |
| `TestEnsurePRReopens` | Publishing a card with an existing pull request sends `state: open` with the title and body |
| `TestCardDiffAfterMerge` | After the card branch merges into `main` with a merge commit and `main` moves on, the diff still lists the card's files against `main` before the merge |
| `TestCardDiffAfterFastForward` | After `main` fast-forwards to the card head, the diff lists the card's files against the first attempt's `base_sha` |
| `TestPRBodyForDisplay` | The display body drops the "Orchestrator metadata" block, keeps the sections, undoes the `&lt;!-- orchestrator:` escape, and leaves a body without metadata trimmed and otherwise unchanged |
| `TestCardPeopleNames` | The card route returns `owner.name` and `reviewers[].name`. A reviewer without a name has `"name": ""` (RFC-058) |
| `TestReviewersSetByCreator` | The creator and a Lead Developer set reviewers on a parsed row. Another Developer is `403`. A non-member reviewer or the creator is `422`, and an unverified tribe member is accepted (RFC-103) |
| `TestReviewerOptionsUnverified` | The intake's `reviewerOptions` lists an enabled member without a verified login with an empty `githubLogin`, and leaves out a disabled member and a non-member (RFC-103) |
| `TestConventionCatalogAdminOnly` | Every signed-in user reads `/api/conventions`. A Lead Developer who is not an admin gets `403` on create, update, and delete. An admin save appends `config_changed` with section `conventions` (RFC-055) |
| `TestConventionRevisions` | Editing a rule increments its version, changes its hash, and keeps the old text in `convention_revisions`. A save with the same text keeps the version |
| `TestDetectStacks` | A repo with `go.mod`, and a `web/package.json` that depends on `@sveltejs/kit` and `typescript`, is detected as Go, TypeScript, and SvelteKit. `node_modules` and a symlinked directory are not walked |
| `TestSyncKeepsManualStacks` | A Sync replaces detected stacks, keeps a stack added by hand, and appends `stacks_detected` |
| `TestProjectConventionsLeadOnly` | A Lead Developer member saves on and off choices for stacks, skills, and rules. A Developer member is `403`. An unknown or disabled id, or an id in both lists, is `422` |
| `TestCreateCardSkills` | `create_card` with an enabled skill stores it in `card_skills` and the card JSON lists it. A skill not enabled for the project stores no card (RFC-056) |
| `TestTribeConventionsSave` | A tribe member reads the tribe's conventions and gets `403` on save. A tribe admin saves stacks, skills, and rules switched off. Another tribe's member is `403`, an unknown tribe `404`, and a disabled id `422` (RFC-104) |
| `TestEffectiveConventions` | A tribe skill reaches the project and a project `off` removes it. A tribe rule `off` removes a global rule and a project `on` brings it back. A tribe stack adds its rules. A disabled skill, rule, or stack never applies. Each item carries its origin (RFC-104) |
| `TestProjectConventionsMigration` | After `00027`, a project's enabled skills and manual stacks are `on` rows, its detected stacks stay, and its effective set is unchanged (RFC-104) |
| `TestPackUsesEffective` | An attempt's `conventions_context` lists a tribe skill attached by the planner, leaves out a rule the tribe switched off, and `create_card` accepts a skill the tribe enabled (RFC-104) |
| `TestCardContextRoles` | A Developer attaches a skill, detaches a rule, and detaches a server; attaching a server is `403` for a Developer and allowed for a Lead Developer and a tribe admin. A Product Owner who is not an admin is `403`. The card JSON lists `context` and `attach-mcp` to match (RFC-105) |
| `TestCardContextDone409` | A context change on a `done` card is `409`, and the card status never changes (RFC-105) |
| `TestCardContextScoped` | A context change on a card of another project is `404`, and `card_context` rows carry the card's project (RFC-105) |
| `TestCardContextPack` | After a card attaches a skill and detaches a rule, the next attempt's `conventions_context` has the skill and not the rule; `inherit` restores the rule (RFC-105) |
| `TestMCPScopeCardAttach` | A card attach adds an enabled server beside the mentioned one, a detach removes a server from scope `all`, `mcp_context` has scope `card` with `attached` and `detached`, and a requirement edited since acceptance still gives `none` (RFC-105) |
| `TestCardContextDisabledServer` | Attaching a disabled server, a server of another tribe, or a disabled skill is `422` (RFC-105) |
| `TestRunnerTokenHashed` | A minted registration token and a runner secret appear in no row; the rows hold their SHA-256 hex (RFC-106) |
| `TestRegistrationSingleUse` | A token registers once; a second use and a token past its hour are `401`; minting is `403` for a non-admin (RFC-106) |
| `TestRunnerAuthScope` | A runner secret is `401` on `/api/cards`, a user bearer is `401` on `/api/runner/heartbeat`, and a removed runner's secret is `401` (RFC-106) |
| `TestRunnerStatusDerived` | A runner is `idle`, then `busy 1/2` with a lease, `draining` after drain, and `offline` once its heartbeat is 61 seconds old; actions follow the state (RFC-106) |
| `TestClaimSkipLocked` | Eight concurrent claims for three run requests make three leases on three distinct runs, and each run's `processed_at` is set (RFC-107) |
| `TestClaimRespectsCap` | With `implementationCap: 1`, a second claim in the project gets nothing until the first lease ends (RFC-107) |
| `TestCardAffinity` | A card's next run request goes to the runner that held it, not to another idle runner, until that runner is offline (RFC-107) |
| `TestReaperRequeues` | A lease of a runner whose heartbeat stopped ends as `lost`; its open attempt is `cancelled`, its card lock is gone, affinity is cleared, the run row is open again, and the card status is unchanged (RFC-107) |
| `TestReconcileSkipsLiveLease` | `ReconcileProject` cancels an unleased card's open attempt and keeps a leased card's attempt and lock (RFC-107) |
| `TestLocalExecutorUnchanged` | With `jobs.executor: local` the worker runs the implement job in process; with `runners` it starts none and the run row stays open for a claim (RFC-107) |
| `TestModelProxyBudgets` | A model request through a lease uses the lease's model whatever the request names, and a request for an ended lease is `404` (RFC-108) |
| `TestLiveFromRunner` | A model call through the proxy publishes `turn`, `delta`, and `turn_end` frames to the card's live hub (RFC-108) |
| `TestRunnerNeverGetsSecrets` | No runner response body contains `databaseUrl`, `tokenKey`, a model key, or the owner's token, and `start` carries the company token only as `cloneToken` (RFC-108) |
| `TestRunnerCannotSetStatus` | A board op named `move` or `transition`, and an `event` of kind `transition` or `gate`, are `422` and change nothing (RFC-108) |
| `TestCheckExitDecides` | A remote attempt whose check reports exit 1 returns to `in_progress` with a `local_check` note and a retry row; exit 0 after a bundle reaches `pr_open` with one `local_checks` gate (RFC-108) |
| `TestBundleOnlyCardBranch` | A bundle with another branch, a head that is not the attempt commit, or a commit that does not descend from the attempt base is `422` and moves no ref (RFC-108) |
| `TestSeedCardBranch` | After a fetch of the published card branch, a runner's missing `orch/<card-id>` points at it and a branch already ahead of it is kept (RFC-107) |
| `TestRunnerLeaseScope` | A runner's job call on another runner's lease is `404`, and an op naming another card still works on its own lease card (RFC-108) |
| `TestMCPProxyNetguard` | The job's MCP tools come from servers the control plane opens with its dialer; a private-address server is blocked there and the runner sees no URL or secret (RFC-108) |
| `TestProtocolMismatchRefused` | A claim from protocol 0 is `409` `protocol`, and the runner lists `updateRequired: true` (RFC-109) |
| `TestRunnerReleaseFiles` | `/runner/install.sh` and `/runner/download/{file}` are `404` with no release directory, serve the listed files from it, and refuse any other name (RFC-109) |
| `TestRunnerChaos` | Two runners share a queue; one stops heartbeating during an attempt; the reaper requeues and the other runner takes the card to `pr_open` (REQ-094) |
| `TestNotifyTriggers` | A card update, a run insert, an event insert, and a notification insert each wake the listener's hub with the card's project, or the user, and a write in another schema wakes nothing (RFC-113) |
| `TestNotifyCursor` | Events written while no listener runs are handed on after it starts; an event committed with a `seq` below the cursor inside the 10-second overlap is handed on once; the cursor only rises (RFC-113) |
| `TestRecipients` | Each RFC-113 trigger notifies the listed people: reviewers or approvers for review, leads for ship, the owner for changes requested, leads too for `deploy_failed`; never the actor, a non-member, or a disabled user, and one note per person (RFC-113) |
| `TestFanoutOnce` | Two notifiers given the same events write one row per person and kind, and only the one that inserted it pushes it (RFC-113) |
| `TestStreamCards` | A non-member's stream is `404`; the first frames are `ready`, a `full` `cards` frame with the viewer's own actions, and `board`; a status change sends only that card, with actions that differ by role (RFC-113) |
| `TestStreamNotification` | Requesting changes on an owned card sends the owner a `notification` frame with the unread count and pushes it, and sends the actor nothing (RFC-113) |
| `TestNotificationsRead` | The list holds only the viewer's rows in projects they still see, newest first; marking ids and all lowers the count; no ids, or more than 50, is `422` (RFC-113) |
| `TestPushRoutes` | Push routes are `503` without a key; an endpoint off the allowlist and bad keys are `422` with the field; save and delete change only the viewer's rows (RFC-113) |
| `TestWebPushEncrypt` | `encrypt` with the RFC 8291 Appendix A keys and salt gives the appendix's message byte for byte; a payload over one record is refused (RFC-113) |
| `TestVAPIDJWT` | The JWT has `ES256`, the endpoint origin as `aud`, `mailto:` as `sub`, `exp` within 24 hours, and a signature that verifies with the public key (RFC-113) |
| `TestPushEndpointGuard` | Each allowlisted push host is accepted; `http`, IP literals, metadata addresses, lookalike hosts, user info, other ports, fragments, and over-long URLs are refused (RFC-113) |
| `TestPushGone` | Against a TLS stub, a `410` deletes that subscription, five `500`s in a row delete another, success keeps one; the request carries `vapid` auth and `aes128gcm`, and the decrypted payload is exactly `title`, `body`, `url`, and `tag` (RFC-113) |
| `TestVAPIDKeySecret` | The first `serve` writes `vapidPrivateKey` to `secrets.json`, a second keeps it, an LLM settings save keeps it, and `GET /api/push/key` returns the public key and never the private one (RFC-113) |
| `TestWaitingAnswerEdges` | `in_progress -> waiting_answer` without an attempt id is `ErrEvidence`; `waiting_answer -> ready` needs an actor; `waiting_answer -> blocked` needs a reason; `waiting_answer -> in_progress` is rejected (RFC-114) |
| `TestAskUserValidation` | Six questions, a `single` question with one option, repeated options, options on a `text` question, and a suggestion that is not one of the options each return `error: ask_user: ...` and store nothing; the attempt goes on (RFC-114) |
| `TestAskUserParksCard` | A Manual card's attempt that calls `ask_user` makes no further model call, finishes with outcome `question`, stores the questions open with the attempt's redaction applied, and the card is `waiting_answer` with `attempts_used` unchanged and no check run (RFC-114) |
| `TestAskUserAutoAnswers` | An Auto card's `ask_user` stores the questions with `answer_source` `auto` and the suggestion as answer, the tool result starts `No person answers on this card.`, and the attempt goes on to commit and `pr_open`; a fourth call in one attempt is a tool error (RFC-115) |
| `TestAskUserAfterCommitRejected` | `ask_user` after `commit` replies `error: ask_user is not available after commit` and stores nothing |
| `TestQuestionRoundCap` | A Manual card with three `question` attempts gets the cap error, stores nothing, and the attempt goes on |
| `TestQuestionAttemptNotCounted` | After a parked attempt and an answer, the next attempt's `n` rises and `attempts_used` counts only that attempt |
| `TestAnswerQueuesRun` | Answering every open question stores each answer with the caller and source `person`, moves the card to `ready`, inserts one open card-scoped run, appends `questions_answered`, and the card JSON lists `queued: true` (RFC-114) |
| `TestAnswerValidation` | A missing question, an unknown id, two options on `single`, text on `single` without `allowOther`, an option not listed, and empty `text` are each `422` on `answers.{n}` or `answers` with the card still `waiting_answer`; a `ready` card is `409`; a Product Owner is `403` |
| `TestAnswerSuggestions` | `{ useSuggestions: true }` stores each suggestion with source `suggestion` and queues the run |
| `TestQuestionModeRights` | A Developer sets `auto` and a `question_mode_changed` event is appended; a Product Owner is `403`; `sometimes` is `422`; a `done` card is `409`; the card JSON carries `questionMode` and lists `question_mode` only for the allowed roles (RFC-115) |
| `TestPackIncludesAnswers` | The next attempt's pack holds `ANSWERS FROM THE TEAM` with each answered question, its answer, and its source, inside the untrusted markers, and an open question is not in it (RFC-114) |
| `TestQuestionNotifiesOwner` | `-> waiting_answer` gives the owner one `question` notification titled "Agent has a question" with push on, and no one else (RFC-114) |
| `TestRunnerQuestionOutcome` | A runner's report with outcome `question` moves its lease card to `waiting_answer` without counting the attempt; the questions op writes rows only for the lease's attempt and card (RFC-114) |
| `TestConfigEvent` | `config_changed` payload has section and field names and not the token |
| `TestSnapshot` | A router save during a running attempt does not change that attempt's model |
| `TestActionsEmpty` | Card JSON for `in_progress` has `"actions": []` |
| `TestActionsApproved` | Card JSON for `approved` has `request_changes` and `reject` only |
| `TestApproveSha` | Wrong SHA returns `422` and `approved_sha` stays null |
| `TestApproveIdempotent` | The same SHA twice inserts one `approvals` row |
| `TestCORS` | Origin `http://127.0.0.1:9999` is `403`. `http://127.0.0.1:4173` is allowed |
| `TestWorkerLock` | A second `serve` exits. `run-next` while `serve` runs inserts a `runs` row and exits `0` without running a job |

## Runtime tests

These use `testdata/fixture-repo`, a git repository with one file and a check script.

| Test | Asserts |
| --- | --- |
| `TestWorktreeOnce` | Two implement jobs for one card create one branch and one worktree |
| `TestCheckExit` | Gating exit `0` can open a pull request. Exit `1` cannot. A model message that says the tests passed does not override exit `1` |
| `TestToolCheckDoesNotMove` | `run_local_check` exit `0` inside an attempt does not change status |
| `TestNoCommitAttempt` | An attempt that ends without a commit counts, writes a `no_commit` note, and the next attempt gets a new id and `n`. The implement job queues that attempt with a `retry` row, covered by `TestRetryAfterNoCommit` (RFC-071) |
| `TestCommandAllowlist` | A tool call that names a command does not exec it |
| `TestEnvScrub` | The sandbox env has no `GH_TOKEN`, model key, or `DEPLOY_TOKEN` |
| `TestVerifyAtDeployedSHA` | An uncommitted edit left in the worktree is absent from the dev check export |
| `TestPROnce` | Calling ensure-PR twice leaves one `pr_url` |
| `TestMetadataRewrite` | The second push updates the metadata block and keeps the summary block |
| `TestCIPending` | A pending required check keeps the card in `agent_review`. A later pass moves it when the check passes |
| `TestCIRed` | A failed required check moves the card to `changes_requested` with a `ci` note |
| `TestStaleSHA` | A new commit after approval clears `approved_sha` and returns the card to `human_review` |
| `TestDeployFailed` | Deploy stub exits `1`. Rollback runs. Card is `blocked` with `deploy_failed` |
| `TestHealthFail` | Stub health returns 500. Rollback runs once. Card is `blocked` with `health`. No `verify` event |
| `TestHealthOnly` | Empty `dev_check` and a healthy probe mark the card `done` with verify payload `mode: health_only` |
| `TestDevFailThenPass` | First dev exit `1` goes to `changes_requested` on the same `pr_url`, and the next implementer pack has the failure note. The second cycle exits `0` and marks `done` |
| `TestDependency` | Card B stays out of `in_progress` until card A is `done` |
| `TestDeployLock` | Two approved cards produce non-overlapping `deploy` events |
| `TestCap` | The third counted attempt failing on a card with `max_attempts` 3 ends `blocked` and opens no other attempt |
| `TestCommitIdentity` | Commits use the `runtime.Git.Author` they are given and carry the three trailers |
| `TestCommitIdentityOwner` | A commit on an owned card has the owner's verified login and `<id>+<login>@users.noreply.github.com` as author and committer (RFC-054) |
| `TestRequestReviewers` | The reviewer request names the union of the card's requirement reviewers without the owner, the bot, or an unverified user. An unverified reviewer is listed in `waiting`. A failed request writes `reviewers_requested` with `ok: false` and the card status is unchanged |
| `TestRequestReviewersOnlyWaiting` | A card whose only reviewer has no verified login makes no GitHub call and writes `reviewers_requested` with empty `reviewers` and that reviewer in `waiting` (RFC-103) |
| `TestReconcileDeploy` | Kill after the `deploy` event and before `health`. Restart probes or rolls back and does not exec deploy again first |
| `TestReconcileCommit` | Kill after a commit and before the gating check. Restart runs the check without a model call and makes no second commit |

The health stub is an `httptest.Server` on `127.0.0.1`, allowed by fixture mode. It is not a public URL.

## REQ-043 runner

`go run ./cmd/fixture` performs the nine steps against a temporary data directory and `testdata/fixture-repo`.

| Step | What the runner checks |
| --- | --- |
| 1 | Ingest of a two-requirement PRD. `run-next` before accept creates no card and no worktree |
| 2 | After accept, the plan job creates two cards. The second `depends_on` the first |
| 3 | Card 1 has a worktree, a gating exit `0`, and one pull request URL |
| 4 | The reviewed attempt has `review_independent` true with two families in the fixture config. The recorded reviewer prompt has no `author-transcript` |
| 5 | Approve the head SHA. A new commit before deploy clears the SHA and sets `human_review`. Approve the new head |
| 6 | Health stub fails. Rollback ran. Card is `blocked` with `health`. No verify event. `unblockTo` is `human_review` |
| 7 | Unblock. The card is in `human_review` and no implementer attempt started. Re-approve the same head. The stub now passes health and the dev check fails. New attempt, same pull request, failure note stored |
| 8 | Health pass and dev exit `0` mark card 1 `done`. Only then does card 2 enter `in_progress` |
| 9 | Process stopped after `deploy` and before `health`. New process reconciles before a second deploy exec |

Step 6 uses the stub. The runner does not need a real dev server. After this command exits `0`, a person may point the same deploy and health code at one real dev target. That run is recorded by hand and is not part of `go test`.

The fixture config uses two families and two model ids so step 4 can require `review_independent` true. The fake provider serves both ids. No network.

After step 9, the runner runs the tool loop with the fake provider on two more cards: one that spends every turn without a commit must end blocked with `turn_cap`, and one over a 100-token card cap must end blocked with `token_cap`, each with one `budget_stop` event (RFC-081). Then it runs `git status --porcelain` in the fixture repo and requires empty output (REQ-045).

## CLI checks

| Command | Expected |
| --- | --- |
| `orchestrator status` on an empty data dir | Exit `0`, stdout contains `board is empty` and `orchestrator ingest` |
| `orchestrator approve card_x` without `--sha` | Exit `1`, no `approvals` row |
| `orchestrator approve card_x --sha wrong` | Exit `1`, stderr has the validation message |
| `orchestrator run-next` when idle | Exit `0`, stdout contains `no card is ready` |
| `orchestrator unblock card_x --reset-attempts` on a `turn_cap` card | Exit `0`, card `ready`, `attemptsUsed` 0 |
| `orchestrator serve --data <repo>/.orchestrator` | Exit `1` before listen |
| `orchestrator usage --by project` with `--tribe tribe_gadget`, `--user usr_ana`, and both with the other tribe (`TestUsageCLIScope`) | Exit `0`. The company lists Gadget and Default; `--tribe` and `--user` list only Gadget; the other tribe with that user prints `no model calls in this range` |
| `orchestrator usage --by source` and `--by card` (`TestUsageCLICard`) | Exit `0`. By source, `card 1 100` then `intake 1 40`; by card, `Export CSV (card_...) 1 100` then `Intake 1 40` |
| `orchestrator usage --from 2026-10-01 --to 2026-10-03 --by task` (`TestUsageCLI`) | Exit `0`, a heading with the range, then `implement 1 100 20 0 120` and `review 1 30 5 0 35`. `--by cost` exits non-zero naming `--by` |

## Frontend checks

From `frontend/`:

```text
npm ci
npm run check
npm run test
npm run build
npm audit --omit=dev
```

The Vitest cases are listed in [0004-frontend.md](0004-frontend.md). `npm audit` with no runtime dependencies should report nothing. Any finding blocks the release until it is read and recorded.

## Browser pass

Run this after `orchestrator serve` and `npm run dev` are up. Use the fixture data dir so the board has a known `in_progress` card, a known `human_review` card, a known `approved` card, and a known `blocked` card.

| Check | Pass |
| --- | --- |
| Open the UI without a bearer | Login form is the only page |
| Wrong password | Live region shows the error, no board |
| Six wrong passwords for one email | Live region says to wait one minute |
| Empty data dir | "No cards yet" and a link to Intake |
| `in_progress` card | No action controls |
| `human_review` card | Approve form shows `headSha`. Submitting another SHA shows the field error and the card stays in `human_review` |
| `approved` card for a Lead Developer | Request changes, Reject, and Deploy |
| `approved` card for a Product Owner | Request changes and Reject, and no Deploy |
| `approved` card for a Lead Developer with Deploy approved cards off | Request changes, Reject, and Squash and merge, and no Deploy (RFC-111) |
| `blocked` card with `health` | Unblock text says the card returns to human review |
| `blocked` card, Won't fix with a reason | The card leaves the board, an open pull request has the comment and is closed, "Show won't fix" shows it under Won't fix, and Reopen returns it to Stopped (RFC-112) |
| Approve in the UI while the CLI approves the same card | One approval row. The second action gets a reload or the same card back |
| Settings GET after a token was saved | Password field is empty. Placeholder says a value is saved. The network response has no token |
| Deploy settings with a repo-relative command | Field error on the command |
| Router with one family | The independence sentence from the API is visible |
| Router save of an unknown model | Field error, yaml unchanged |
| Leave the board while a poll is active | No further card requests after navigation |
| Two browsers, two people, one project: one requests changes on the other's card | The other's board moves the card within a second with no poll request, their bell count rises with a toast, and the actor gets no notification (RFC-113) |
| Turn on browser notifications in Account, then Send a test | The OS notification appears; clicking it focuses the tab on Account. A card notification opens the card, switching project when needed (RFC-113) |
| Built page | Response `index.html` has the CSP meta tag. The console shows no CSP violation |
| Keyboard | Tab reaches nav, Run, and the settings submit. Focus outline is visible |

This pass is manual. The automated locks for buttons are `TestActionsEmpty`, `TestActionsApproved`, and the `CardActions` Vitest cases.

## Hardening tests

The cases in [0006-security-and-harness.md](0006-security-and-harness.md) are part of `go test ./...`. A build that skips `TestCheckOnExport`, `TestHookNotRun`, `TestFsmonitorOff`, `TestHealthLinkLocal`, `TestEventsImmutable`, `TestPushURL`, or `TestDataDirInsideRepo` is not a release candidate.

Sprints 11–13 add `TestPasswordLogin`, `TestWrongPassword`, `TestSessionLockout`, `TestBearerExpiry`, `TestLastAdmin`, `TestUserGitHubOmitted`, `TestNonAdminForbidden`, `TestMemberSeesProject`, `TestOutsiderMissesProject`, `TestReqIdsPerProject`, `TestBranchPerProject`, `TestAdminManagesMembership`, `TestParallelProjects`, `TestLeadDeploy`, `TestOwnerApproveStops`, `TestAcceptByOwner` (Sprint 16 replaces it with `TestAcceptSeniorRoles`), `TestDeveloperRunOnly`, `TestActorOnTransition`, and `TestCloudBind`. `TestLoopbackOnly` still passes when `ORCHESTRATOR_CLOUD` is unset. The REQ-043 fixture stays one project. Those new tests add a second project beside it. They are not part of the fixture's nine steps.

## Release order

1. `go test ./...` in `backend/`.
2. `go run ./cmd/fixture` exits `0`.
3. Frontend checks.
4. Browser pass on the fixture cards.
5. Optional: one real dev target for health failure and health success, using the same deploy code paths, with secrets only in the data directory's `secrets.json`.

Stop at the first failure. A green browser pass does not substitute for the fixture command.
