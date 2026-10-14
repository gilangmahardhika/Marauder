# RFC-0005: Teams and a cloud board

Status: Accepted for implementation. Date: 2026-10-10.

This RFC adds decisions `RFC-045` through `RFC-048`. They supersede earlier statements only where they name the id. Those statements stay in their files.

Sprints 11–13 in [../plan/sprints.md](../plan/sprints.md) implement this file. The fixture in REQ-043 stays a single project.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-045 | Sessions are accounts | RFC-032, session and, when cloud mode is on, the bind; RFC-035, loopback UI sentence in cloud mode |
| RFC-046 | A job role gates the person, not the model | RFC-006, allowlist sentence; RFC-018, who accepts; RFC-035, operator login checked against approvers |
| RFC-047 | A tribe owns its projects | RFC-036 and RFC-044, one board per repo; RFC-030, secret file as the only store for GitHub tokens |
| RFC-048 | Workers run per project | RFC-010, one lock for every deploy; RFC-043, one worker for the process |

## RFC-045 Sessions are accounts

A person logs in with email and password. `POST /api/session` returns a bearer bound to that user. The password is stored as a hash. The bearer is stored as a hash in PostgreSQL, expires 12 hours after issue, and is checked on every later request. The printed UI token is not a login. `serve` refuses an address that is not loopback unless `ORCHESTRATOR_CLOUD=1`. In that mode the process may bind the configured address, the `Host` header must be on the configured allowlist, CORS uses the configured UI origin, and TLS is terminated in front of the process.

The UI token was one shared capability. A cloud board has many people, and a bearer that dies with the process cannot survive a restart or name the caller.

Set aside: binding off loopback without `ORCHESTRATOR_CLOUD`. Local `serve` stays on loopback. Also set aside: a second login that still accepts the printed token.

Consequences: five bad passwords for one email pause that email for one minute. A disabled user cannot log in, and their bearers no longer pass. `DELETE /api/session` drops the presented bearer. Default `serve` still fails `TestLoopbackOnly` for `--addr 0.0.0.0:8787`. Cloud mode is the only bind that RFC-032 no longer covers.

Requirements: REQ-046, REQ-042.

## RFC-046 A job role gates the person, not the model

A user has one job role: `lead_developer`, `senior_developer`, `developer`, `qa_lead`, `qa`, or `product_owner`. Admin is a boolean, not a role. The role applies in every tribe the user belongs to. Model profiles stay the packs in REQ-016 and are not these roles. A human gate runs only when the user is a member of the card's tribe and the role allows that gate. Approval still stores one SHA. The project's bot login cannot approve. Settings no longer carry an approver list or an operator login.

An allowlist in a config file cannot tell a Product Owner from a Developer, and it records the configured login no matter who held the bearer.

Set aside: a different role on each tribe membership. The role is one column on the user. Also set aside: treating admin as permission to approve or deploy.

Consequences: the server builds `actions` from the status and from the caller's role and membership. The client still renders only that list. The gates are:

| Caller | May |
| --- | --- |
| Admin flag | Create and disable users, set role and admin, set a password, create tribes, add or remove members, create projects. Not approve or deploy unless the job role allows it |
| Product Owner, and a member | Accept requirements, request changes, reject, approve, unblock |
| Lead Developer, and a member | Start a run, request changes, reject, approve, unblock, start deploy, edit that project's repo, checks, router, models, and deploy settings |
| QA Lead, and a member | Request changes, reject, approve |
| Senior Developer, and a member | Start a run, request changes |
| Developer or QA, and a member | Start a run |
| Any user | Change their own password and their own GitHub host, login, and token |

The last admin cannot be removed or disabled. Approval stores the user's GitHub login and is rejected when that login is empty or equals the project bot login. Accept stores the user id. New transition events may include `actorId`. Existing event rows are not updated. The CLI that can read the service data directory remains break-glass for bootstrap and the fixture. HTTP gates use the session user. `orchestrator users bootstrap` creates the first admin from `ORCHESTRATOR_ADMIN_EMAIL` and `ORCHESTRATOR_ADMIN_PASSWORD` when the user table is empty.

Requirements: REQ-003, REQ-030, REQ-032, REQ-047, REQ-051.

Superseded in part by RFC-049 in [0006-company-github-and-linked-projects.md](0006-company-github-and-linked-projects.md). The bot login an approver may not use is the company bot login. Admin edits the company GitHub account. A Lead Developer edits the project's repository, links, checks, and deploy settings.

Superseded in part by RFC-052 in [0007-global-models-and-router.md](0007-global-models-and-router.md). LLM APIs and the router are one company setting. An admin edits them, and a Lead Developer does not.

Superseded in part by RFC-053 in [0008-manual-requirements.md](0008-manual-requirements.md). A Lead Developer, Senior Developer, QA Lead, or Product Owner member may accept requirements. Any member may add a requirement by hand.

Superseded in part by RFC-057 in [0011-tribe-admin.md](0011-tribe-admin.md). A tribe admin may add and remove members of their tribe, make other members tribe admins, and create projects on their tribe.

Superseded in part by RFC-059 in [0013-project-settings-admins.md](0013-project-settings-admins.md). A company admin may save the settings of every project, and a tribe admin those of every project in their tribe, as well as a Lead Developer member. Opening a project's settings does not grant card access.

## RFC-047 A tribe owns its projects

Users, sessions, tribes, membership, and projects live in one PostgreSQL database. The connection string is `databaseUrl` in the service `secrets.json`. The API does not return it. A project belongs to one tribe. A tribe has one or more projects. A user may belong to several tribes. The user sees a project's cards, intake, and settings only as a member of that tribe. A non-member, and a request for an unknown project id, gets `404`. Admin manages user, tribe, membership, and project records. Each project has its own repo, settings, worktrees, and intake rows. Requirement ids and decision ids are unique per project.

Two target repos on one shared board used to collide on `REQ-001`, on `deploy_lock`, and on one GitHub token. Membership is what keeps one tribe out of another's repo.

Set aside: one database per project. The service has one `databaseUrl`. Also set aside: a `403` that tells the caller the project exists.

Consequences: board queries include `project_id`. Unique keys for a branch and a pull request URL include `project_id`. `intake_state` and `deploy_lock` are one row per project. The user's GitHub token and the project's bot token are ciphertext in PostgreSQL, encrypted with `tokenKey` from the service secrets file. Jobs decrypt the project bot token for push and pull requests. They do not use the logged-in user's token. GET returns `tokenSet` and never the token or the password hash. Deploy tokens and model keys stay in the project secrets file, mode `0600`, and stay off the API. Redaction still runs on the plaintext before a pack or an event is written.

Requirements: REQ-044, REQ-045, REQ-048, REQ-049, REQ-051.

Superseded in part by RFC-049 and RFC-050 in [0006-company-github-and-linked-projects.md](0006-company-github-and-linked-projects.md). Jobs use the company token, not a project bot token. A project names a repository in the company organization, and the service clones it. A user's GitHub token stays ciphertext. Deploy tokens stay in the project secrets file.

Superseded in part by RFC-052 in [0007-global-models-and-router.md](0007-global-models-and-router.md). Model keys are in the service secrets file, not the project secrets file.

Superseded in part by RFC-054 in [0009-card-owner.md](0009-card-owner.md). Jobs push and open pull requests with the card owner's verified token. Clone and fetch keep the company token.

Superseded in part by RFC-057 in [0011-tribe-admin.md](0011-tribe-admin.md). A tribe admin also manages that tribe's membership and creates its projects.

## RFC-048 Workers run per project

Each project has its own data directory outside that project's repo, its own `worker.lock`, and its own deploy lock. Workers for different projects run at the same time. Inside one project, one deploy runs at a time, and implementation stays under that project's cap. A card in `approved` does not deploy until a Lead Developer who is a member starts deploy. The push refspec stays `refs/heads/orch/<card-id>` for that project's remote (RFC-034).

One process-wide lock made every tribe wait on one dev server. One project still has one dev server, so two deploys of that project would still overwrite each other.

Set aside: a deploy that starts by itself when the card reaches `approved`. The Lead Developer action is the start. Also set aside: two deploys of the same project.

Consequences: `Reconcile` for a project runs only in the process that holds that project's worker lock, and it resets that project's deploy lock and that project's card locks. A second project keeps running. `POST` deploy is allowed only when `deploy` is in `actions`. The worker then runs the existing deploy job for that card.

Requirements: REQ-009, REQ-033, REQ-034, REQ-050.

Superseded in part by RFC-107 in [0046-remote-runners.md](0046-remote-runners.md). With the `runners` executor, a project's cap counts its open leases, and Reconcile leaves cards that a live lease holds.
