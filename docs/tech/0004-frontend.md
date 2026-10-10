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

`@sveltejs/adapter-static` writes a static site with `fallback: 'index.html'`. There is no `+server.ts`, no `hooks.server.ts`, and no SvelteKit form actions. Every read and write is `fetch` from the browser to `http://127.0.0.1:8787`.

The dev and preview servers bind to `127.0.0.1:4173`, which is the origin the Go API allows.

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

SvelteKit writes the policy as a `<meta http-equiv>` tag in the built `index.html`, with hashes for its own inline bootstrap script. Do not add `'unsafe-inline'` or `'unsafe-eval'`. If a component needs an inline style, use a class in `app.css`.

`package.json` pins exact versions and commits `package-lock.json`:

| Script | Command |
| --- | --- |
| `dev` | `vite dev` |
| `build` | `vite build` |
| `preview` | `vite preview` |
| `check` | `svelte-kit sync && svelte-check --tsconfig ./tsconfig.json` |
| `test` | `vitest run` |

Dev dependencies: `@sveltejs/kit`, `@sveltejs/adapter-static`, `@sveltejs/vite-plugin-svelte`, `svelte`, `vite`, `typescript`, `svelte-check`, `vitest`, `@testing-library/svelte`, `jsdom`. No runtime dependencies beyond these.

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
  src/routes/settings/github/+page.svelte
  src/routes/settings/llm/+page.svelte
  src/routes/settings/router/+page.svelte
  src/routes/settings/deploy/+page.svelte
  src/lib/api.ts
  src/lib/session.ts
  src/lib/types.ts
  src/lib/components/Live.svelte
  src/lib/components/SecretField.svelte
  src/lib/components/CardActions.svelte
  src/lib/api.test.ts
  src/lib/components/CardActions.test.ts
  src/lib/components/SecretField.test.ts
```

`src/lib/types.ts` mirrors the JSON in 0002. If a field is added to the API, add it here in the same change.

`src/app.html` keeps the default SvelteKit shell: a skip link is added in `+layout.svelte`, and `%sveltekit.body%` is the only body slot.

## Session and routes

`src/lib/session.ts` reads and writes `sessionStorage` key `orchestrator.bearer`. It stores the bearer only. It does not store GitHub tokens, model keys, or the deploy token.

`src/routes/+layout.ts` loads in the browser. When the path is not `/login` and no bearer is stored, it `redirect`s to `/login`.

| Path | Page |
| --- | --- |
| `/login` | UI token form |
| `/` | Board |
| `/intake` | Ingest and accept |
| `/cards/[id]` | Card |
| `/cards/[id]/trace` | Trace |
| `/settings/github` | GitHub settings |
| `/settings/llm` | LLM API settings |
| `/settings/router` | Router settings |
| `/settings/deploy` | Deploy and checks |

`+layout.svelte` renders the nav on every page except `/login`.

| Label | href |
| --- | --- |
| Board | `/` |
| Intake | `/intake` |
| GitHub | `/settings/github` |
| LLM APIs | `/settings/llm` |
| Router | `/settings/router` |
| Deploy | `/settings/deploy` |

Unknown paths use SvelteKit's default error page with a link to `/`.

## API wrapper

`src/lib/api.ts` exports one function per route in 0002. Each function:

1. Sends `Authorization: Bearer` from `session.bearer()`.
2. Sends `Content-Type: application/json` on PUT and POST bodies.
3. Parses `error` on non-2xx and throws `ApiError` with `status`, `code`, `message`, and `fields`.
4. On `401`, clears the stored bearer and sends the browser to `/login`.
5. Does not log the request body. Secret fields must not reach `console`.

Base URL is `http://127.0.0.1:8787`. A failed `fetch` throws a network error the page shows as "API unreachable at http://127.0.0.1:8787". The page does not invent cards or settings in that case.

## Markup rules

Svelte text `{value}` is the way API strings hit the page. `{@html}` is forbidden. A pull request URL becomes an `<a href>` only when it starts with `https://github.com/` or `http://127.0.0.1`. Any other value is text.

Buttons that submit a form use `type="submit"`. Buttons that do not submit use `type="button"`. Every input has a `<label for>`.

`Live.svelte` is one paragraph:

```svelte
<p role="status">{message}</p>
```

Pages set `message` to "Loading board", "Running the worker", "Saving GitHub settings", or the error string. The paragraph stays in the page so the text change is announced.

## Login

`src/routes/login/+page.svelte`. One password field labeled "UI token" and a submit button "Open session". Submit calls `POST /api/session`. Success stores the bearer and `goto('/')`. Failure leaves the typed token in the field and sets the live message to `error.message`.

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

`in_progress` has an empty `actions` array. The page shows the fields and no approve form. An `approved` card shows Request changes and Reject, and no Approve. Those are the checks the verification guide looks for.

## Intake

`GET /api/intake`.

Each requirement is a block: id, accepted or not, a "Drafted by the planner" note when `sourceKind` is `draft`, statement, acceptance lines, source path, and links to `linkedCards`. Decisions are a list under them: id and statement.

Button "Ingest" POSTs `/api/ingest` and replaces the list. Button "Accept unaccepted ids" POSTs `/api/accept` with the ids on screen whose `accepted` is false. It is disabled while `draftPending` is true.

While `draftPending` is true, the live message is "Planner is drafting requirement ids" and the page polls `GET /api/intake` every 3 seconds. When `planError` is set, show it under the heading as text. The live message distinguishes "Reading docs/prd and docs/rfc" from "Saving acceptance".

## Trace

`GET /api/cards/{id}/trace`.

A list, oldest first. Each item shows time, kind, attempt id, and the payload inside `<pre>{JSON.stringify(payload, null, 2)}</pre>`. No model call from this page.

## Settings pages

`SecretField.svelte` is used for the GitHub token, each LLM key, and the deploy token.

1. Input `type="password"`, autocomplete `off`.
2. If the GET flag (`tokenSet`, `keySet`, or `deployTokenSet`) is true, placeholder "A value is saved".
3. Empty input on submit means the JSON omits that secret field.
4. A "Remove" button sets `clear: true` and omits the secret string.
5. The value is never written to the live message or the console.

Disable the submit button and set the live message while the PUT is in flight. On `422`, paint `fields` under the matching inputs and keep what the operator typed.

### GitHub

`src/routes/settings/github/+page.svelte`. GET and PUT `/api/settings/github`.

Fields: host, owner, repository, base branch, approvers (textarea, one login per line), bot login, token.

"Test connection" calls `POST /api/settings/github/test`. Disable it when `tokenSet` is false and the token input is empty. Show `login` on success. Show `message` on `ok: false`.

### LLM APIs

`src/routes/settings/llm/+page.svelte`. GET and PUT `/api/settings/llm`.

One fieldset per family: id, protocol select with `anthropic` and `openai` only, base URL, model ids (a list plus "Add model" and a remove button per row), API key.

"Add family" appends a fieldset with an empty id. The operator fills the id before save.

"Test" on a family calls `POST /api/settings/llm/{id}/test` and uses the same result line as GitHub.

### Router

`src/routes/settings/router/+page.svelte`. GET and PUT `/api/settings/router`. Also GET `/api/settings/llm` so the selects can list saved model ids.

A table of five rows: `plan`, `implement`, `implement_hard`, `review`, `triage`. Each row has a model `<select>` and a fallback `<select>`. The review model select includes `different-family-from-author` plus the saved model ids.

Four number inputs for the budgets. When the GET budgets are missing, show 3, 12, 200000, and 30.

Above the table, print the server sentence:

| `reviewIndependentPossible` | Text |
| --- | --- |
| `true` | Review can use a second family. |
| `false` | Only one family is configured, so review will record review.independent = false. |

The page does not calculate that flag itself.

The save result line includes "Applies to the next route decision."

### Deploy and checks

`src/routes/settings/deploy/+page.svelte`. GET and PUT `/api/settings/deploy`.

Fields, in three fieldsets:

| Fieldset | Fields |
| --- | --- |
| Dev server | Dev URL, dev hosts (one per line), deploy command, rollback command, health URL, deploy token |
| Checks | Local check, dev check, allowed executables (one per line), sandbox engine select (`podman`, `docker`), sandbox image |
| Board | Implementation cap (number, default 2), PRD path, RFC path, operator login |

Static help text:

| Under | Text |
| --- | --- |
| Deploy and rollback command | Absolute path to a script outside the target repo and the data directory. Not group or world writable. |
| Dev hosts | Health URL and dev URL hosts must be listed here. |
| Sandbox image | Pin by digest, for example `name@sha256:...`. |
| Fieldset legend for dev server | Empty values can be saved. Deploy stops with deploy_config until they are set. |

## Styles

`src/app.css` is plain CSS, imported from `+layout.svelte`. No CSS framework.

| Class | Role |
| --- | --- |
| `.wrap` | Page padding, max width 72rem |
| `.nav` | Row of links, wraps |
| `.groups` | Board sections, row on wide screens, one column under 720px |
| `.row` | One card, status text visible |
| `.form` | Single column, label above control |
| `.error` | Field error under an input |
| `.danger` | Blocked status text |
| `.primary` | The one accent button on the page |

System font stack: `system-ui, sans-serif`. Inputs and buttons have `min-height: 44px`. `:focus-visible` keeps an outline. Do not remove outlines.

Use the accent on `.primary` only. Status color is limited to `.danger` for `blocked`.

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

Open `http://127.0.0.1:4173/`. Paste the UI token from the server log.

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
