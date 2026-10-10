# RFC-0007: Global models and router

Status: Accepted for implementation. Date: 2026-10-10.

This RFC adds decision `RFC-052`. It supersedes earlier statements only where it names the id. Those statements stay in their files.

Sprint 15 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-052 | LLM APIs and the router are one company setting that an admin edits | RFC-046, Lead Developer edits router and models; RFC-047, model keys in the project secrets file |

## RFC-052 LLM APIs and the router are one company setting

The service has one list of LLM families and one router: the families with their base URLs, models, and keys, and the routes and budgets per task class. Every project's jobs use them. Every signed-in user may read them, without the keys. Only an admin may save them or run a family's connection test. The keys stay in the service `secrets.json`, mode `0600`, and the API returns only `keySet`.

A company buys model access once. A key per project meant one more secret to rotate for each repository, and a Lead Developer could route a project to a model the company had not approved.

Set aside: a router override per project. A project that needs a different model gets it when the admin changes the shared route. Also set aside: letting a Lead Developer edit the shared settings.

Consequences: the routes are `/api/settings/llm`, `/api/settings/llm/{family}/test`, and `/api/settings/router`. A save appends `config_changed` with the field names and `actorId`. The UI lists LLM APIs and Router under Admin. A project's config holds deploy, checks, sandbox, scheduler, and paths, and its secrets file holds only the deploy token.

Requirements: REQ-044, REQ-051.
