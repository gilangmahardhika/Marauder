# RFC-0024: Self review on a requirement

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-070` for REQ-073. It supersedes in part RFC-054 ("The owner cannot approve their own card") and the per-project scope of RFC-067 ("The switch is per project"), each only for a card whose requirements all have Self review on.

Sprint 34 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-070 | A Lead Developer or an admin may turn Self review on for one requirement. A card whose requirements all have it is approved as if its project had Solo approval on | RFC-054 owner rule and RFC-067 project-only scope, for those cards only |

## RFC-070 Self review on a requirement

### Problem

Solo approval (RFC-067) is all or nothing for a project. A team project keeps the owner rule, yet a Lead Developer sometimes brings in a small requirement that nobody else needs to read, such as a copy fix or a config change. They own its card, so they cannot approve it, and turning Solo approval on for the project to get past one card drops the rule for every card.

### Decision

`requirements.self_review` is a boolean, default `false` (migration `00018`). Repository ingest does not write it, so a re-ingest keeps the value.

Only a Lead Developer or an admin may set it (`intake.CanSetSelfReview`). They may set it when they add a manual requirement, with `selfReview` in the `POST /api/projects/{projectId}/requirements` body, or later on any requirement with `PUT /api/projects/{projectId}/requirements/{id}/self-review` and the body `{"selfReview": true}`. Anyone else gets `403`. The update is scoped by `project_id`, and a missing requirement is `404`. Each requirement in the intake JSON has `selfReview` and `canSetSelfReview`.

A card is self-reviewed when it lists at least one requirement and every requirement it lists has `self_review = true` in the card's project (`app.CardSelfReview`). A card with no requirements, or one that names a missing requirement, is not.

For a self-reviewed card, the approval gate uses `GateConfig.Solo = true`, the same as a project with `approval.solo` on:

- `Approve` does not refuse the card owner, and the card's `actions` list keeps `approve` for the owner.
- `Approve`, `RequestChanges`, and `Reject` do not refuse a GitHub login equal to the company bot login.
- The other rules stay. The job role must allow the gate, the user must be a member of the project's tribe, the user must have a saved GitHub login, and the SHA must be the current head.

`approvals.independent` stays `false` for an approval by the owner or the bot login. `machine.Transition` and its evidence do not change. The card JSON adds `selfReview`, and the card page shows it in Details.

On GitHub nothing changes: the owner is already left out of the requested reviewers (RFC-054), and other named reviewers are still requested.

The New requirement page shows a Self review checkbox under Reviewers to a Lead Developer or an admin. The Reviewers panel on the requirement page shows the same checkbox to them, and a read-only "Self review: on" line to anyone else when the flag is on.

### Why

The flag sits on the requirement because that is where the owner comes from (RFC-054) and where the reviewers are named (RFC-054). A Lead Developer already may change any requirement's reviewers, so letting them say "no second reviewer" is the same trust. A Developer cannot waive their own review.

Every requirement on the card must have the flag, so a card that mixes a self-reviewed requirement with an ordinary one keeps the owner rule. The stored `independent = false` keeps the record honest, as with Solo approval.

### Set aside

- Letting the requirement's creator set the flag when they are a Developer. That would let anyone waive their own review.
- Requiring that no reviewers are named. Named reviewers still get the GitHub request, and any of them may approve too.
- A Self review flag on the card. Cards are written by the planner, and the flag would be lost when a card is re-planned.

### Tests

- `TestSelfReviewNeedsLead`: a Developer posting `selfReview: true` gets `403`; a Lead Developer's post stores it; the creator, as a Developer, gets `403` on the self-review route; an admin can turn it on and off; an unknown requirement is `404`.
- `TestSelfReviewOwnerApproves`, with Solo approval off: while the card also lists a requirement without the flag, the owner has no `approve` and approving is `403`. With only the flagged requirement, the owner has `approve`, approves to `approved`, and the approval is stored with `independent = false`.
- `TestCardSelfReview`: all requirements flagged gives true; mixed, none flagged, no requirements, or a missing requirement gives false.
- Vitest: `canSetSelfReview`; the Reviewers panel shows the checkbox to a lead and the read-only line to others.
