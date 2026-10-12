# RFC-0021: Solo approval

Status: Accepted for implementation. Date: 2026-10-12.

This RFC adds decision `RFC-067` for REQ-070. It supersedes in part RFC-054 ("The owner cannot approve their own card"), RFC-049 ("Approval is rejected when the caller's GitHub login equals the company bot login"), and the bot rule of RFC-046 ("The project's bot login cannot approve"), each only for a project that turns the switch on.

Sprint 30 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-067 | A project setting lets the card owner and the company bot login approve, and such an approval is stored as not independent | RFC-054 owner rule, RFC-049 and RFC-046 bot rule, for that project only |

## RFC-067 Solo approval

### Problem

The first real card (`regol-app/scbd-solid-web#34`) reached `human_review` on an install with one person. That person brought the requirement in, so they own the card, and their one GitHub account is also the company token, so their login is the bot login. Both rules refuse their approval, and there is no second member to approve. RFC-0017 already noted that the owner and the bot can be one account; it did not say who approves then.

### Decision

The project's `config.yaml` has `approval.solo`, default `false`. The Deploy settings route reads and writes it as `soloApproval`, and the Deploy page shows it as the Solo approval checkbox in the Board panel. Saving writes the usual `config_changed` event, which names `soloApproval`.

While `approval.solo` is `true` for the card's project:

- `Approve` does not refuse the card owner, and the card's `actions` list keeps `approve` for the owner.
- `Approve`, `RequestChanges`, and `Reject` do not refuse a GitHub login equal to the company bot login.
- The other rules stay. The job role must allow the gate, the user must be a member of the project's tribe, the user must have a saved GitHub login, and the SHA must be the current head.

`approvals.independent` is `true` by default. An approval by the card owner, or by a login equal to the bot login (compared without case), stores `false`. `machine.Transition` and its evidence do not change.

The switch is per project, so a team project keeps both rules while a personal project turns them off.

### Why

Approval moves the card to `approved` only. It does not merge (NG-002), and deploy to dev still needs a Lead Developer to press Deploy. On a one-person install the choice is between an approval by the only person and no approval at all. The stored `independent = false`, like `review_independent` (RFC-065), keeps the record honest.

### Set aside

- Making `botLogin` editable. The Test sets it from the token, and an edited value would drift from the account that writes the pull request.
- An admin override. The PRD says an admin does not approve unless the job role allows it, and an override would leave no record that the approval was not independent.
- Reassigning the card owner. RFC-054 rules out picking the owner, and a new owner without a token blocks any rework with `owner_github`.

### Tests

`TestSoloOwnerApproves`:

- with the switch off, the owner's approval is `403`;
- after an admin saves `soloApproval: true`, the owner's `actions` has `approve`;
- the owner, whose login is the bot login, approves to `approved`, with `approvals.independent = false`.

`TestOwnerCannotApprove` and `TestBotApprover` keep the default.
