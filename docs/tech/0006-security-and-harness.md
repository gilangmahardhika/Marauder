# Security and harness hardening

Status: Accepted for implementation. Date: 2026-10-10. Revised after review.

Decisions are in [../rfc/0002-hardening.md](../rfc/0002-hardening.md) and [../rfc/0003-review-corrections.md](../rfc/0003-review-corrections.md). This file is how to build them. The worker shape is in [0003-backend.md](0003-backend.md). Where 0003 is looser, this file wins. Schema and field rules are in [0002-data-and-api.md](0002-data-and-api.md).

## Trust boundary

| Source | Trust | May influence |
| --- | --- | --- |
| Project settings and the service secrets file | Trusted to name commands, hosts, images, and tokens | Deploy argv, dev hosts, check argv, image digest |
| CLI that can read the service data directory | Trusted as break-glass for bootstrap and the fixture | Bootstrap, and a gate only with `--as` |
| Loopback or cloud API after a live bearer | Trusted as that user, and only for the actions their role and membership allow | The gates in RFC-046 |
| Target repo files, diffs, PR comments, CI output, logs | Untrusted | Text inside a pack |
| Model output | Untrusted | Tool name and arguments, which the harness then filters |
| Check processes, local and dev | Untrusted code | Their own exit code and output, from inside the sandbox |
| Fixture mode | Trusted only when the resolved repo ends with `/testdata/fixture-repo` | `sandbox` off, `http://127.0.0.1` for health and dev URL |

The model is outside the boundary. Agreement from the model does not cross it.

## Fail closed

| Condition | Result |
| --- | --- |
| Data directory inside the repo, or the reverse | Process exits before opening the database |
| Tool name not in the profile | Error to the model, no exec |
| Path escapes the root, or a symlink does | Tool error, no write |
| Check command fails parsing, or its first word is not allowed | `sandbox_config`, check does not run |
| Card check differs from settings | Card is blocked with `source_changed` |
| `execChild` argv names a shell | Process refuses to start |
| Sandbox image has no digest on a real repo | `sandbox_config`, check does not start |
| Deploy or rollback command fails the path rule | `deploy_config`, no exec |
| `devHosts` empty, or a URL host not listed | `deploy_config`, no probe |
| Resolved health address is loopback, link-local, multicast, or unspecified | Probe fails, rollback runs |
| Secret appears in a pack or event | Write the redacted bytes, or refuse the pack if redaction cannot run |
| `--addr` is not loopback, and `ORCHESTRATOR_CLOUD` is unset | Process exits before listen |
| `Host` header not allowed | `403` |
| Push refspec destination is not the card branch | No network call |
| Card owner missing, disabled, unverified, or without a token | `owner_github`, no push and no pull request call. The company token is never used instead (RFC-054) |
| Approver is the card owner | `403`, no approval row |
| Rule or skill text asks for a tool, a command, or a status | Nothing changes. The text sits inside the untrusted markers, and the tool list is the profile constant (RFC-055) |
| Rules and skills exceed the 64 KiB section cap | The blocks that do not fit become `omitted` notes. The pack never exceeds its cap |
| `create_card` names a skill the project has not enabled | No card is stored (RFC-056) |
| Stack detection meets a symlink or a file over 1 MiB | The link is not followed, and the file is read only to 1 MiB |
| Event `UPDATE` or `DELETE` | Postgres trigger aborts the statement |
| Deploy SHA is not 40 or 64 hex digits | `deploy_config`, no exec |
| Transition status changed after read | `409`, no side effect |
| Worker lock held by another process | `serve` exits, `run-next` only queues |

## Data directory

Startup resolves both paths with `filepath.EvalSymlinks` and compares them with a separator-aware prefix check. Equal paths count as containment. The directory is created `0700`. An existing directory with group or world bits exits with a message naming `chmod 700`.

`config.yaml` and `secrets.json` are `0600`. Both are written by temp file, `fsync`, and rename in the same directory.

## Untrusted packs

`contextpack.Build` returns one system string and one user string.

The system string is a constant in the binary, one per profile. Its SHA-256 is stored on the attempt as `system_hash`. A test fails if the constant names a tool that the profile does not have.

The user string is sections in this order:

```text
CARD
<card fields from the board>

UNTRUSTED START
<company rules and the card's skills>
<requirements, diff, files, log tail, comments>
UNTRUSTED END
```

Company rules and skills are written by an admin, not by the target repo, and they still sit inside the markers (RFC-055). They are guidance for the code the model writes. They cannot widen the tool list, name a check command, or move a card. The system string says so in one fixed sentence. Each rule or skill block is capped at 16 KiB, and the blocks together at 64 KiB, inside the 256 KiB untrusted cap.

The system string tells the model that text between the markers is data. Builders strip any copy of the marker lines from untrusted text before wrapping it. They refuse a section over the caps below. They run `redact` on the user string before return. They do not append model text from an older attempt.

The provider client drops any tool call whose name is not in the profile snapshot for that request. A dropped call is a `tool_call` event with `rejected: true` and no dispatch.

## Path rule

```text
func safePath(root, requested string) (string, error)
```

1. Reject an empty path, a NUL byte, and an absolute path.
2. Clean with `path/filepath.Clean`.
3. Join under `root`. The result must have `root` as a prefix after both are cleaned.
4. Resolve symlinks on the deepest existing ancestor. The resolved path must still have the resolved `root` as a prefix.
5. Reject a final path whose relative form is `.git` or `.orchestrator`, or starts with `.git/` or `.orchestrator/`.
6. `read_doc` uses the configured PRD and RFC directories as its only roots.
7. `read_linked_file` uses the export of one linked project's base SHA as its only root (RFC-051). The model names the linked project by id, and an id that is not a link of the card's project is an error. Shared paths saved on a link pass steps 1, 2, and 5 when they are saved, and the full check when they are read.

`edit_file` refuses a target that exists and is not a regular file, and refuses writes over 1 MiB. `read_file` returns at most 256 KiB and records `truncated: true` when it cuts. The data directory is outside the repo, so it is outside every root.

## Checks

### Source

On card insert, copy `checks.local` and `checks.dev` from settings onto the card. `create_card` has no command field. A command field in the payload is ignored and logged as a rejected `tool_call`.

Changing `checks.local` or `checks.dev` in settings updates cards in `backlog` and blocks cards past `backlog`, except `done`, with `source_changed`.

### Parsing

A check string is split on runs of ASCII spaces. Refuse the string when it contains a newline, a tab, or any of these characters:

```text
; & | $ ` < > ( ) { } [ ] * ? ! ~ " ' \
```

The first word must be in `checks.allowExecutables`. It is a bare name resolved inside the image, not a host path. The result is the argv passed after the image in the container command. An empty string means no check: health-only for `devCheck`, and `sandbox_config` for `localCheck` on a non-manual card.

### Exports

The sandbox never mounts the worktree (RFC-037).

| Check | Export |
| --- | --- |
| `run_local_check` tool | Copy of the working tree. Skip `.git`. Copy regular files and directories only. Do not follow symlinks; recreate them as symlinks only when the target stays inside the tree, otherwise skip them |
| Gating local check | `git archive <commit_sha>` piped to an in-process tar reader that refuses absolute names, `..`, and links outside the export |
| Dev check | Same as gating, for `deployed_sha` |

Exports live under `<data>/exports/<card-id>-<sha-or-attempt>/` and are deleted when the check ends. `Reconcile` deletes leftovers.

## execChild

```text
type Child struct {
    Argv    []string      // Argv[0] is an absolute path
    Env     []string      // full environment, not merged with os.Environ
    Dir     string
    Timeout time.Duration
    MaxOut  int
}
```

`Argv[0]` must be absolute and must be one of: the `git`, `gh`, or sandbox engine path resolved at startup, the deploy or rollback command from the settings snapshot, or the orchestrator binary for askpass. A basename of `sh`, `bash`, `zsh`, `dash`, `fish`, `cmd`, or `powershell` is refused. In fixture mode with the sandbox off, the check's first word is resolved once against `/usr/local/bin:/usr/bin:/bin`.

Do not pass `os.Environ()` into `Env`. Build it.

| Child | Env | Timeout | MaxOut |
| --- | --- | --- | --- |
| `git` | `PATH` with only the binary's directory, `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`, `GIT_TERMINAL_PROMPT=0`, `HOME=<data>/empty-gh`. For network calls also `GIT_ASKPASS`, `ORCH_ASKPASS=1`, `GH_TOKEN` | 2 min | 1 MiB |
| `gh` | `PATH` as above, `GH_TOKEN`, `GH_HOST`, `GH_CONFIG_DIR=<data>/empty-gh`, `GH_PROMPT_DISABLED=1`, `NO_COLOR=1` | 2 min | 1 MiB |
| Sandbox engine | `PATH` with only the engine's directory, `HOME` and `XDG_RUNTIME_DIR` from the operator session so rootless Podman works | 10 min | 1 MiB |
| Inside the container, local check | `PATH=/usr/local/bin:/usr/bin:/bin`, `HOME=/tmp`, `CI=1` | | |
| Inside the container, dev check | Same, plus `DEV_URL` and `ORCH_SHA` | | |
| Deploy and rollback | `PATH=/usr/bin:/bin`, `DEPLOY_TOKEN`, `DEV_URL`, `HOME` an empty dir under the data directory | 10 min | 1 MiB |

Container env is passed with `--env NAME=value` flags. Tokens never appear in container env. On timeout or oversize, kill the process group (`Setpgid` at start, negative pid on kill). Store exit `124` for timeout and `125` for oversize. Those are failed checks, not passes.

## Sandbox

Real repo targets use this Podman shape. Docker uses the same flags, minus `relabel`.

```text
podman run --rm
  --network=none                      # local checks
  --cap-drop=ALL
  --security-opt=no-new-privileges
  --pids-limit=256
  --memory=2g
  --cpus=2
  --read-only
  --tmpfs /tmp:rw,nosuid,size=256m
  --mount type=bind,src=<export>,dst=/work,relabel=private
  --workdir /work
  --user 65532:65532
  --env CI=1
  <image>@sha256:<digest>
  <check argv>
```

Dev checks replace `--network=none` with the engine's default rootless network and add `--env DEV_URL=<devUrl> --env ORCH_SHA=<sha>`. They run only for a SHA a person approved and the harness deployed. Egress is not restricted by host in v1 (RFC-037).

`sandbox.image` must contain `@sha256:` and 64 hex chars. The harness passes `--pull=never` once the image is present, and runs `podman pull <image>` only from the settings test. It never mounts the home directory, the data directory, the worktree, or the container socket.

The sandbox off mode is accepted only in fixture mode.

## Deploy commands

Checked at settings save and again inside the deploy lock before exec (RFC-038):

1. Absolute path. `filepath.EvalSymlinks` result is outside the resolved repo and data directory.
2. `Lstat` after resolving: a regular file, owned by the current uid, mode has an execute bit, and no group or world write bit.
3. Its parent directory has no group or world write bit.

Argv is `[command, sha]` for deploy and `[command, previousSha]` or `[command]` for rollback. `sha` matches `^[0-9a-f]{40}$` or `^[0-9a-f]{64}$`.

## Health probe

```text
func probe(raw string, hosts []string, fixture bool) error
```

1. Parse the URL. Reject userinfo and a scheme other than `https`. In fixture mode also allow `http` with host `127.0.0.1`.
2. The lowercased host must equal an entry in `deploy.devHosts`.
3. Resolve once. Reject loopback, link-local (`169.254.0.0/16`, `fe80::/10`), multicast, and unspecified addresses, including IPv4-mapped IPv6 forms. Private ranges are allowed for a listed host. Fixture mode allows `127.0.0.1` only.
4. Dial the resolved address with a custom `DialContext`, so DNS cannot change between the check and the request. TLS `ServerName` is the host, and the certificate is verified.
5. 5 second timeout. Redirects are refused (`CheckRedirect` returns an error).
6. Read at most 64 KiB. Success is HTTP 200. Store the status and byte count, not the body. The body never goes to a model.

`devUrl` passes steps 1 to 3 at save time. The dev check container reaches it on its own network, outside this function.

## Secrets

Load secrets once per job into that project's snapshot, never into a global that HTTP handlers can format. The list is `databaseUrl`, `tokenKey`, the caller's GitHub token if it was decrypted, the card owner's GitHub token when the job decrypted it (RFC-054), the company GitHub token, each model key, and the deploy token. Password hashes are not redacted by substring search. They are never written to a pack, an event, or a log. `redact(s string, secrets []string) string` replaces each non-empty secret with `[redacted]`. Apply it to:

- event payloads
- pack user strings
- tool results before they re-enter the model
- HTTP error messages
- child output before it reaches the log file

A secret shorter than 8 characters is refused at save, and a job refuses to start if one is loaded. Short values make redaction collide with ordinary text.

Children receive a secret only in `Env`, never in `Argv`.

## Event log

The insert-only triggers are part of `00001_init.sql` (0002). `trace.Append` is the only writer. Resume reads. Nothing repairs history by editing a row.

## Locks and transitions

Card lock acquire:

```sql
INSERT INTO locks (card_id, holder, acquired_at)
VALUES ($1, $2, $3)
ON CONFLICT (card_id) DO NOTHING;
```

Proceed only when `RowsAffected` is `1`. `holder` is a new random id for that job. Release:

```sql
DELETE FROM locks WHERE card_id = $1 AND holder = $2;
```

A release that matches zero rows appends `assertion_failed`. It does not delete someone else's lock. The deploy lock:

```sql
UPDATE deploy_lock
SET card_id = $1, holder = $2, acquired_at = $3
WHERE project_id = $4 AND card_id IS NULL;
```

Release sets the three columns to null `WHERE project_id = $4 AND holder = $5`. Another project's row is not updated.

Transitions are compare-and-swap on `id` and `status`, and the `WHERE` also includes `project_id` (0003). Only the holder of that project's worker lock runs `Reconcile` for that project, which is the only code that clears locks it does not hold (RFC-048).

## API session

| Control | Value |
| --- | --- |
| Listen | `127.0.0.1` or `::1` unless `ORCHESTRATOR_CLOUD=1` |
| Cloud bind | Configured address. TLS terminates in front of the process |
| `Host` header | Loopback names, or the configured allowlist in cloud mode, else `403` |
| Password | argon2id hash. Minimum 12 characters. Never logged |
| Compare | `subtle.ConstantTimeCompare` on bearer SHA-256, and a password verifier that does not branch on secret length in the HTTP response |
| Failures | 5 bad passwords for one email, then `429` for 1 minute |
| Bearer | 32 bytes from `crypto/rand`. The row stores SHA-256. 12 hour absolute expiry, checked on every request |
| GitHub tokens | AES-256-GCM with `tokenKey` from the service secrets file. GET returns `tokenSet` only |
| Body cap | 1 MiB, then `413` |
| Header timeout | 30 seconds on the HTTP server |
| CORS | Exact configured origin, no `*`. A mismatched `Origin` is `403` |
| Hidden project | `404` for a non-member and for an unknown project id |

The worker uses a context that is not the HTTP request context, so the 30 second cap does not kill a check.

## Git

Every git argv starts with:

```text
git -c core.hooksPath=<data>/empty-hooks
    -c core.fsmonitor=false
    -c protocol.file.allow=never
    -c credential.helper=
    -c commit.gpgSign=false
```

Calls on a worktree add `--git-dir=<worktree_gitdir> --work-tree=<worktree_path>` from the card row. The harness never lets git discover the admin directory by reading the worktree's `.git` file after creation. `<data>/empty-hooks` is an empty directory created at startup.

Network calls (`clone`, `fetch`, `push`) use an explicit URL built from settings: `https://<host>/<org>/<repo>.git`, where `host` and `org` come from the company GitHub account and `repo` from the project (RFC-049, RFC-050). `owner` and `repo` match `^[A-Za-z0-9._-]+$`. Remotes configured in the repo are not used. A remote a model added cannot redirect a push.

Authentication uses askpass. `GIT_ASKPASS` is the absolute path of the orchestrator binary. With `ORCH_ASKPASS=1`, the binary prints `x-access-token` for a prompt starting with `Username` and the value of `GH_TOKEN` for a prompt starting with `Password`, then exits. `credential.helper=` empties the helper list so the operator's keychain is not consulted.

Push:

```text
git push <url> refs/heads/orch/<card-id>:refs/heads/orch/<card-id>
```

The card id matches `^card_[0-9a-f]{12}$`. Build the refspec in code. Do not format it from model text.

Before and after each implement job, read local `refs/heads/<baseBranch>`. A change appends `assertion_failed` and blocks with `base_moved`. Remote base movement is normal and is not checked.

## UI

The SvelteKit build is static. It has no server code (0004).

| Control | Rule |
| --- | --- |
| CSP | `kit.csp` in hash mode: `default-src 'self'`, `script-src 'self'`, `style-src 'self'`, `img-src 'self'`, `connect-src 'self' http://127.0.0.1:8787`, `object-src 'none'`, `base-uri 'self'`, `form-action 'self'`, `frame-ancestors 'none'` where the server can send headers |
| Markup | No `{@html}`. Model text, logs, and PR bodies render as text |
| Bearer | `sessionStorage` key `orchestrator.bearer`. Never `localStorage`, cookies, or URLs |
| Secrets | Secret inputs are `PasswordInput`, start masked, use `autocomplete="off"`, and are cleared after save. The eye toggle reveals only typed text and masks again when the value is cleared |
| Dependencies | `package-lock.json` committed. Install with `npm ci`. Versions pinned exactly. `npm audit --omit=dev` in the release order |

## Harness assertions

These run in the worker, not only in tests. A failed assertion appends `assertion_failed` with the name and blocks the card with `assertion`, except where the table names another reason.

| Assertion | When |
| --- | --- |
| Profile snapshot hash matches the binary's profile | Before the provider call |
| System prompt hash matches the constant | Before the provider call |
| No secret substring in the user pack | After `redact` ran |
| Attempt has no parent transcript bytes | At attempt start |
| `approved_sha` equals the refreshed head before deploy exec | Inside the deploy lock |
| Deploy event exists before the deploy process starts | Same function, in that order |
| Health event exists for every deploy | `Reconcile` on the next start |
| Card lock holder equals this job | Before release |
| Local base ref unchanged | End of implement job. Reason `base_moved` |
| Export path is under `<data>/exports` and is not the worktree | Before the container starts |

`Reconcile` stays free of model calls. It may probe health under RFC-039.

## Limits

| Limit | Value |
| --- | --- |
| Read returned to the model | 256 KiB |
| Write from `edit_file` | 1 MiB |
| Log kept for a pack | Last 200 lines, after redaction, max 64 KiB |
| Child output | 1 MiB |
| Check timeout | 10 min |
| Git and `gh` timeout | 2 min |
| Health timeout | 5 s |
| Health body | 64 KiB |
| API body | 1 MiB |
| Bearer life | 12 h |
| Session failures | 5, then 1 min pause |
| Secret minimum length | 8 |
| `implementationCap` | 1 to 8 |
| `maxAttempts` | 1 to 10 |

Router budgets still apply on top of this table.

## Tests to add

These sit in the suite in [0005-verification.md](0005-verification.md).

| Test | Passes when |
| --- | --- |
| `TestDataDirInsideRepo` | `--data <repo>/.orchestrator` exits non-zero before the database opens |
| `TestToolNotInProfile` | A scripted `deploy` tool call does not exec and writes `rejected: true` |
| `TestPathEscape` | `../x`, an absolute path, `.git/config`, and a symlink out of the worktree all error |
| `TestCreateCardIgnoresCommand` | A `command` field on `create_card` leaves `local_check` equal to settings |
| `TestCheckParse` | `go test ./...` parses. `go test; id` and `$(id)` are refused. `bash -c x` is refused by the allowlist |
| `TestNoShell` | `execChild` refuses an argv whose basename is `sh` or `bash` |
| `TestSHAArg` | `dev;id` is refused before exec. A 40 hex SHA is accepted |
| `TestDeployCommandPath` | A command inside the repo, inside the data directory, or group writable is refused at save and at deploy |
| `TestSandboxFlags` | The local check argv has `--network=none`, `--cap-drop=ALL`, the digest, and an export mount, and has no socket or worktree path |
| `TestDevCheckNetwork` | The dev check argv has no `--network=none` and has `DEV_URL` |
| `TestCheckOnExport` | A check that writes `/work/.git` leaves the worktree's `.git` file unchanged |
| `TestTarEscape` | An archive entry named `../x` is refused |
| `TestSandboxOffRejected` | Sandbox off outside the fixture does not run the check |
| `TestHealthLinkLocal` | A host resolving to `169.254.169.254` fails the probe |
| `TestHealthLoopback` | A listed host resolving to `127.0.0.1` fails outside fixture mode |
| `TestHealthPrivateListed` | A listed host resolving to `10.0.0.5` is probed |
| `TestHealthUnlisted` | A host not in `devHosts` is `deploy_config` |
| `TestHealthRedirect` | A 302 is a failure and the second URL is not requested |
| `TestRedactPack` | The deploy token planted in a log is `[redacted]` in the pack and the event |
| `TestShortSecret` | A 4 character token PUT returns `422` |
| `TestEventsImmutable` | `UPDATE events` and `DELETE events` fail |
| `TestLockHolder` | A release with the wrong holder leaves the row and appends `assertion_failed` |
| `TestLoopbackOnly` | Without `ORCHESTRATOR_CLOUD`, `--addr 0.0.0.0:8787` exits non-zero |
| `TestCloudBind` | With `ORCHESTRATOR_CLOUD=1`, a non-loopback `--addr` listens, and a `Host` outside the allowlist is `403` |
| `TestHostHeader` | `Host: evil.example:8787` is `403` when that name is not allowed |
| `TestPasswordLogin` | A correct email and password returns a bearer that loads that user |
| `TestWrongPassword` | A wrong password is `401` and creates no session |
| `TestSessionLockout` | The 6th bad password for one email inside the pause window is `429`. A different email is not paused |
| `TestBearerExpiry` | A bearer older than 12 hours is `401` |
| `TestLastAdmin` | Disabling or removing the admin flag from the last admin is `422` |
| `TestUserGitHubOmitted` | `GET /api/me/github` has `tokenSet` and no token |
| `TestNonAdminForbidden` | A non-admin `POST /api/users` is `403` |
| `TestUserNameRequired` | `POST /api/users` without a name, with only spaces, over 80 characters, or with a control character is `422` on `name`. A trimmed name is stored and listed (RFC-058) |
| `TestAdminRenamesUser` | An admin renames a user with `PUT /api/users/{id}`. An empty name is `422`. A save without `name` keeps it. A non-admin is `403` |
| `TestOwnName` | `PUT /api/me` sets the caller's name and `GET /api/me` returns it. An empty name is `422`. Another user's name is unchanged |
| `TestMemberSeesProject` | A member lists the project and its cards |
| `TestOutsiderMissesProject` | A non-member receives `404` for the project and for a card |
| `TestReqIdsPerProject` | Two projects can both store `REQ-001` |
| `TestBranchPerProject` | Two projects can both store the same branch name |
| `TestAdminManagesMembership` | An admin adds a member. A non-admin cannot |
| `TestParallelProjects` | Two projects can implement at the same time |
| `TestLeadDeploy` | A Lead Developer member moves `approved` to `deploying_dev` |
| `TestOwnerApproveStops` | A Product Owner approval leaves the card in `approved` |
| `TestAcceptSeniorRoles` | Lead Developer, Senior Developer, QA Lead, and Product Owner members can accept. Developer and QA members get `403` (RFC-053) |
| `TestDeveloperRunOnly` | A Developer member can start a run and cannot approve or deploy |
| `TestCardRunRole` | A Product Owner member sees no `run` action, is `403` on a card run, and the board has `canRun: false`. Lead Developer, Senior Developer, Developer, and QA members may run (RFC-061) |
| `TestActorOnTransition` | A new transition event stores `actorId` and does not change older rows |
| `TestCompanyGitHubTokenNeverReturned` | `GET` and `PUT /api/settings/github` return `tokenSet` and no token, and the row holds ciphertext |
| `TestCompanyGitHubAdminOnly` | A non-admin `PUT /api/settings/github` is `403` |
| `TestRepoListUsesCompanyToken` | The repo list runs `gh` twice, for the organization list and `user/repos`, each with the company token in `GH_TOKEN` and not in argv |
| `TestRepoListIncludesCollaboratorRepos` | With an empty organization list, a private repository from `user/repos`, filtered to the organization, is listed |
| `TestCompanyTestReportsAccess` | A classic token without the `repo` scope that sees no repositories gets `tokenKind: classic`, its scopes, `repoCount: 0`, and the repo-scope hint. The token is in neither the response nor argv |
| `TestProjectRepoUnique` | A second project on the same repo is `422` on `repo` |
| `TestCloneTokenNotInArgv` | The clone and fetch argv never contain the token, and the URL is built from the company org and project repo |
| `TestCheckoutUsesBaseBranch` | A worktree branches from the project's base branch |
| `TestIntakePerProject` | Ingest and accept on one project leave another project's requirements unchanged |
| `TestProjectDeploySettingsIsolated` | Deploy settings saved on one project are not returned for another |
| `TestProjectDeployTokenNeverReturned` | The project deploy route returns `deployTokenSet` and no token, and the token is in that project's `secrets.json` |
| `TestDeployLeadOnly` | A Developer member who saves deploy settings is `403` |
| `TestProjectLinkSameTribeOnly` | A link to a project in another tribe is `422` |
| `TestProjectLinkNoSelf` | A link from a project to itself is `422` |
| `TestProjectLinkOutsiderMisses` | A non-member reading links is `404` |
| `TestTribeAdminManagesMembers` | A tribe admin who is a Developer lists candidates, adds and removes members, and cannot remove themselves (`422`) (RFC-057) |
| `TestTribeAdminGrantsTribeAdmin` | A tribe admin sets and clears another member's flag, `/api/me` lists the tribe in `tribeAdminOf`, clearing their own flag is `422`, and a non-member is `404` |
| `TestNonTribeAdminCannotManage` | A plain member is `403` on add, remove, flag, candidates, and project create. A tribe admin is `403` on another tribe's members and projects |
| `TestTribeAdminCreatesProjectWithLinks` | A tribe admin creates a project with a same-tribe link. The `201` lists the link and `project_links` holds it |
| `TestCreateProjectLinkOtherTribeRollsBack` | A create naming a project in another tribe is `422` on `links` and stores no project |
| `TestTribeAdminSavesLinks` | A tribe admin who is a Developer saves links. A plain Developer member is `403` |
| `TestAdminEditsProjectSettings` | A company admin with the Product Owner role, not in the tribe, reads and saves repository, links, deploy, and conventions with `canEdit: true` and passes the Sync role check. The same admin is `404` on the project's cards, and an unknown project is `404` (RFC-059) |
| `TestProjectRepoCheckedOnGitHub` | Creating a project on a missing, archived, or moved repository is `422` on `repo`, a missing branch is `422` on `baseBranch`, and no project is stored. A repository save with a missing repository is `422`, and one GitHub finds is saved (RFC-060) |
| `TestProjectRepoNeedsCompanyToken` | With no company token, creating a project is `422` and names the company GitHub token |
| `TestProjectRepoGitHubDown` | A GitHub call that fails with anything but 404 is `502` |
| `TestTribeAdminEditsProjectSettings` | A tribe admin who is a Developer saves all four sections of a project in their tribe and is `404` on another tribe's project. A plain Developer member reads each with `canEdit: false` and is `403` on each save |
| `TestPackIncludesLinkedRequirements` | The implementer pack holds the linked project's accepted requirements as `<project>:REQ-001` |
| `TestSharedPathConfined` | A shared path with `..`, an absolute path, or `.git` is rejected |
| `TestReadLinkedFileReadOnly` | `read_linked_file` reads the linked export, rejects an unlinked project, and there is no linked write tool |
| `TestLinkedContextIsUntrusted` | Linked content sits inside the untrusted markers and a planted secret is `[redacted]` |
| `TestHookNotRun` | A fixture `pre-commit` hook is not executed |
| `TestFsmonitorOff` | A repo config with `core.fsmonitor` set to a command does not run it |
| `TestPushURL` | A remote named `origin` pointing elsewhere is ignored. The push goes to the settings URL with the card refspec only |
| `TestAskpass` | The token is absent from every recorded git argv and present only in the askpass env |
| `TestPushUsesOwnerToken` | Push, pull request, and reviewer calls carry the owner's token in the child environment, never in argv, and never the company token (RFC-054) |
| `TestOwnerGitHubBlocks` | A card whose owner has no verified token moves to `blocked` with `owner_github`, and no push or `gh` call runs |
| `TestGitHubLoginVerified` | The account token test stores the login `gh api user` reports. A new token clears it. A second user cannot verify the same login |
| `TestBaseRefUnchanged` | Local `refs/heads/main` is unchanged after a job, and a forced change blocks with `base_moved` |
| `TestPromptMarkers` | The user pack contains both markers once, untrusted copies of the markers are stripped, and the system hash matches |
| `TestPackConventions` | Rule and skill blocks sit between the markers before the requirements, a block over 16 KiB ends with `truncated: true`, blocks past 64 KiB become `omitted` notes, and a planted secret is `[redacted]` (RFC-055) |
| `TestConventionsCannotAddTool` | A rule whose body asks for a `deploy` tool leaves the implementer tool list unchanged, and a scripted `deploy` call is rejected |
| `TestConventionsContextEvent` | An attempt appends one `conventions_context` event with each rule and skill id, version, and hash |
| `TestReviewerPackConventions` | The reviewer pack carries the card's rules and skills inside the markers |

## Operator duties

These stay outside the program.

| Duty | Why it is still theirs |
| --- | --- |
| Choose the sandbox image digest | The harness will not pull a floating tag |
| Install deploy and rollback scripts outside the repo | RFC-038 refuses any other location |
| List dev hosts | The probe refuses an empty list |
| Keep the data directory off shared or synced disks | It holds tokens and the board |
| Leave `ORCHESTRATOR_CLOUD` unset on a laptop | Loopback stays the default bind |
| Set cloud mode only behind TLS | RFC-045 refuses a public bind without that flag |
| Keep `tokenKey` in the service secrets file | It decrypts every GitHub token |
| Ask developers for fine-grained tokens limited to the company organization, with Contents and Pull requests write | Unattended jobs push with each owner's token (RFC-054) |
| Review what dev checks reach | Dev check egress is open in v1 |

A production credential is still out of scope (NG-001). Hardening v1 does not add a safe place to put one.
