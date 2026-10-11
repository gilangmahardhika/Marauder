# RFC-0011: Tribe admins

Status: Accepted for implementation. Date: 2026-10-11.

This RFC adds decision `RFC-057`. It supersedes earlier statements only where it names the id. Those statements stay in their files.

Sprint 19 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-057 | A tribe admin runs one tribe: its members, its projects, and their links | RFC-046, admin row for membership and project create; RFC-047, admin manages membership and project records; RFC-051, only a Lead Developer links |

## RFC-057 A tribe admin runs one tribe: its members, its projects, and their links

Tribe admin is a flag on one membership row, not a job role and not the company admin flag. A member with the flag may, inside that tribe only:

- list, add, and remove members;
- make another member a tribe admin, or remove the flag;
- create a project on the tribe, and choose its linked projects in the same request;
- save the linked projects of any project in the tribe.

A company admin keeps every power from RFC-046 and may also choose linked projects when creating a project. A Lead Developer member still saves the links of their project. Links stay one way and inside one tribe (RFC-051). A tribe admin cannot remove themselves or clear their own flag, so a tribe is not left without one by accident; a company admin can.

Creating a project and saving links in two calls failed for the person who creates projects: create needed the admin flag, and links needed a Lead Developer member. A company also grows past one admin who knows every team.

Set aside: a tribe admin who creates tribes, creates users, or edits company settings. Those stay with the company admin. Also set aside: a different job role per tribe (RFC-046).

Consequences: `tribe_members.admin` is a new column. The member JSON adds `tribeAdmin`, and `GET /api/me` adds `tribeAdminOf`, the tribe ids where the caller has the flag. `PUT /api/tribes/{id}/members/{userId}` sets the flag. `GET /api/tribes/{id}/candidates` lists users who could be added, for a tribe admin who cannot list every user. `POST /api/projects` accepts `links`, checked with the same rules as the links route and stored in the same transaction as the project, so a bad link stores no project.

Requirements: REQ-048, REQ-049, REQ-054, REQ-063.

Superseded in part by RFC-059 in [0013-project-settings-admins.md](0013-project-settings-admins.md). A tribe admin saves every setting of a project in their tribe, not only its links. A company admin saves every project's settings.
