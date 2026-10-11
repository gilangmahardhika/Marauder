# RFC-0006: Company GitHub account and linked projects

Status: Accepted for implementation. Date: 2026-10-10.

This RFC adds decisions `RFC-049` through `RFC-051`. They supersede earlier statements only where they name the id. Those statements stay in their files.

Sprints 14 and 15 in [../plan/sprints.md](../plan/sprints.md) implement this file. The fixture in REQ-043 stays a single project attached with `--repo`.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-049 | One company GitHub account | RFC-030, GitHub token storage; RFC-046, project bot login; RFC-047, project bot token |
| RFC-050 | A project is one repository that the service clones | RFC-047, repo path given at create |
| RFC-051 | Linked projects share read-only context | RFC-013, fixed pack contents and tool list; RFC-024 and RFC-025, read roots |

## RFC-049 One company GitHub account

The service has one GitHub account for the company: a host, an organization, a bot login, and a token. An admin sets it. The token is ciphertext in PostgreSQL, sealed with `tokenKey`, and the API returns only `tokenSet`. Jobs use this token for clone, fetch, push, and pull requests in every project. They never use a signed-in user's token. The test runs `gh api user` and `gh api orgs/{org}` with the token in the child environment, and stores the login it reports as the bot login.

A bot token per project meant one credential to rotate per repository, and nothing kept two projects of the same company on the same identity.

Set aside: a GitHub App installation. The table keeps room for it, but a fine-grained token is the only kind in v1. Also set aside: a token per tribe.

Consequences: the project rows no longer carry a host, an owner, a bot login, or a bot token. Approval is rejected when the caller's GitHub login equals the company bot login. A user's own GitHub host, login, and token stay, and only name the approver. `secrets.json` no longer holds a GitHub token.

Requirements: REQ-044, REQ-051, REQ-052.

Superseded in part by RFC-054 in [0009-card-owner.md](0009-card-owner.md). The company token clones and fetches. Jobs commit as the card owner and push and open pull requests with the owner's verified token. A card whose owner has no usable token is blocked with `owner_github`. A user's login is the one GitHub reports for their token.

## RFC-050 A project is one repository that the service clones

A project names one repository in the company organization and one base branch. Two projects cannot name the same repository. The service clones it into `<data>/projects/<id>/repo` with the company token, and fetches the base branch before ingest and on Sync. Worktrees, exports, and logs for that project live under the same project directory. Deploy, checks, sandbox, scheduler, and paths are project settings in that directory's `config.yaml`, and the deploy token is in its `secrets.json`.

A path typed by an operator could point at any directory on the host, and it could be shared by two projects without anyone noticing.

Set aside: accepting a repo path from the API. `--repo` stays for the fixture and for break-glass use of the CLI.

Consequences: `POST /api/projects` takes `repo` and `baseBranch`. The base branch replaces the fixed `main` for worktrees. Intake, accept, and the requirement reads of the loop include `project_id`.

Requirements: REQ-049, REQ-053.

## RFC-051 Linked projects share read-only context

A Lead Developer may link their project to other projects in the same tribe. A link is one way: the project reads from the linked project. For each link the implementer pack and the reviewer pack add the linked project's accepted requirements and decisions, and the files named in the link's shared paths, read at the linked base branch. The implementer also gets `read_linked_file(project, path)`. It reads from an export of the linked repository at that SHA, with the same path rules and read cap as `read_file`, and it cannot write.

A frontend card often depends on the API contract in another repository. Without it the model guesses the contract, or the operator pastes it into the card.

Set aside: links across tribes. Membership is what keeps one tribe out of another's repo. Also set aside: a writable tool on the linked repository, and reading a live checkout.

Consequences: linked content sits inside the untrusted markers and is redacted like the rest of the pack. Ids are shown as `<project>:REQ-001`. The linked project ids and SHAs are written to an event with the attempt, so the pack can be rebuilt. The implementer tool list snapshot changes in the same release.

Requirements: REQ-016, REQ-017, REQ-054.

Superseded in part by RFC-057 in [0011-tribe-admin.md](0011-tribe-admin.md). A tribe admin may also save the links of any project in their tribe, and the creator of a project may choose its links at create.

Superseded in part by RFC-059 in [0013-project-settings-admins.md](0013-project-settings-admins.md). A company admin may save the links of every project, and a tribe admin those of every project in their tribe.
