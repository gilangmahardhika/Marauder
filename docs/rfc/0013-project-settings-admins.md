# RFC-0013: Admins edit project settings

Status: Accepted for implementation. Date: 2026-10-11.

This RFC adds decision `RFC-059`. It supersedes earlier statements only where it names the id. Those statements stay in their files.

Sprint 22 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-059 | A company admin edits every project's settings, and a tribe admin edits its tribe's | RFC-046, the Lead Developer row for project settings; RFC-049, "A Lead Developer edits the project's repository, links, checks, and deploy settings"; RFC-051, "A Lead Developer may link"; RFC-055, "A Lead Developer member may add stacks by hand"; RFC-056, "A Lead Developer member enables skills"; RFC-057, "A Lead Developer member still saves the links" |

## RFC-059 A company admin edits every project's settings, and a tribe admin edits its tribe's

A project's settings are its repository, linked projects, deploy settings, and conventions (the stacks added by hand and the enabled skills). Three people may save them:

| Caller | May save the settings of |
| --- | --- |
| Company admin | Every project, whether or not the admin is a member of its tribe |
| Tribe admin | Every project in their tribe |
| Lead Developer, and a member | That project |

Any other member of the project's tribe may read the settings, and a save is `403`. Anyone else, including a company admin for an id that does not exist, receives `404`. Whoever may save the settings may also run Sync, as may a Developer, Senior Developer, or Lead Developer member.

Opening a project's settings does not open its board. A company admin who is not a member of the tribe still receives `404` for its cards, intake, and runs (RFC-046, RFC-048).

A company admin sets up the company GitHub account and creates projects, and then had to find a Lead Developer to type the repository and deploy settings of each one. A tribe admin already saved a project's links (RFC-057), but not the rest of the same page set.

Set aside: giving a company admin the board of every project. Card access stays a membership grant. Also set aside: a per-project editor list. The job role and the two admin flags cover the cases seen so far.

Consequences: every settings `GET` and `PUT` body adds `canEdit`, true when the caller may save. The pages lock their fields and the Save button when it is false. The role check for conventions moves from the store into the service call, so one check backs all four sections. A settings change still writes `config_changed` with the caller's id.

Requirements: REQ-065.
