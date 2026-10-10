# RFC-0008: Manual requirements

Status: Accepted for implementation. Date: 2026-10-10.

This RFC adds decision `RFC-053`. It supersedes earlier statements only where it names the id. Those statements stay in their files.

Sprint 16 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-053 | A member may add a requirement by hand, and every senior role may accept | RFC-018, intake only parses markdown; RFC-046, only a Product Owner accepts |

## RFC-053 A member may add a requirement by hand, and every senior role may accept

Intake has two sources. Ingest parses the PRD and RFC paths in the project clone, as before. A member of the project's tribe may also add a requirement by hand: an id, a statement, and acceptance lines. The id is optional. A typed id has the form `REQ-` and digits and must be unused in that project. A blank id gets the next free number, one above the highest `REQ-` id in the project, with at least three digits. The row is stored with `source_kind = 'manual'` and the id of the user who added it. It is not accepted when it is saved.

A member may edit or delete a manual requirement. A requirement parsed from the repository is edited in the repository. An edit that changes the statement or the acceptance list clears acceptance, as REQ-004 does for a source edit. A manual requirement that a card lists cannot be deleted.

Ingest never overwrites a manual requirement. When the repository defines an id that is already manual in that project, the manual row is kept and ingest reports the id as skipped.

A Lead Developer, Senior Developer, QA Lead, or Product Owner who is a member may accept requirements, manual or parsed. A Developer or QA member may not. Acceptance still stores the user id and writes a `runs` row.

A project without a PRD in its repository still needs a way to start work, and a small change does not deserve a document edit and a sync. One accept rule for every requirement keeps the Intake page to one button.

Set aside: a separate id series for manual rows. Ids stay `REQ-` so the reviewer checklist and the pull request body read the same. Also set aside: accepting a manual requirement on save. The person who wrote it is not the gate.

Consequences: `requirements.source_kind` gains `manual`, and `requirements.created_by` holds the adding user. The routes are `POST /api/projects/{projectId}/requirements` and `PUT` and `DELETE /api/projects/{projectId}/requirements/{id}`. The intake response adds `canAccept` for the caller and `createdBy` on each requirement. The ingest response adds `skipped`.

Requirements: REQ-003, REQ-047, REQ-055.
