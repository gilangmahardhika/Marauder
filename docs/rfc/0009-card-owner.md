# RFC-0009: Card owner

Status: Accepted for implementation. Date: 2026-10-10.

This RFC adds decision `RFC-054`. It supersedes earlier statements only where it names the id. Those statements stay in their files.

Sprint 17 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-054 | A card belongs to the person who brought its requirements in, and its pull request is theirs | RFC-006, only the bot is excluded from approval; RFC-047, jobs use a bot token for push; RFC-049, jobs never use a signed-in user's token |

## RFC-054 A card belongs to the person who brought its requirements in, and its pull request is theirs

Every requirement records the user who brought it in: the member who added it by hand, or the member who ran the ingest that first stored it. A re-ingest keeps that user. The CLI stores no user. When the planner creates a card, the harness reads that user from the card's requirements and stores it as the card owner. The planner does not pass an owner, and a card whose requirements were brought in by different users is rejected, so the planner splits them.

The owner's GitHub login is the one GitHub reports for the owner's token. The token test runs `gh api user` with the token in the child environment and stores that login and the numeric account id. Saving a new token clears the check. Two users cannot hold the same verified login on one host.

Jobs commit as the owner, with the owner's login as the name and `<id>+<login>@users.noreply.github.com` as the email. They push the card branch and open or update the pull request with the owner's token, in the child environment only. Clone and fetch keep the company token. When the owner is missing, disabled, unverified, or has no token, the job does not push and the card moves to `blocked` with reason `owner_github`. It never falls back to the company token for a push.

The member who brought a requirement in, or a Lead Developer member, may name reviewers for it. Reviewers are members of the project's tribe with a verified GitHub login. When the card's pull request is open, the harness requests the union of its requirements' reviewers, without the owner and the company bot. A failed request writes an event and does not change the card.

The owner cannot approve their own card. The check compares user ids. Request changes, reject, and deploy keep their RFC-046 rules. A review on GitHub never counts as `approved`. The SHA gate stays the only approval.

A pull request opened by a shared bot hides who asked for the change, and the reviewers a developer wants are known when the requirement is written, not when the pull request opens.

Set aside: pushing with the company token and naming the owner only in a `Co-authored-by` trailer. Two identities on one pull request make the author unclear, and a missing owner token would pass silently. Also set aside: picking the owner on the card. The model would then choose a credential.

Consequences: `cards.owner_id` names the owner, and the card JSON adds `owner`. `requirement_reviewers` holds reviewers per requirement. `user_github` adds `github_id` and `verified_at`. The blocked reason `owner_github` unblocks to `ready`. The intake response adds `reviewers` on each requirement and `reviewerOptions`. The route is `PUT /api/projects/{projectId}/requirements/{id}/reviewers`. The owner token joins the redaction list of the job that decrypts it.

Requirements: REQ-051, REQ-056, REQ-057, REQ-058.
