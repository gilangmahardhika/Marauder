# RFC-0014: GitHub confirms a project's repository

Status: Accepted for implementation. Date: 2026-10-11.

This RFC adds decision `RFC-060`. It does not supersede an earlier statement. RFC-050 still says a project names one repository in the company organization, and this file says how the server makes sure it does.

Sprint 23 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-060 | GitHub confirms a project's repository and base branch before they are saved | None |

## RFC-060 GitHub confirms a project's repository and base branch before they are saved

Creating a project, and saving a project's repository settings with a new repository or base branch, asks GitHub first. The server calls `gh api repos/{org}/{repo}` and `gh api repos/{org}/{repo}/branches/{base}` with the company token in the child environment, as for the repository list (RFC-049). The save goes through only when the repository exists, belongs to the company organization, is not archived, and has that branch. The name GitHub reports is the one stored.

No company organization or token is a `422` on `repo` that asks an admin to set it. GitHub's answer that the repository or branch does not exist is a `422` on `repo` or `baseBranch`. Any other failed call is `502`, and nothing is saved.

The repository picker offers only the repositories the organization list returned. There is no free-text fallback. Archived repositories are listed but cannot be picked.

The API accepted any name that matched the name pattern, and the pages offered a text field when the list did not load. A typo stored a project that failed on its first Sync, long after the person who made it had left the page.

Set aside: searching GitHub as the user types. The search index can lag a new repository by minutes, while the organization list is current. Also set aside: saving without the check when GitHub is unreachable.

Consequences: `POST /api/projects` and `PUT /api/projects/{id}/settings/repository` may return `502`. A save that keeps the same repository and branch makes no GitHub call. The CLI `--repo` path for the fixture does not call GitHub.

Requirements: REQ-066.
