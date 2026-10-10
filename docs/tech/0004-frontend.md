# Frontend implementation

Status: Accepted for implementation. Date: 2026-10-10. Revised for SvelteKit and after review.

This client is a separate SvelteKit app. It talks to the API in [0002-data-and-api.md](0002-data-and-api.md). It does not use React. It does not decide card status (RFC-035). Security rules for the client are in [0006-security-and-harness.md](0006-security-and-harness.md). Node runs Vite for development and for the static build. Node is not the harness, and this app has no server routes.

Build it after the JSON API exists. Until then, the CLI is enough to run the fixture.

## Shape

SvelteKit 2 with Svelte 5. Rendering is client-only:

```ts
// src/routes/+layout.ts
export const ssr = false;
export const prerender = false;
```

`@sveltejs/adapter-static` writes a static site with `fallback: 'index.html'`. There is no `+server.ts`, no `hooks.server.ts`, and no SvelteKit form actions. Every read and write is `fetch` from the browser to the Go API. Local dev uses `http://127.0.0.1:8787`. A cloud build uses the configured API origin.

The dev and preview servers bind to `127.0.0.1:4173`, which is the default origin the Go API allows. Cloud mode uses the configured UI origin (RFC-045).

```ts
// vite.config.ts
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [sveltekit()],
  server: { host: '127.0.0.1', port: 4173, strictPort: true },
  preview: { host: '127.0.0.1', port: 4173, strictPort: true }
});
```

```js
// svelte.config.js
import adapter from '@sveltejs/adapter-static';

export default {
  kit: {
    adapter: adapter({ fallback: 'index.html' }),
    csp: {
      mode: 'hash',
      directives: {
        'default-src': ['self'],
        'script-src': ['self'],
        'style-src': ['self'],
        'img-src': ['self'],
        'connect-src': ['self', 'http://127.0.0.1:8787'],
        'object-src': ['none'],
        'base-uri': ['self'],
        'form-action': ['self']
      }
    }
  }
};
```

SvelteKit writes the policy as a `<meta http-equiv>` tag in the built `index.html`, with hashes for its own inline bootstrap script. Do not add `'unsafe-inline'` or `'unsafe-eval'`. If a component needs an inline style, use a class in `app.css`. A cloud build replaces `http://127.0.0.1:8787` in `connect-src` with the configured API origin. It still does not add a wildcard.

`package.json` pins exact versions and commits `package-lock.json`:

| Script | Command |
| --- | --- |
| `dev` | `vite dev` |
| `build` | `vite build` |
| `preview` | `vite preview` |
| `check` | `svelte-kit sync && svelte-check --tsconfig ./tsconfig.json` |
| `test` | `vitest run` |

Dev dependencies: `@sveltejs/kit`, `@sveltejs/adapter-static`, `@sveltejs/vite-plugin-svelte`, `svelte`, `vite`, `typescript`, `svelte-check`, `vitest`, `@testing-library/svelte`, `jsdom`, `tailwindcss`, `@tailwindcss/vite`. No runtime dependencies beyond these.

## Layout

```text
frontend/
  package.json
  svelte.config.js
  vite.config.ts
  src/app.html
  src/app.css
  src/routes/+layout.ts
  src/routes/+layout.svelte
  src/routes/login/+page.svelte
  src/routes/+page.svelte                      board
  src/routes/intake/+page.svelte
  src/routes/cards/[id]/+page.svelte
  src/routes/cards/[id]/trace/+page.svelte
  src/routes/account/+page.svelte
  src/routes/admin/users/+page.svelte
  src/routes/admin/users/new/+page.svelte
  src/routes/admin/users/[id]/+page.svelte
  src/routes/admin/github/+page.svelte
  src/routes/tribes/+page.svelte
  src/routes/tribes/new/+page.svelte
  src/routes/tribes/[id]/+page.svelte
  src/routes/projects/+page.svelte
  src/routes/projects/new/+page.svelte
  src/routes/settings/+layout.svelte
  src/routes/settings/repository/+page.svelte
  src/routes/settings/links/+page.svelte
  src/routes/settings/llm/+page.svelte
  src/routes/settings/llm/new/+page.svelte
  src/routes/settings/llm/[family]/+page.svelte
  src/routes/settings/router/+page.svelte
  src/routes/settings/deploy/+page.svelte
  src/lib/api.ts
  src/lib/session.ts
  src/lib/types.ts
  src/lib/roles.ts
  src/lib/llm.ts
  src/lib/shell.svelte.ts
  src/lib/ui/Button.svelte
  src/lib/ui/Field.svelte
  src/lib/ui/Select.svelte
  src/lib/ui/Menu.svelte
  src/lib/ui/SearchSelect.svelte
  src/lib/ui/Panel.svelte
  src/lib/ui/Badge.svelte
  src/lib/ui/Page.svelte
  src/lib/ui/SaveBar.svelte
  src/lib/ui/FieldGroup.svelte
  src/lib/ui/Row.svelte
  src/lib/ui/Table.svelte
  src/lib/ui/Checkbox.svelte
  src/lib/ui/Notice.svelte
  src/lib/ui/EmptyState.svelte
  src/lib/ui/control.ts
  src/lib/components/Live.svelte
  src/lib/components/SecretField.svelte
  src/lib/components/CardActions.svelte
  src/lib/components/FamilyForm.svelte
  src/lib/api.test.ts
  src/lib/components/CardActions.test.ts
  src/lib/components/SecretField.test.ts
  src/lib/ui/SearchSelect.test.ts
```

`src/lib/types.ts` mirrors the JSON in 0002. If a field is added to the API, add it here in the same change.

`src/app.html` keeps the default SvelteKit shell: a skip link is added in `+layout.svelte`, and `%sveltekit.body%` is the only body slot.

## Session and routes

`src/lib/session.ts` reads and writes `sessionStorage` keys `orchestrator.bearer` and `orchestrator.project`. It stores the bearer and the selected project id. It does not store GitHub tokens, passwords, model keys, `tokenKey`, or the deploy token.

`src/routes/+layout.ts` loads in the browser. When the path is not `/login` and no bearer is stored, it `redirect`s to `/login`.

| Path | Page |
| --- | --- |
| `/login` | Email and password |
| `/` | Board for the selected project |
| `/intake` | Ingest and accept for the selected project |
| `/cards/[id]` | Card |
| `/cards/[id]/trace` | Trace |
| `/account` | Password and the caller's GitHub settings |
| `/admin/users` | Admin user list |
| `/admin/users/new` | Create a user |
| `/admin/users/[id]` | Edit role, admin, disabled, and an optional password |
| `/admin/github` | Company GitHub account |
| `/tribes` | Tribes |
| `/tribes/new` | Create a tribe |
| `/tribes/[id]` | Tribe members |
| `/projects` | Projects for the caller's tribes |
| `/projects/new` | Create a project |
| `/settings/repository` | Project repository, base branch, and Sync |
| `/settings/links` | Linked projects and their shared paths |
| `/settings/llm` | LLM families for the project |
| `/settings/llm/new` | Create an LLM family |
| `/settings/llm/[family]` | Edit, test, or delete one LLM family |
| `/settings/router` | Router settings for the project |
| `/settings/deploy` | Deploy and checks for the project |

`+layout.svelte` renders the Marauder shell on every page except `/login`. The product name in the shell is Marauder. The sidebar groups are Workspace (Board, Intake), Project (Projects, Repository, Linked projects, LLM APIs, Router, Deploy), and Admin (Users, Tribes, Company GitHub). Admin links render when `GET /api/me` has `admin: true`. Those links are not a card permission check. A `403` or `404` from the API is still shown.

The sidebar shows a project `SearchSelect` bound to `orchestrator.project`. Options come from `GET /api/projects`, each with its repository name as the detail line, and the search matches name or repository. Choosing a different project stores the id and reloads the board. The signed-in user menu links to Account and signs out with `DELETE /api/session`. Below 1024px the same links open from a menu button. The top bar shows the current project and the live status line. The page title and page actions sit in the body, in the `Page` header.

Create and edit forms are their own routes. They are not dialogs.

Unknown paths use SvelteKit's default error page with a link to `/`.

## API wrapper

`src/lib/api.ts` exports one function per route in 0002. Each function:

1. Sends `Authorization: Bearer` from `session.bearer()`.
2. Sends `Content-Type: application/json` on PUT and POST bodies.
3. Parses `error` on non-2xx and throws `ApiError` with `status`, `code`, `message`, and `fields`.
4. On `401`, clears the stored bearer and sends the browser to `/login`.
5. Does not log the request body. Secret fields must not reach `console`.

Base URL is `http://127.0.0.1:8787` unless the build is given the cloud API origin. A failed `fetch` throws a network error the page shows as "API unreachable at http://127.0.0.1:8787", using the configured origin in cloud mode. The page does not invent cards or settings in that case. Project routes include the selected project id. A `404` on those routes sets the live message to "That project is not available."

## Markup rules

Svelte text `{value}` is the way API strings hit the page. `{@html}` is forbidden. A pull request URL becomes an `<a href>` only when it starts with `https://github.com/` or `http://127.0.0.1`. Any other value is text.

Buttons that submit a form use `type="submit"`. Buttons that do not submit use `type="button"`. Every input has a `<label for>`.

`Live.svelte` is one paragraph:

```svelte
<p role="status">{message}</p>
```

Pages set `message` to "Loading board", "Running the worker", "Saving GitHub settings", or the error string. The paragraph stays in the page so the text change is announced.

## Login

`src/routes/login/+page.svelte`. Email and password fields, and a submit button "Sign in". Submit calls `POST /api/session` with `{ email, password }`. Success stores the bearer and `goto('/')`. Failure leaves the typed password out of the live region and sets the live message to `error.message`. A `429` says to wait one minute. The password input is `type="password"` and `autocomplete="current-password"`.

## Board

`src/routes/+page.svelte` calls `GET /api/cards` from `onMount` (or `$effect` on first run).

Render six sections in this order: Waiting, Running, Review, Shipping, Finished, Stopped. Place each card by `group`. Print `status` as text on the row. The row links to `/cards/{id}`.

The primary button is "Run". It is enabled when `canRun` is true and `running` is false. It POSTs `/api/runs`, then reloads the list. The live message is "Running the worker" while the request is in flight, and the button is disabled.

When `running` is true, or any card is in the `running` or `shipping` group or in `agent_review`, an `$effect` starts a 3 second `setInterval` that calls `GET /api/cards` again. The effect cleanup clears the timer when the condition becomes false and when the page is destroyed.

Empty `cards`: one paragraph, "No cards yet", and a link "Ingest requirements" to `/intake`. No table of placeholder rows.

## Card

`src/routes/cards/[id]/+page.svelte` loads `GET /api/cards/{id}` using the `params.id` from the page.

Show title, raw status, requirement ids, decision ids, risk, attempts used and max attempts, local check, dev check, branch, pull request link, head SHA, reviewed SHA, whether review was independent, approved SHA, deployed SHA, failure reason, and blocked reason. Omit a row when the value is null. Do not replace null with a fake SHA.

Render a control only when its name is in `actions`:

| Name | Control |
| --- | --- |
| `approve` | Form. SHA input defaulted from `headSha`. Button "Approve SHA". |
| `request_changes` | Button "Request changes" |
| `reject` | Button "Reject" |
| `deploy` | Button "Deploy approved SHA". |
| `unblock` | Form. Checkbox "Reset attempt counter", unchecked by default. Button "Unblock". Text above the button from `unblockTo` |

`unblockTo` text:

| `unblockTo` | Text |
| --- | --- |
| `ready` | Unblock returns this card to Ready. |
| `human_review` | Unblock returns this card to human review. Approve the head again to redeploy. |
| `null` on a blocked card | Accept the changed requirements in Intake before this card can be released. |

These controls live in `CardActions.svelte`, which takes the card object as its only prop.

`{#if actions.includes('approve')}` is the pattern. Do not also check `status` in the template. The API list is the source.

After a successful POST, replace the page state with the returned card. On `422`, keep the form values and show `fields.sha` under the SHA input, or `error.message` in the live region. On `409`, reload the card and set the live message to "Card changed. Reloaded."

A link "Trace" goes to `/cards/{id}/trace`.

`in_progress` has an empty `actions` array. The page shows the fields and no approve form. An `approved` card shows Deploy only when `actions` includes `deploy`, plus Request changes and Reject when those names are present, and no Approve. Those are the checks the verification guide looks for. The page does not show Deploy because the status is `approved`.

## Intake

`GET /api/intake`.

Each requirement is a block: id, accepted or not, a "Drafted by the planner" note when `sourceKind` is `draft`, statement, acceptance lines, source path, and links to `linkedCards`. Decisions are a list under them: id and statement.

Button "Ingest" POSTs `/api/ingest` and replaces the list. Button "Accept unaccepted ids" POSTs `/api/accept` with the ids on screen whose `accepted` is false. It is disabled while `draftPending` is true.

While `draftPending` is true, the live message is "Planner is drafting requirement ids" and the page polls `GET /api/intake` every 3 seconds. When `planError` is set, show it under the heading as text. The live message distinguishes "Reading docs/prd and docs/rfc" from "Saving acceptance".

## Trace

`GET /api/cards/{id}/trace`.

A list, oldest first. Each item shows time, kind, attempt id, and the payload inside `<pre>{JSON.stringify(payload, null, 2)}</pre>`. No model call from this page.

## Settings pages

`SecretField.svelte` is used for the caller's GitHub token, the company GitHub token, each LLM key, and the deploy token.

1. Input `type="password"`, autocomplete `off`.
2. If the GET flag (`tokenSet`, `keySet`, or `deployTokenSet`) is true, placeholder "A value is saved".
3. Empty input on submit means the JSON omits that secret field.
4. A "Remove" button sets `clear: true` and omits the secret string.
5. The value is never written to the live message or the console.

Disable the submit button and set the live message while the PUT is in flight. On `422`, paint `fields` under the matching inputs and keep what the operator typed.

### Account

`src/routes/account/+page.svelte`. `GET /api/me` and `GET /api/me/github`.

Password fieldset: current password, new password, button "Change password", `PUT /api/me/password`.

GitHub fieldset: host, login, token. The help text says this is the caller's own account, and that clones, pushes, and pull requests use the company account. "Test connection" calls `POST /api/me/github/test`. Disable it when `tokenSet` is false and the token input is empty. Show `login` on success. Show `message` on `ok: false`.

### Users and tribes

`src/routes/admin/users/+page.svelte` lists users. Create is `/admin/users/new` (email, password, role, admin) and calls `POST /api/users`. Edit is `/admin/users/[id]` and calls `PUT /api/users/{id}`. A `403` sets the live message from `error.message`.

`src/routes/tribes/+page.svelte` lists tribes. Create is `/tribes/new` with a name. `/tribes/[id]` lists members from `GET /api/tribes/{id}/members` and adds one by picking a user who is not yet a member in a `SearchSelect` searched by email. The request still sends `userId`. Remove asks on the row, then calls `DELETE /api/tribes/{id}/members/{userId}`. `src/routes/projects/+page.svelte` lists projects from `GET /api/projects` with their repository. Create is `/projects/new` with tribe, name, a repository `SearchSelect` filled from `GET /api/settings/github/repos`, and base branch. Picking a repository fills the base branch with its default branch. When the list cannot load, the repository is a text field.

### Company GitHub

`src/routes/admin/github/+page.svelte`. GET and PUT `/api/settings/github`. Admin only. Fields: host, organization, token. "Test connection" calls `POST /api/settings/github/test`, and shows the bot login on success or `message` on `ok: false`. The saved bot login shows as read-only text.

### Repository

`src/routes/settings/repository/+page.svelte`. GET and PUT `/api/projects/{id}/settings/repository`. Shows the company host and organization as text, the repository and base branch as fields, and whether the clone exists with its head SHA. "Sync" calls `POST /api/projects/{id}/sync`.

### Linked projects

`src/routes/settings/links/+page.svelte`. GET and PUT `/api/projects/{id}/settings/links`. Each link is a panel with the project name, its repository, a list of shared paths with "Add path" and a remove button per row, and "Remove link". "Add link" is a `SearchSelect` of projects in the same tribe that are not this project and not linked yet. The page explains that linked context is read only.

### LLM APIs

`src/routes/settings/llm/+page.svelte` lists the families from GET `/api/projects/{id}/settings/llm` in a table: id, protocol, base URL, the first model ids, and whether a key is saved. Each row links to its edit page. "Add family" in the page header opens `/settings/llm/new`.

`src/lib/components/FamilyForm.svelte` is the form both pages use: id, protocol select with `anthropic` and `openai` only, base URL, model ids (a list plus "Add model" and a remove button per row), API key.

`src/routes/settings/llm/new/+page.svelte` creates a family. The id must be set, must not be `new`, and must not match an existing family. `src/routes/settings/llm/[family]/+page.svelte` edits one family. Its id is fixed after create, because the saved key is stored under the id. That page has Save, "Test connection", and "Delete family" behind an inline confirm.

The API has no per-family write. Create, Save, and Delete each GET the current list, change one entry, and PUT the whole families list. The other families are sent without `apiKey` or `clear`, so their saved keys stay.

"Test connection" calls `POST /api/projects/{id}/settings/llm/{family}/test` and shows the result in the panel footer.

### Router

`src/routes/settings/router/+page.svelte`. GET and PUT `/api/projects/{id}/settings/router`. Also GET that project's LLM settings so the selects can list saved model ids.

A list of five rows: `plan`, `implement`, `implement_hard`, `review`, `triage`. Each row has a model `SearchSelect` and a fallback `SearchSelect`. Options are every model id from LLM APIs, each with its family id as the detail line, and the search matches either. Both pickers start with `None`, which saves an empty value. The review model picker also lists `different-family-from-author` first. A saved model that no family lists any more still shows, marked "Not in LLM APIs". From 640px the rows are three columns: class, model, fallback. Below that each row stacks and the picker labels show.

Four number inputs for the budgets. When the GET budgets are missing, show 3, 12, 200000, and 30.

Above the table, print the server sentence:

| `reviewIndependentPossible` | Text |
| --- | --- |
| `true` | Review can use a second family. |
| `false` | Only one family is configured, so review will record review.independent = false. |

The page does not calculate that flag itself.

The save result line includes "Applies to the next route decision."

### Deploy and checks

`src/routes/settings/deploy/+page.svelte`. GET and PUT `/api/projects/{id}/settings/deploy`. These settings belong to the selected project (RFC-050). Any member can read them. Save is for a Lead Developer member, and a `403` shows in the live message.

Fields, in three fieldsets:

| Fieldset | Fields |
| --- | --- |
| Dev server | Dev URL, dev hosts (one per line), deploy command, rollback command, health URL, deploy token |
| Checks | Local check, dev check, allowed executables (one per line), sandbox engine select (`podman`, `docker`), sandbox image |
| Board | Implementation cap (number, default 2), PRD path, RFC path |

Static help text:

| Under | Text |
| --- | --- |
| Deploy and rollback command | Absolute path to a script outside the target repo and the data directory. Not group or world writable. |
| Dev hosts | Health URL and dev URL hosts must be listed here. |
| Sandbox image | Pin by digest, for example `name@sha256:...`. |
| Fieldset legend for dev server | Empty values can be saved. Deploy stops with deploy_config until they are set. |

## Styles

The product name is Marauder. `src/app.css` imports Tailwind CSS 4 and defines the theme: ground `#f4f1ea`, surface `#fffcf8`, sidebar `#efeae1`, ink `#1c1915`, line `#e6dfd4`, accent `#1f4d3a`. `tailwindcss` and `@tailwindcss/vite` are dev dependencies. There is no runtime CSS library and no React component kit.

Shared controls live in `src/lib/ui/`. One primary button on a page. `.danger` is for blocked status. `.error` is for a field error. Inputs and buttons are at least 44px tall. `:focus-visible` keeps a 2px ink outline. Do not remove outlines. Do not put layout in a `style` attribute.

The theme also defines the layout scale. Page padding is 24px, and 32px from 1024px. Page measures are `max-w-form` (42rem) for forms and settings, `max-w-wide` (72rem) for tables, and full width for the board. Field widths are `sm` 10rem, `md` 20rem, `lg` 32rem, and `full`. `Field`, `Select`, and `SecretField` take a `width` prop that defaults to `md`. Inside a two-column grid a field uses `full`.

Every page body starts with `Page`: the `h1`, an optional description, an optional `meta` snippet beside the title, and an `actions` snippet aligned right on the same row. A hairline separates the header from the content. The top bar is a 48px strip with the menu button below 1024px, the current project, and the live status line. It holds no page actions.

Buttons size to their label and never stretch. `full` is only for the login submit. `Panel` has a title row with an optional `actions` snippet and an optional `footer` snippet. The footer is a right-aligned action bar. Settings pages that hold more than one panel end in `SaveBar`, a bar that sticks to the bottom of the viewport with the primary Save. `FieldGroup` groups related controls under one legend. `Row` is one entry in a bordered list, such as one model id, with a remove button. Tables use `Table` and the `th`, `tr`, and `td` classes from `src/lib/ui/control.ts`: an 11px uppercase header and 48px rows.

`SearchSelect` is the shared searchable select for long lists: the project switcher, Router models, and tribe member search. It is an ARIA combobox. Typing filters options by label and detail, arrow keys move, Enter picks, and Escape closes and restores the selected label. An optional `emptyLabel` adds a first option that clears the value. `onchange` fires only when a pick changes the value. Short fixed lists, such as protocol or role, keep the native `Select`.

System font stack: `system-ui, sans-serif`. Below 720px board groups stack in one column. From 720px they sit in one row of lanes at least 15rem wide that scrolls sideways when the window is narrow.

## Page states

| Situation | UI |
| --- | --- |
| No bearer | Redirect to `/login` |
| Board empty | "No cards yet" and the intake link |
| Loading | Live region names the page |
| In flight | Submit disabled, live region names the action |
| `422` | Field errors, typed secrets kept |
| `409` | Reload, "Card changed. Reloaded." |
| `401` | Bearer cleared, redirect to `/login` |
| `429` on login | "Too many attempts. Wait one minute." |
| Network failure | "API unreachable" and the base URL |
| Unknown card | "Card not found" and a link back to the board |

## Build

From `frontend/`, once:

```text
npm ci
```

`npm install` is used only when changing a dependency, and the updated lockfile is committed in the same change.

Then, with the API already listening on `127.0.0.1:8787`:

```text
npm run dev
```

Open `http://127.0.0.1:4173/`. Sign in with email and password. There is no UI token to paste.

`npm ci` is a frontend step. It is not a step for `backend/`. A target repo may still use Node for its own `localCheck`. That command runs in the sandbox and is not this app.

A static build, still served as files and still calling the Go API:

```text
npm run build
npm run preview
```

`preview` stays on `127.0.0.1:4173`. Do not add an adapter that runs a Node server for card actions.

## Checks

```text
npm run check
npm run test
npm run build
```

| Test | Passes when |
| --- | --- |
| `CardActions` with `actions: []` | No Approve, Request changes, Reject, or Unblock control is rendered |
| `CardActions` on an approved card | Request changes and Reject render. Approve does not |
| `CardActions` on a blocked card with `unblockTo: null` | No Unblock button. The Intake sentence renders |
| `SecretField` saved with an empty input | The emitted body omits the secret field |
| `api` on `401` | The bearer is cleared |
| Built `build/index.html` | Contains a CSP meta tag with `script-src 'self'` and a hash, and no `unsafe-inline` |
