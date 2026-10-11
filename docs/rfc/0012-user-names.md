# RFC-0012: User names

Status: Accepted for implementation. Date: 2026-10-11.

This RFC adds decision `RFC-058`. It does not supersede an earlier statement. RFC-046 still defines the user row, and this file adds one field to it.

Sprint 20 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-058 | A user has a display name, and pages show it instead of the email | None |

## RFC-058 A user has a display name, and pages show it instead of the email

Every user has a display name. An admin types it when creating the user, and may change it on the user's edit page. Each user may change their own name on the Account page. The name is trimmed, 1 to 80 characters, has no control characters, and need not be unique. Saving an empty name is rejected.

Wherever a page lists a person, it shows the name first: the board owner and reviewers, the card page, the signed-in user menu, Users, tribe members, and the reviewer pickers. The email stays as the second line where there is room. A user created before this RFC has no name until someone sets one, and shows the verified GitHub login, then the email, until then.

The board showed email addresses, which are long and say little about who the person is. A GitHub login is not set for every user, and is not a person's name either.

Set aside: using the name to sign in, to match "Mine" on the board, or as a GitHub identity. The email stays the sign-in id and the match key. The verified login stays the GitHub identity (RFC-054).

Consequences: `users.name` is a new column, empty for older rows. The user, member, candidate, card owner, and reviewer objects add `name`. `POST /api/users` requires `name`, and `PUT /api/users/{id}` accepts it. `PUT /api/me` with `{ "name": "..." }` changes the caller's own name.

Requirements: REQ-064.
