# RFC-0058: Board settings leave the Deploy page

Status: Accepted for implementation. Date: 2026-10-15.

This RFC adds decision `RFC-126` under REQ-065. It supersedes the statements it names below. What each setting does, its default, its validation, and where it is stored do not change.

Sprint 98 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-126 | The PRD and RFC paths are saved with the project repository, the implementation cap and Solo approval on a new Workflow settings page, and the Deploy settings keep only the dev server, the checks, and the deploy switch | RFC-067 "The Deploy settings route reads and writes it as `soloApproval`, and the Deploy page shows it as the Solo approval checkbox in the Board panel"; RFC-059 "A project's settings are its repository, linked projects, deploy settings, and conventions"; RFC-0001 "What stays configurable", "Parallel implementation cap: 2, in deploy settings" |

## RFC-126 Board settings leave the Deploy page

### Problem

The Deploy page had a Board panel with the implementation cap, the PRD path, the RFC path, Deploy approved cards, and Solo approval. Each field was added to the deploy route because that route already wrote the project's `config.yaml`. Only Deploy approved cards is about deploying. A person looking for where intake reads its docs, or who may approve, did not look on the Deploy page. One `PUT` also wrote all of them, so a page that showed some of them would reset the rest.

### Decision

- `GET`/`PUT /api/projects/{projectId}/settings/repository` carry `prdPath` and `rfcPath` (`paths.prd`, `paths.rfc`). A `PUT` without a path keeps the stored one. The paths are checked before the repository is saved, so a bad path saves nothing. The Repository page shows them in a Docs panel.
- `GET`/`PUT /api/projects/{projectId}/settings/workflow` carry `implementationCap` (`scheduler.implementationCap`) and `soloApproval` (`approval.solo`), with `canEdit`. A `PUT` without a field keeps the stored one. Saving writes `config_changed` with section `workflow`. The new Workflow page in the Project sidebar group shows them.
- `GET`/`PUT /api/projects/{projectId}/settings/deploy` no longer carry those four fields, and a deploy save leaves them as they are. `deployEnabled` stays on the deploy route and the Deploy page (RFC-111).
- A project's settings are its repository with its doc paths, linked projects, deploy settings, workflow, and conventions. RFC-059 decides who saves each one, unchanged.
- `orchestrator ingest` reads the paths from the project's `config.yaml`, as the ingest route does.

### Why

Each page now holds what a person looks for there: the paths sit with the repository they point into, and the cap and approval rule sit together as how the project's board works. Each route writes only the keys it shows, so saving one page cannot reset another.

### Set aside

- Putting the doc paths on the Intake page. Intake is a work page any member uses, and the paths are a setting only a settings editor saves.
- Keeping one route with partial `PUT`s for every field. Separate routes keep each page's body and `config_changed` section clear.

### Tests

- `TestSettingsSectionsKeepEachOther`: a deploy save keeps the cap, the paths, and Solo approval; a workflow save keeps the paths and `deploy.enabled`; a paths save keeps the cap and Solo approval.
- `TestWorkflowSettings`: the defaults are cap 2 and Solo approval off; a Lead Developer saves both; a cap of 9 is `422` on `implementationCap`; a Developer member's save is `403`; the deploy route returns neither field.
- `TestRepositoryDocPaths`: a save returns the paths; a bad path is `422` on `prdPath` and leaves the repository unchanged; a save without paths keeps them.
- `TestSoloOwnerApproves` saves `soloApproval` through the workflow route.
- `settings/workflow/page.test.ts`, `settings/repository/page.test.ts`, and `layout.test.ts`: the Workflow page loads and saves both fields, the Repository page saves the paths, and the sidebar links to Workflow.
