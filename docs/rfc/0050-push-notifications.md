# RFC-0050: Push notifications

Status: Accepted for implementation. Date: 2026-10-14.

This RFC adds decision `RFC-113` for REQ-095. The server pushes board changes to open pages over one server-sent events stream per tab, stores personal notifications for the people who must act on a card, and sends those as browser push notifications to people who turned them on. It supersedes no statement. The trace stream (RFC-036, RFC-088) stays as it is.

Sprints 82 to 84 in [../plan/sprints.md](../plan/sprints.md) implement this file.

## Decision index

| Id | Decision | Supersedes |
| --- | --- | --- |
| RFC-113 | PostgreSQL triggers `pg_notify` on writes to `events`, `cards`, and `runs`. `serve` listens, reads events after a stored `seq` cursor, writes notifications per a fixed trigger table, and pushes them and the touched cards over `GET /api/stream`, and as Web Push (RFC 8291, VAPID) with a minimal payload | none |

## RFC-113 Push notifications

### Problem

The board polled every 3 seconds while anything ran, and the card page never refreshed. Nobody learned that a card waited for them unless they opened the board. Writers live in several processes (`serve`, the worker, `run-next`, the runner API), so `serve` cannot learn of a change from memory.

### Decision

The wake-up:

- Migration 31 adds triggers: `AFTER INSERT` on `events`, and `AFTER INSERT OR UPDATE` on `cards` and `runs`. Each calls `pg_notify('orchestrator', ...)` with `{"t": table, "projectId": ..., "cardId": ...}`. Every writer is covered, whatever process it runs in.
- `serve` holds one dedicated connection that runs `LISTEN orchestrator`. A notification only wakes it; nothing is lost when no listener runs.
- The listener reads `events` with `seq` after the stored `notify_cursor`, minus a 10-second overlap so a transaction that committed late is not skipped. It also wakes every 30 seconds. Touched card ids are debounced per project for 250 ms and sent to the open streams.

The stream:

- `GET /api/stream?project={id}` is server-sent events read through `fetch` with the bearer in a header, like the trace stream. Frames: `cards` (the board JSON of each touched card in the viewed project, with the viewer's own `actions`), `board` (`readyCount`, `running`, `canRun`), `notification` (the viewer's new rows), `ping` every 15 seconds, and `resync` when the viewer fell behind. A viewer that is not a member of the project gets `404`; notifications still need no project.
- The board stops polling while its stream is open; it reads the board once after a reconnect, a `resync`, or the tab becoming visible.

Who is notified (a person is never notified of their own action):

| Trigger | Who | Title | Browser push |
| --- | --- | --- | --- |
| `-> human_review` | The card's requirement reviewers, or when it has none, every member who may approve. The owner only with solo approval or self review | Review needed | yes |
| `-> approved` | Who can ship: Lead Developer members with deploy on; with deploy off, those `CanMerge` allows | Ready to deploy, or Ready to merge | yes |
| `-> approved` | The owner, when not already notified above | Approved | no |
| `-> changes_requested` | The owner | Changes requested | yes |
| `-> blocked` | The owner, and Lead Developer members for `deploy_failed`, `health`, or `deploy_config` | Blocked: {reason label} | yes |
| `-> blocked` with `owner_github` | The owner | Connect your GitHub account | yes |
| AI review `failed` | The owner, and members who may retry it | AI review gave no verdict | yes |
| `-> done` | The owner | Done | no |
| `-> wont_fix`, and reopen | The owner | Closed as won't fix, or Reopened | no |

- A notification row holds `kind`, `title`, `body` (the card title and project name, at most 120 characters), `projectId`, `cardId`, `url` (`/cards/{id}?project={projectId}`), `createdAt`, and `readAt`.
- `UNIQUE (event_id, user_id, kind)` and `ON CONFLICT DO NOTHING` make one event notify a person once. Only the process whose insert wrote the row pushes it, so two `serve` processes do not send twice.
- Reading notifications lists only rows in projects the person can still see.

Browser push:

- A person turns it on in Account. The browser subscribes with the server's VAPID public key, and the server stores the endpoint and keys in `push_subscriptions`.
- The VAPID key pair is made on the first `serve` start. The private key lives in `<data>/secrets.json` as `vapidPrivateKey` and is never returned; `GET /api/push/key` returns the public key only. The JWT `sub` is `mailto:` the first admin's email.
- The payload is encrypted with RFC 8291 `aes128gcm` and sent with an RFC 8292 VAPID JWT, through the standard library and `netguard.Client`. It holds only `title`, `body`, `url`, and `tag` (the card id). It never holds a reason, note, code, log, or token, because the payload passes through the browser vendor's push service.
- An endpoint must be `https` on a known push service host (`fcm.googleapis.com`, `*.push.services.mozilla.com`, `*.notify.windows.com`, `web.push.apple.com`, `*.push.apple.com`), else `422`. A `404` or `410` reply, or five failures in a row, deletes the subscription.
- The service worker shows the notification and opens or focuses the card. It caches nothing and calls no API.

### Why

Triggers catch every writer without each code path remembering to publish. Treating `NOTIFY` as a wake-up and the `seq` cursor as the truth keeps the design correct when `serve` restarts or `NOTIFY` is dropped. One stream per tab costs less than polling and reaches the card page too. Notifying only the people a card waits on keeps the bell worth reading. Web Push needs no third-party account, and a minimal payload keeps card details behind sign-in.

### Set aside

- WebSockets. The UI only listens, and server-sent events already work through the bearer header.
- A hosted push or messaging service. It needs an account and sees more data.
- Email and chat channels, and per-kind preferences.
- Pushing Intake planner progress and runner heartbeats; those pages keep their polls.

### Tests

- `TestNotifyTriggers`, `TestNotifyCursor`, `TestRecipients`, `TestStreamCards`, `TestStreamNotification`, `TestNotificationsRead`.
- `TestWebPushEncrypt` (RFC 8291 Appendix A), `TestVAPIDJWT`, `TestPushEndpointGuard`, `TestPushGone`, `TestVAPIDKeySecret`.
- Vitest: `stream.test.ts`, the board without polling, the bell, the Account notifications panel, and the card page `?project=` switch.
