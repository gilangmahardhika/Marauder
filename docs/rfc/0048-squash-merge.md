# RFC-0048: Squash and merge when deploy is off

Status: Accepted for implementation. Date: 2026-10-14.

This RFC adds decision `RFC-111` under REQ-033. A project turns deploy on or off. When it is off, an approved card offers "Squash and merge" instead of "Deploy approved SHA", and a merged card is done. It supersedes no statement. NG-002 still holds: the harness never merges on its own, and the merge is one person's action.

Sprint 79 in [../plan/sprints.md](../plan/sprints.md) implements this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-111 | A project deploy switch, on by default. With it off, a Lead Developer, or the solo-approving owner, squash-merges an approved card's pull request with their own GitHub token, pinned to the approved SHA, and the card moves `approved -> done` | none |

## RFC-111 Squash and merge when deploy is off

### Problem

Many projects have no dev deploy. Their approved cards sat in `approved` forever, because the only next step on the card was Deploy, and nothing ran it. The person then merged on GitHub, and the board never learned the work was finished.

### Decision

The switch:

- The project `config.yaml` gains `deploy.enabled`. A missing key means on, so existing projects keep Deploy. `GET`/`PUT /api/projects/{projectId}/settings/deploy` carry it as `deployEnabled`; a `PUT` without the key leaves it as it is. Saving it is a `config_changed` field.
- With it off, `POST .../cards/{id}/deploy` is `422` "deploy is off for this project". The deploy fields stay editable and unused.

The action:

- With deploy off, an `approved` card drops `deploy` from `actions` and lists `merge` for a Lead Developer, and for the card owner when `approval.solo` is on (RFC-067). Other roles see neither.
- `POST /api/projects/{projectId}/cards/{id}/merge` checks the same rule (`403` otherwise), that deploy is off (`422`), and that the card is `approved` with an approved SHA (`409`).
- The server calls `PUT repos/{owner}/{repo}/pulls/{n}/merge` with `merge_method` `squash` and `sha` set to `approvedSha`, through `gh api` with the JSON in a file. GitHub refuses when the head is no longer the approved SHA, so the merged code is the approved code.
- The call uses the caller's own verified GitHub token on the company host, so GitHub shows the merge under that person. A caller with no verified token gets `422` "connect your GitHub account in Account to merge", and no call is made.
- On success the card moves `approved -> done` through `machine.Transition` with the merge commit SHA as evidence, and a `merge` event records `prUrl`, `approvedSha`, `mergeSha`, `login`, and `method`. No gate row is written; `dev_tests` stays for `verifying -> done`.
- When GitHub refuses (not mergeable, head moved, checks required), the route is `422` with GitHub's first line, redacted, and the card stays `approved`.

### Why

Merging is the step that finishes a card on a project without dev deploy, and it is already a human decision on GitHub. Doing it from the card keeps the board true without a webhook. The person's own token keeps GitHub's audit trail and branch protection honest: the company token would merge as the bot and could bypass review rules meant for people. Pinning to the approved SHA keeps the rule that only approved code moves on.

### Set aside

- A new `merged` status. `done` already means the work is finished, and a new status needs a migration and a board column for no new decision.
- Merging with the company token. The merge would not name the person who pressed it.
- Merge and rebase methods. Squash keeps one commit per card, which matches one card per pull request.
- Reading merges done on GitHub. A card merged outside the harness still needs a person on the card.

### Tests

- `TestApprovedToDone`: the edge needs a SHA and writes no gate.
- `TestDeployEnabledDefault`: a missing key is on, `false` round-trips, and a save without the key keeps it.
- `TestActionsDeployOff`: with deploy off, a Lead Developer gets `merge` and not `deploy`, a QA Lead gets neither, and the owner gets `merge` only with solo approval.
- `TestSquashMerge`: one `PUT repos/acme/widget/pulls/7/merge` with `squash` and the approved SHA, the user token in the environment only, the card `done`, and a `merge` event.
- `TestSquashMergeNoToken`: `422`, no GitHub call, and the card stays `approved`.
- `TestSquashMergeRefused`: GitHub's `405` comes back as `422`, and the card stays `approved`.
- `TestDeployRefusedWhenOff`: merge with deploy on and deploy with deploy off are both `422`.
- `CardActions.test.ts` and `settings/deploy/page.test.ts`: the Squash and merge button posts to `merge`, and the switch loads and saves `deployEnabled`.
