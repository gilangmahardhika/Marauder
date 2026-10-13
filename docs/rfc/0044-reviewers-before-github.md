# RFC-0044: Reviewers before GitHub verification

Status: Accepted for implementation. Date: 2026-10-13.

This RFC adds decision `RFC-103` under the new REQ-090. A requirement may name any enabled member of its project's tribe as a reviewer, and GitHub is asked for that member's review once they have a verified GitHub login. It supersedes the RFC-054 statement it names. That statement stays in its file.

Sprint 67 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-103 | Reviewers are the enabled members of the project's tribe. The harness asks GitHub only for those with a verified login and records the others as waiting | RFC-054 "Reviewers are members of the project's tribe with a verified GitHub login." |

## RFC-103 Reviewers before GitHub verification

### Problem

The reviewer picker lists only tribe members with a verified GitHub login, and leaves out the requirement's creator. In a tribe where the creator is the only verified member, the picker is empty. The creator cannot name the colleague who should review, and the page does not say why.

### Decision

- The member who brought a requirement in, or a Lead Developer member, may name any enabled member of the project's tribe as a reviewer, except the creator. The limit stays at 15. A non-member, a disabled user, an unknown id, or the creator is refused.
- The intake response's `reviewerOptions` lists every enabled member of the project's tribe. A member without a verified login has an empty `githubLogin`.
- When the card's pull request is open, the harness asks GitHub for the reviewers with a verified login on the company host, without the owner and the company bot, as RFC-054 does.
- The `reviewers_requested` event adds `waiting`: the name, or else the email, of each named reviewer without a verified login. The event is written when either list is not empty. A request for no login makes no GitHub call.
- Each publish asks again, so a reviewer who verifies later is asked on the next push.
- The picker labels a member without a verified login "No verified GitHub login", and says such a member is asked on GitHub after they verify.

### Why

Who should review a requirement is known when it is written, before every colleague has set up GitHub. Naming them early keeps that choice with the requirement. Only a verified login is ever sent to GitHub, so a guessed or unverified login still never reaches a pull request.

## Set aside

- Inviting a user to the GitHub organization.
- Asking GitHub for an unverified login the user typed.
- Notifying a waiting reviewer inside the orchestrator.

## Tests

Sprint 67: `TestReviewersSetByCreator`, `TestReviewerOptionsUnverified`, `TestRequestReviewers`, `TestRequestReviewersOnlyWaiting`, and the reviewer picker, `ReviewersPanel`, and trace Vitest cases.
