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
  src/routes/choose-tribe/+page.svelte
  src/routes/+page.svelte                      board
  src/routes/intake/+page.svelte
  src/routes/intake/new/+page.svelte
  src/routes/intake/[id]/+page.svelte
  src/routes/cards/[id]/+page.svelte
  src/routes/cards/[id]/trace/+page.svelte
  src/routes/account/+page.svelte
  src/routes/admin/usage/+page.svelte
  src/routes/tribes/[id]/usage/+page.svelte
  src/routes/account/usage/+page.svelte
  src/routes/admin/users/+page.svelte
  src/routes/admin/users/new/+page.svelte
  src/routes/admin/users/[id]/+page.svelte
  src/routes/admin/github/+page.svelte
  src/routes/admin/llm/+page.svelte
  src/routes/admin/llm/new/+page.svelte
  src/routes/admin/llm/[family]/+page.svelte
  src/routes/admin/router/+page.svelte
  src/routes/admin/conventions/+page.svelte
  src/routes/admin/conventions/new/+page.svelte
  src/routes/admin/conventions/[id]/+page.svelte
  src/routes/admin/skills/+page.svelte
  src/routes/admin/skills/new/+page.svelte
  src/routes/admin/skills/[id]/+page.svelte
  src/routes/tribes/+page.svelte
  src/routes/tribes/new/+page.svelte
  src/routes/tribes/[id]/+page.svelte
  src/routes/projects/+page.svelte
  src/routes/projects/new/+page.svelte
  src/routes/settings/+layout.svelte
  src/routes/settings/repository/+page.svelte
  src/routes/settings/links/+page.svelte
  src/routes/settings/deploy/+page.svelte
  src/routes/settings/conventions/+page.svelte
  src/lib/api.ts
  src/lib/session.ts
  src/lib/types.ts
  src/lib/roles.ts
  src/lib/llm.ts
  src/lib/conventions.ts
  src/lib/shell.svelte.ts
  src/lib/ui/Button.svelte
  src/lib/ui/Field.svelte
  src/lib/ui/autogrow.ts
  src/lib/ui/PasswordInput.svelte
  src/lib/ui/Select.svelte
  src/lib/ui/Menu.svelte
  src/lib/ui/SearchSelect.svelte
  src/lib/ui/MultiSearchSelect.svelte
  src/lib/ui/MentionTextarea.svelte
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
  src/lib/ui/Toolbar.svelte
  src/lib/ui/SearchBox.svelte
  src/lib/ui/Segmented.svelte
  src/lib/ui/FilterSelect.svelte
  src/lib/ui/ShowMore.svelte
  src/lib/ui/control.ts
  src/lib/filter.ts
  src/lib/params.ts
  src/lib/pager.svelte.ts
  src/lib/board.ts
  src/lib/people.ts
  src/lib/trace.ts
  src/lib/terminal.ts
  src/lib/format.ts
  src/lib/components/Live.svelte
  src/lib/components/SecretField.svelte
  src/lib/components/CardActions.svelte
  src/lib/components/CardChanges.svelte
  src/lib/components/CardNotes.svelte
  src/lib/components/CardInstructions.svelte
  src/lib/components/TraceBlock.svelte
  src/lib/components/TraceRow.svelte
  src/lib/components/TraceText.svelte
  src/lib/components/TerminalView.svelte
  src/lib/components/FamilyForm.svelte
  src/lib/components/RequirementForm.svelte
  src/lib/components/MentionList.svelte
  src/lib/components/RefineCompare.svelte
  src/lib/components/RefinePanel.svelte
  src/lib/components/Markdown.svelte
  src/lib/components/AcceptanceList.svelte
  src/lib/components/IntakeActions.svelte
  src/lib/components/RequirementRow.svelte
  src/lib/components/ReviewersPanel.svelte
  src/lib/components/AIReviewModeField.svelte
  src/lib/components/RuleForm.svelte
  src/lib/components/StackForm.svelte
  src/lib/components/SkillForm.svelte
  src/lib/requirements.ts
  src/lib/textdiff.ts
  src/lib/intake.ts
  src/lib/api.test.ts
  src/lib/conventions.test.ts
  src/lib/intake.test.ts
  src/lib/markdown.test.ts
  src/lib/filter.test.ts
  src/lib/board.test.ts
  src/lib/people.test.ts
  src/lib/trace.test.ts
  src/lib/terminal.test.ts
  src/lib/llm.test.ts
  src/lib/components/CardActions.test.ts
  src/lib/components/CardChanges.test.ts
  src/lib/components/CardNotes.test.ts
  src/lib/components/CardInstructions.test.ts
  src/lib/components/TraceRow.test.ts
  src/lib/components/TraceBlock.test.ts
  src/lib/components/TerminalView.test.ts
  src/lib/pager.svelte.test.ts
  src/routes/cards/[id]/trace/page.test.ts
  src/routes/settings/deploy/page.test.ts
  src/lib/components/IntakeActions.test.ts
  src/lib/components/ReviewersPanel.test.ts
  src/lib/components/Markdown.test.ts
  src/lib/components/RequirementForm.test.ts
  src/lib/requirements.test.ts
  src/lib/components/RefineCompare.test.ts
  src/lib/components/RefinePanel.test.ts
  src/lib/textdiff.test.ts
  src/routes/intake/[id]/page.test.ts
  src/routes/intake/new/page.test.ts
  src/routes/choose-tribe/page.test.ts
  src/routes/layout.test.ts
  src/routes/layout-load.test.ts
  src/routes/login/page.test.ts
  src/routes/projects/page.test.ts
  src/routes/projects/new/page.test.ts
  src/lib/components/SecretField.test.ts
  src/lib/ui/SearchSelect.test.ts
  src/lib/ui/MultiSearchSelect.test.ts
  src/lib/ui/PasswordInput.test.ts
```

`src/lib/types.ts` mirrors the JSON in 0002. If a field is added to the API, add it here in the same change.

`src/app.html` keeps the default SvelteKit shell: a skip link is added in `+layout.svelte`, and `%sveltekit.body%` is the only body slot.

## Session and routes

`src/lib/session.ts` reads and writes `sessionStorage` keys `orchestrator.bearer`, `orchestrator.tribe`, and `orchestrator.project`. It stores the bearer, the selected tribe id, and the selected project id. Clearing the session removes all three. It does not store GitHub tokens, passwords, model keys, `tokenKey`, or the deploy token.

`src/routes/+layout.ts` loads in the browser. When the path is not `/login` and no bearer is stored, it `redirect`s to `/login`. When a bearer is stored but no tribe is, a project route (`/`, `/intake`, `/cards`, `/settings`, and the pages under them) `redirect`s to `/choose-tribe`. Account, admin, tribe, and project pages open without a tribe, so an admin on a new install can create one.

| Path | Page |
| --- | --- |
| `/login` | Email and password |
| `/choose-tribe` | Pick the tribe to work in, after every sign-in |
| `/` | Board for the selected project |
| `/intake` | Ingest and accept for the selected project |
| `/intake/new` | Add a requirement by hand |
| `/intake/[id]` | Edit or delete a manual requirement, and name its reviewers |
| `/cards/[id]` | Card |
| `/cards/[id]/trace` | Trace |
| `/account` | Name, password, and the caller's GitHub settings |
| `/account/usage` | My token usage (RFC-090) |
| `/admin/usage` | Token usage across every project (RFC-089) |
| `/admin/users` | Admin user list |
| `/admin/users/new` | Create a user |
| `/admin/users/[id]` | Edit role, admin, disabled, and an optional password |
| `/admin/github` | Company GitHub account |
| `/admin/llm` | LLM families for the company |
| `/admin/llm/new` | Create an LLM family |
| `/admin/llm/[family]` | Edit, test, or delete one LLM family |
| `/admin/router` | Router for the company |
| `/admin/conventions` | Coding rules by Global and by stack, and a Stacks tab |
| `/admin/conventions/new` | Add a rule, or a stack with `?kind=stack` |
| `/admin/conventions/[id]` | Edit or delete one rule or stack |
| `/admin/skills` | Integration skills |
| `/admin/skills/new` | Add a skill |
| `/admin/skills/[id]` | Edit or delete one skill |
| `/tribes` | Tribes |
| `/tribes/new` | Create a tribe |
| `/tribes/[id]` | Tribe members |
| `/tribes/[id]/usage` | Token usage of the tribe's projects (RFC-090) |
| `/projects` | Projects of the active tribe |
| `/projects/new` | Create a project |
| `/settings/repository` | Project repository, base branch, and Sync |
| `/settings/links` | Linked projects and their shared paths |
| `/settings/deploy` | Deploy and checks for the project |
| `/settings/conventions` | The project's stacks, enabled skills, and the rules that apply |

`+layout.svelte` renders the Marauder shell on every page except `/login` and `/choose-tribe`. The product name in the shell is Marauder. The sidebar groups are Workspace (Board, Intake), Project (Projects, Repository, Linked projects, Deploy, Conventions), and Admin (Token usage, Users, Tribes, Company GitHub, LLM APIs, Router, Conventions, Skills). LLM APIs, Router, Conventions, and Skills are company settings (RFC-052, RFC-055), so they do not wait for a selected project. Admin links render when `GET /api/me` has `admin: true`. A user who is not an admin but has a non-empty `tribeAdminOf` sees a Tribe group with the Tribes link (RFC-057). Those links are not a card permission check. A `403` or `404` from the API is still shown.

The sidebar shows the selected tribe's name with a "Switch tribe" link to `/choose-tribe`, then a project `SearchSelect` bound to `orchestrator.project`. Options come from `GET /api/projects`, only those whose `tribeId` is the selected tribe, each with its repository name as the detail line, and the search matches name or repository. Choosing a different project stores the id and reloads the board. A stored project outside the selected tribe is replaced by the tribe's first project, or cleared when the tribe has none. A stored tribe that `GET /api/tribes` no longer returns is cleared and the browser goes to `/choose-tribe`. The signed-in user menu shows the caller's name (or email when unnamed) and initials from it, links to Account and My token usage, and signs out with `DELETE /api/session`. Below 1024px the same links open from a menu button. The top bar shows the current tribe and project as `Tribe / Project`, and the live status line. With no project in the tribe, the board shows "This tribe has no projects yet." with a link to Projects. The page title and page actions sit in the body, in the `Page` header.

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

A requirement statement is markdown, and so is a card note with source `review` or `approver`. `$lib/markdown` parses it into a token tree and `Markdown.svelte` renders that tree as elements with text nodes, never as an HTML string. The subset is paragraphs, ATX headings (rendered from `h3` down), bullet and ordered lists with one nested level, fenced code, blockquotes, rules, inline code, bold, italic, links, autolinks, and hard breaks. Tables, images, raw HTML, and setext headings stay literal text. A link becomes `<a target="_blank" rel="noreferrer noopener">` only under the same URL rule, `https://github.com/`, or `http://127.0.0.1` with an optional numeric port followed by `/`, `?`, `#`, or the end; any other link, including `http://127.0.0.1:1@host`, reads as `label (url)`. Each acceptance line is inline markdown: `Markdown` with `inline` renders inline code, bold, italic, links, and autolinks, and `AcceptanceList` shows the lines as a checklist. `plainText` gives the one-line excerpt for clamped cells.

Buttons that submit a form use `type="submit"`. Buttons that do not submit use `type="button"`. Every input has a `<label for>`.

`Live.svelte` is one paragraph:

```svelte
<p role="status">{message}</p>
```

Pages set `message` to "Loading board", "Queueing ready cards", "Saving GitHub settings", or the error string. The paragraph stays in the page so the text change is announced.

## Search and filters

A list page that can grow long has a `Toolbar` above it, and every page filters the same way. Filtering runs in the browser on the list the page already loaded. The API takes no filter parameters.

- `SearchBox` is a search input with a screen-reader-only label. It writes `q` to the URL 200 ms after the last keystroke, and Enter does not submit a surrounding form. `matchText` in `src/lib/filter.ts` matches when any searched field contains the query, ignoring case. A blank query matches everything.
- `Segmented` is a button group for a few fixed choices, with `aria-pressed` and an optional count on each button. `FilterSelect` is a compact native select for longer or data-driven choices. Both have screen-reader-only labels and are 44px tall.
- Filter state lives in the URL, so a reload, Back, or a return from an edit page keeps it. `setParams` in `src/lib/params.ts` updates the URL with `replaceState`, `keepFocus`, and `noScroll`, and an empty value removes the parameter. `pickParam` ignores a value that is not a known option.
- A long list renders 50 rows, then `ShowMore`: "Showing x of y" and a "Show n more" button. `pager` in `src/lib/pager.svelte.ts` goes back to 50 rows when a filter changes, and not when a poll replaces the data.
- When filters hide every row, an `EmptyState` titled "No ... match" offers "Clear filters", which removes the page's filter parameters. When the list itself is empty, the page keeps its own empty state and shows no toolbar.

| Page | Search | Filters | Show more |
| --- | --- | --- | --- |
| Board | Card id, title, status, owner and reviewer names, emails, and logins, requirement ids | `owner` avatars (comma list of user ids, `none` for no owner), `blocked=1` Blocked only | No |
| Intake | See Intake | `tab`, `status`, `source` | Yes |
| Projects | Name, repository, base branch | None; the list is the active tribe's projects | No |
| Tribes | Name | None | No |
| Tribe members | Name, email | `role` | Yes |
| Users | Name, email | `role`, `access` All, Admins, Active, or Disabled with counts | Yes |
| Conventions, Rules tab | Title and body | `category`, `applies` Any scope, Global, or one stack | No |
| Conventions, Stacks tab | Name and markers | None | No |
| Skills | Slug, title, "use when", body | `state` All, Enabled, or Disabled with counts | No |
| Project Conventions | Rule title and stack name | `category`, on the rules table only | No |
| LLM APIs | Family id, base URL, model ids | `protocol` All, Anthropic, or OpenAI and `key` All, Saved, or Not set, with counts. Any protocol other than `openai` counts as Anthropic | No |
| Card trace | Kind and its label, attempt id, the row's label and detail, payload JSON | `kind` ("Tool call (88)"), and `attempt` ("Attempt 8 (24)") when there is more than one attempt, built from the events present with counts | Yes |

Router, Company GitHub, Linked projects, and Account have no toolbar. Their lists are short or are edit forms.

## Login

`src/routes/login/+page.svelte`. Email and password fields, and a submit button "Sign in". Submit calls `POST /api/session` with `{ email, password }`. Success stores the bearer and `goto('/choose-tribe')`. Failure leaves the typed password out of the live region and sets the live message to `error.message`. A `429` says to wait one minute. The password input is a `Field type="password"` with `autocomplete="current-password"`; its eye toggle shows or hides the typed password.

`src/routes/choose-tribe/+page.svelte` opens after every sign-in, even for a member of one tribe, without the sidebar. It lists the tribes from `GET /api/tribes` (an admin gets every tribe) as buttons at least 44px tall, each with its name and "N projects" counted from `GET /api/projects`, and marks the stored tribe as current. Picking a tribe stores its id and the id of its first project, or an empty project when it has none, and loads `/`. With no tribe it says "You are not in a tribe yet. Ask an admin to add you.", and an admin also gets a "Create a tribe" link to `/tribes/new`. A ghost "Sign out" button ends the session. A failed load shows the API error in the live region.

## Board

`src/routes/+page.svelte` calls `GET /api/cards` from `onMount` (or `$effect` on first run).

Render six sections in this order: Waiting, Running, Review, Shipping, Finished, Stopped. Place each card by `group`. Print the status as words from `statusLabel` in `src/lib/status.ts`, such as "Waiting for review" for `human_review`, with the raw status in `title`. Below it, the row shows the owner's initials avatar and display name, `name`, then `githubLogin`, then the email, or a dashed avatar and "No owner" (RFC-054, RFC-058). Every page that lists a person uses that order (`displayName` in `src/lib/people.ts`) and the `Avatar` component. When `reviewers` is not empty, the same line ends with up to three overlapping reviewer avatars and "+n" for the rest. Their names are in the `title` and `aria-label` ("Reviewers: ..."). The name clamps to one line. The tile is `src/lib/components/BoardCard.svelte`. Its body links to `/cards/{id}`.

Below the link, outside it, a ready card has a footer row (RFC-061). It shows a 44px "Run" button when `actions` includes `run`, a "Queued" pill when `queued` is true, or "Waiting on N cards" when `waitingOn` is not empty, with the ids in `title`. Run POSTs `/api/projects/{id}/cards/{card}/run`, swaps the returned card into the list, and sets the message to "Queued {title}." A `409` reloads the board with "Card changed. Reloaded." The card page shows Run through `CardActions`, where it first opens the instructions box (RFC-073).

The primary button reads "Run ready cards (N)", where N is `readyCount`. It is enabled when `canRun` is true and N is above zero. Its `title` says why when it is disabled: "All ready cards are queued", "No ready card can start", or "Your role cannot start cards". It POSTs `/api/projects/{id}/runs`, then reloads the list, and the message reads "Queued N cards." The page says "Queued" and never "Running" for these cards, because the worker loop is not built yet.

When `running` is true, or any card is in the `running` or `shipping` group or in `agent_review`, an `$effect` starts a 3 second `setInterval` that calls `GET /api/cards` again. The effect cleanup clears the timer when the condition becomes false and when the page is destroyed.

Above the lanes, a toolbar searches the cards, filters them by owner, and has Blocked only (see Search and filters). The owner filter is a row of overlapping 36px avatars in 44px toggle buttons, one per distinct card owner (`ownerChoices` in `src/lib/board.ts`). The signed-in user comes first, matched by `owner.email` ignoring case (RFC-058). The others follow by display name. A dashed "No owner" circle comes last when a card has no owner. Each button has `aria-pressed`, the label "Cards owned by {name}", and the name and card count in `title`. Several owners can be on at once, and the board keeps cards owned by any of them. Past six owners, the rest sit behind a "+n" button that opens a checklist. It has a ring while one of its owners is selected. The selection is `?owner=` with user ids separated by commas. An old `?owner=mine` link maps to the signed-in user's id, or is dropped when that user owns no card. Clear filters also clears the owners. While a filter is on, each lane header counts "x of y". The poll keeps the filter.

Empty `cards`: one paragraph, "No cards yet", and a link "Ingest requirements" to `/intake`. No table of placeholder rows.

## Card

`src/routes/cards/[id]/+page.svelte` loads `GET /api/cards/{id}` using the `params.id` from the page.

The header shows the title and a badge with `statusLabel`, with the raw status in `title`.

Above Notes, when `actions` includes `instructions`, `CardInstructions` shows an Instructions panel (RFC-073) with the hint "Every later attempt reads these with the newest note, until you change them. An attempt that already started keeps the prompt it started with." It renders `instructions` with `Markdown`, or "No instructions. Attempts follow the requirements and the newest note." when it is `null`. The panel's Edit button opens a textarea "Instructions for later attempts", prefilled, growing, with the hint "Leave it empty to clear them. Markdown works.", and Save and Cancel buttons. Save PUTs `{ instructions }` to `/api/projects/{id}/cards/{card}/instructions`, replaces the card with the response, and closes the form. A `422` shows `fields.instructions` under the textarea.

Above Next step, `CardNotes` shows a Notes panel when `notes` is not empty, with the hint "Newest first. The next attempt gets the newest note and the current instructions." Each note shows its source as a badge (AI review, Approver, Local check, CI checks, Dev check, No commit, or Instructions, and the raw source when unknown), its local time, and "attempt N" when `attemptN` is above 0. A `review`, `approver`, or `instructions` note renders with `Markdown` and headings from `h4`, and a blank `instructions` note reads "Instructions cleared". Any other note is check output and renders with `TraceText`. The notes the next attempt reads are open: the newest note that is not `instructions`, and the newest `instructions` note when it is not blank. When neither applies, the newest note is open. The rest wait behind "Show N earlier notes".

Under Next step, the page has six panels in this order:

- People: the owner and each reviewer with avatar, display name, email, and "GitHub @login" or "No verified GitHub login", labelled Owner or Reviewer. With no reviewers, a line says reviewers come from the card's requirements and links "Edit reviewers" to `/intake/{first requirement id}`.
- Intake: each of `requirements` with its id, heading, the statement as a `plainText` excerpt clamped to three lines, and a Draft badge when not accepted. Each links to `/intake/{id}`.
- Rules: `rules` grouped by `label`, Global first, each with its title and category badge. The footer links to `/settings/conventions`.
- Skills: each skill's title and `useWhen`, clamped to two lines.
- Details: decision ids, risk, attempts used and max attempts, local check, dev check, branch, pull request link, head SHA, reviewed SHA, whether review was independent, "Approval: Self review: the owner approves alone" when `selfReview` is true (RFC-070), "After AI review: A person reviews" when `aiReviewMode` is `manual` (RFC-072), approved SHA, deployed SHA, failure reason, and blocked reason. Reasons read as words from `reasonLabel`, with unknown codes in sentence case. Omit a row when the value is null. Do not replace null with a fake SHA. When `blockedReason` is `owner_github`, a notice says the owner saves a token on the Account page and runs Test connection, then the card can be unblocked.
- Changes (`CardChanges`, RFC-068): loads `GET /api/projects/{id}/cards/{card}/diff` once per card id and `headSha`. The header shows the file count and the total added and deleted lines. Each file is a button with `aria-expanded` that shows its path, `+a -d` or "binary", and opens its hunks. Added lines have a green background and a `+`, deleted lines a red background and a `-`, as text nodes in a monospace block that scrolls sideways. After 50 files, Show more reveals the rest. A cut diff shows "The diff is large, so it is cut", a missing commit shows "Commit {short} is not in the project clone yet.", and a card with no commit says there is nothing to show. The footer links "View these changes on GitHub" to `compareUrl` when it is set. A load error shows in the panel and does not hide the rest of the page.

Render a control only when its name is in `actions`:

| Name | Control |
| --- | --- |
| `run` | Button "Run" with "Queues this card for the worker. You can add instructions first." It opens a form: textarea "Instructions for the next attempt", 4 rows, growing, prefilled with `instructions`, hint "Optional. Every later attempt reads these with the newest note, until you change them. Markdown works." Buttons "Run" and "Cancel". The POST body is `{ instructions }` only when the trimmed text differs from `instructions`, else `{}` (RFC-073). The board's Run stays one click |
| `retry_review` | Button "Retry AI review" with "The AI review gave no verdict. The worker runs it again on this commit." It POSTs `/api/projects/{id}/cards/{card}/retry-review` with `{}` (RFC-076) |
| `approve` | Form. SHA input defaulted from `headSha`. Button "Approve SHA". |
| `request_changes` | Menu item "Request changes" opens a form. Textarea "What should change?", 4 rows, growing, hint "Optional. The next attempt gets this note. Markdown works." Buttons "Request changes" and "Cancel". The body is `{ reason }` |
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

After a successful POST, replace the page state with the returned card. On `422`, keep the form values and show `fields.sha` under the SHA input, `fields.reason` under the reason, `fields.instructions` under the instructions, or `error.message` in the live region. A successful Request changes closes and clears the reason form. A successful Run closes the instructions form. On `409`, reload the card and set the live message to "Card changed. Reloaded."

A link "Trace" goes to `/cards/{id}/trace`.

`in_progress` has an empty `actions` array. The page shows the fields and no approve form. An `approved` card shows Deploy only when `actions` includes `deploy`, plus Request changes and Reject when those names are present, and no Approve. Those are the checks the verification guide looks for. The page does not show Deploy because the status is `approved`.

## Intake

`GET /api/projects/{projectId}/intake`.

The page has two tabs, Requirements and Decisions, each with its count. `?tab=decisions` selects Decisions. The header badge says how many requirements are accepted.

A toolbar sits above each table. The search box matches id, statement, heading, source path, and acceptance lines, ignoring case. On Decisions it matches id, statement, and source path. The Requirements tab also has a status filter, "Needs acceptance", "Accepted", or "All", each with its count, and a source select: All sources, PRD, RFC, Manual, or Draft. The filters live in the URL as `q`, `status`, and `source`, so returning from `/intake/[id]` keeps them. Without `status`, the page shows "Needs acceptance" while any requirement is unaccepted, and "All" otherwise. When nothing matches, an empty state offers "Clear filters".

Requirements are a table: a select checkbox, id, statement as a `plainText` excerpt clamped to two lines with the source path under it ("Added by" with `createdBy` for a manual row), source badge ("Draft" in amber), reviewer count, the first of `linkedCards` as a link to `/cards/[id]` with "+n" for the rest, Accepted or Not accepted, and a details button with `aria-expanded`. The checkbox column renders only when a shown row can be accepted. Under 1280px wide the reviewer column is hidden, and under 720px the source and card columns are hidden too. Clicking the statement or the details button opens a row under it with the full statement rendered as markdown, the acceptance lines each rendered as inline markdown, the source path or "Added by" with `createdBy`, "Ingested by" for a parsed row with `createdBy`, the `reviewers` as badges by name, and the card links. A manual row has an "Edit requirement" link to `/intake/[id]`. A parsed row whose `canEditReviewers` is true has an "Edit reviewers" link to the same page (RFC-054). Decisions are a table of id, statement, and source path.

Each table renders 50 rows, then a "Show 50 more" button. Changing the tab, search, or a filter goes back to 50 rows and drops selected ids that are no longer shown.

Button "Ingest" POSTs `ingest` and replaces the list. When the response has `skipped`, a notice names those ids and says the manual text was kept. Button "Add requirement" links to `/intake/new`.

Acceptance needs `canAccept` (RFC-053). When it is false, there are no checkboxes and no accept buttons. When it is true, each unaccepted row has a checkbox, and the header checkbox selects every unaccepted row that the filter shows, including rows past the first 50. While any row is selected, a sticky bar at the bottom says how many, with Clear and the page's primary button, "Accept selected", which POSTs `accept` with the selected ids. "Accept all shown (n)" in the toolbar POSTs `accept` with every unaccepted id the filter shows. Both accept buttons are disabled while `draftPending` is true. After an accept, the selection is cleared and the page shows the returned intake.

`/intake/new` has an optional id field with the hint "Leave blank to use the next free REQ- number.", a statement, and acceptance lines, one per line. `RequirementForm` puts one Write/Preview `Segmented` control, labelled "Requirement view", above the statement and acceptance. Write shows a statement textarea of 8 rows with the hint "Markdown works: headings, lists, code, bold, italic, and links." and an acceptance textarea of 6 rows with the hint "One check per line. If you paste a markdown list, only its items are saved. Markdown works inside a line: code, bold, italic, and links." Preview renders the statement with `Markdown` and the acceptance lines, split by `toLines` as they will be saved, with `AcceptanceList`; an empty one shows "Nothing to preview", and each field keeps its API error under its preview. `toLines` (through `acceptanceChecks`) reads the textarea the way ingest reads an Acceptance section: when the text has a markdown list, `listItems` keeps only its items, each as its own inline markdown, a wrapped item joined with a space and a nested item as its own check, and leaves out headings, bold labels, prose, rules, and fenced code. Preview then says "N lines are not list items, so they are not saved." Without a list, each non-blank line is a check. A leading list or checkbox marker is dropped either way, the same rule as the API. `/intake/[id]` loads saved checks with `fromLines` as a `- ` list, so adding a list item later keeps them. Both textareas grow with their text (`Field` `grow`), so the page scrolls instead of the box. It POSTs `requirements` and returns to `/intake`. `/intake/[id]` shows the id read-only, edits the statement and acceptance with `PUT`, and deletes after a confirm step. For a row that is not manual it says to edit the requirement in the repository and shows no requirement form. Field errors come from the API `fields`.

Both pages pass `refine` to `RequirementForm` (RFC-086, RFC-087): a `RefineStream` that calls `streamRefine` with the requirement id (the typed id, or blank, on `/intake/new`). In Write, the form then puts a spark icon button (`Icon name="spark"`) at the right of the Statement and the Acceptance label rows, through `Field`'s `labelAction` snippet. Each is 44px square, labelled "Refine statement with AI" or "Refine acceptance with AI", with `aria-expanded` and `aria-controls`, and toggles a `RefinePanel` under its own field. Both panels may be open and streaming at once. Preview shows no icons and no panels, and leaving Write aborts a running stream.

MCP mentions (RFC-101). Both pages pass the intake's `mcpServers` to `RequirementForm` as `servers`, `[]` when the field is missing. The statement and acceptance are `MentionTextarea` fields, which keep `Field`'s label, hint, error, and growth, and add the hint "Type @ to name an MCP server." Typing `@` at the start of the text, or after a space or an opening bracket or quote, opens a listbox under the field with the tribe's slugs that start with what follows the `@`, each with "N tools" or "Disabled" as the detail; with no server it shows "No MCP servers in this tribe." While the list is open, the arrow keys move, Enter or Tab inserts `@slug ` in place of the typed text, and Escape closes it; while it is closed, the keys do what they do in any textarea, and keys during IME composition are ignored. The textarea keeps its role and sets `aria-autocomplete="list"`, `aria-controls`, `aria-expanded`, and `aria-activedescendant`; options are 44px tall. Under each field a list labelled "MCP servers mentioned" shows the field's mentions from `mentions` in `$lib/requirements`: a known enabled server as a plain badge, a disabled one amber with "disabled: jobs will not call it", and a slug the tribe does not have red with "no server with this slug". These never block Save. On `/intake/[id]` a row that is not manual shows its stored `mentions` the same way, and the expanded row on `/intake` adds "MCP servers" with the row's mentions as badges.

`RefinePanel` handles one field. A `reset` event empties the text so far, because that turn ended in MCP tool calls, and a `tool` event shows "Looking up <server> / <tool>" in place of the model line until the next delta (RFC-095). It has a "What to improve (optional)" input and a secondary "Refine" button. With a blank statement it says "Write a statement first, then refine it." (or, for acceptance, "Write a statement first. Acceptance is refined against it.") and sends nothing. Otherwise it calls the stream with `field`, the trimmed statement, `toLines(acceptance)`, and the guidance, and an `AbortController` of its own. While streaming it shows "Waiting for the implement model…", then "Writing with <model>" after `start`, and the text as the deltas arrive in a region labelled "Refined statement so far" (or acceptance) with `aria-live="polite"`, `aria-busy`, and a blinking caret (`motion-safe:animate-pulse`). "Stop" aborts the fetch and says "Stopped. Nothing changed." An `error` event, a rejected request, or a stream that ends without `done` shows the message in an alert and the button reads "Try again". On `done` it shows `RefineCompare` for that field.

`RefineCompare` takes `field`, `before` and `after` (for acceptance, the lines joined by newlines), and `modelId`. It has a "Refined by <model>" heading, a "<Field> comparison view" `Segmented` control with Changes, Before, and After, the statement as a word diff from `diffWords` in `$lib/textdiff` with `<ins>` and `<del>` and screen-reader "Added:" and "Removed:" labels, or the acceptance lines as a line diff from `diffLines` marked `+` and `−` with the added and removed counts. Before and After render each version with `Markdown` or `AcceptanceList`. When that field changed after the request, it says "You edited this field after asking. Apply replaces those edits." "Apply" sets only that field (the statement, or `fromLines` of the lines) and closes the panel; "Discard" returns the panel to its input. Nothing is saved until the page's Save or Add requirement. The edit page's panel hint is "Changing the text, by hand or with the refine buttons, clears acceptance when you save." Rows that are not manual have no form, so no refine buttons.

`streamRefine(body, onEvent, signal)` in `api.ts` uses `fetch` with the session bearer, because `EventSource` cannot send an `Authorization` header. It shares `send` with `request`: the bearer, the JSON content type, and the 401 handling. A non-OK reply becomes `ApiError` from its JSON body. On `200` it reads `response.body.getReader()` with a streaming `TextDecoder` (`readStream`, shared with `streamTrace`), and `parseEvents` splits the buffer on blank lines, keeps an unfinished event for the next chunk, skips comment lines, and turns each `event:` and `data:` pair into an object whose `kind` is the event name, set after the data so a payload's own `kind` cannot replace it. An abort rejects with the fetch's `AbortError`, not a network error.

Both pages name PR reviewers with a `MultiSearchSelect` (at most 15). Options are `reviewerOptions` without the row's creator, labelled by name with the login and email as the detail line, or "No verified GitHub login" and the email for a member without a login (RFC-103). The search placeholder is "Search by name, login, or email", or "No other tribe members yet" when there is no option. A hint under the picker reads "Someone without a verified GitHub login is asked to review on GitHub after they verify." On `/intake/[id]` a Reviewers panel shows for every row: editable with "Save reviewers" when `canEditReviewers` is true, which PUTs `requirements/{id}/reviewers` with `{ userIds }`, and read only otherwise. On `/intake/new` the picker sits under the acceptance lines and leaves out the caller. After the POST, a non-empty pick is saved with the same PUT. If that PUT fails, the page opens `/intake/[id]` for the new row so the reviewers can be saved again.

Self review (RFC-070) is a `Checkbox` under the reviewer picker. On `/intake/new` it shows only when `canSetSelfReview(shell.role, shell.admin)` from `$lib/requirements` is true, with the hint "You review and approve this requirement's cards yourself. The approval is recorded as not independent.", and the POST sends `selfReview`. In the Reviewers panel on `/intake/[id]` it shows when the row's `canSetSelfReview` is true. "Save reviewers" then PUTs `reviewers` when the picks changed and `self-review` with `{ selfReview }` when the box changed. A caller who cannot change it sees "Self review: on. The card owner approves alone." when the flag is on, and nothing when it is off.

The AI review mode (RFC-072) sits under Self review in both places and follows the same rule for who sees it. `AIReviewModeField` is a `Segmented` control labelled "When the AI review asks for changes" with "Run the next attempt" (`continue`) and "A person reviews" (`manual`), from `reviewModes` in `$lib/requirements`, and a hint under it that explains the chosen option. On `/intake/new` the POST sends `aiReviewMode`, and `continue` when the caller cannot choose. In the Reviewers panel "Save reviewers" also PUTs `ai-review-mode` with `{ aiReviewMode }` when the choice changed. A caller who cannot change it sees "When the AI review asks for changes, a person reviews before another attempt." when the row is `manual`, and nothing for `continue`.

While `draftPending` is true, the live message is "Planner is drafting requirement ids" and the page polls `GET /api/intake` every 3 seconds. When `planError` is set, show it under the heading as text. The live message distinguishes "Reading docs/prd and docs/rfc" from "Accepting 3 requirements".

## Trace

`GET /api/projects/{projectId}/cards/{id}/trace`, `GET /api/projects/{projectId}/cards/{id}/trace/stream` while the card works, and `GET /api/projects/{projectId}/cards/{id}` for the title and status. No model call from this page.

While the response's `live` is true and the tab is visible, the page holds one trace stream (RFC-088): `streamTrace(id, since, onEvent, signal)` in `api.ts`, with `since` set to the newest event's `ts`, read with `fetch` and the bearer like `streamRefine`. A `trace` frame merges its events by `id` (`trace.mergeEvents`, ordered by `ts` then `seq`) and sets `attempts` and `live`. `turn` starts the reply being written (`pending`: attempt, turn, role, model, and text), `delta` text for that turn is added once per animation frame, `reset` empties it, and a merged `model_call` of the same attempt and role, or `live` turning false, clears it. `done` with `reconnect` and `resync` open the stream again; `resync` first reads the whole trace. The stream closes while the tab is hidden and opens again when it is shown. When the stream fails or ends without `done`, the page reads the trace route every 3 seconds (every second in the terminal view) with `since`, as RFC-074 did, and tries the stream again after 5 seconds. A poll is skipped while a request is in flight, a response for another card is dropped, and a failed poll keeps the rows already shown. The card is read again when `live` changes or an attempt ends. Events are kept in `$state.raw`.

The page reads as a timeline, oldest first:

- Header: "Trace", the card title as the description, and the status badge in `meta`, with a sky "Live" badge while `live` is true, titled "This page shows output as it arrives".
- Now running: while `live` is true, a strip above the summary shows "Attempt N" (or "Waiting for the worker" before an attempt opens), the phase from the newest status change, the elapsed time from `startedAt` ticking each second, "Turn X of Y" from the attempt's implementer model calls and `maxTurns`, the attempt's tokens, and the newest event's label, detail, and age, or "The model is writing…" while a reply streams. The live region announces only when an attempt ends, such as "Attempt 2 ended: Checks passed".
- Follow latest: a toolbar checkbox shown while `live` is true, on by default. When a poll adds rows, the page scrolls the newest entry into view. When every timeline entry was shown, new entries show too (`pager.follow`); a block that showed every row keeps showing every row as rows arrive, and a block keeps its open state.
- Summary: a strip shows attempts, events, total model tokens, errors (a link to `?kind=job_error` when above zero), and the first and last event time.
- Day headers split the timeline where the local day changes.
- Milestones: events without an attempt, such as status changes, refused status changes, errors, and reviewer requests. Consecutive milestones share one bordered list.
- Attempt blocks (`TraceBlock`): a run of consecutive events with one `attemptId`. The reviewer's events carry the implement attempt's id, so they form a block titled "Review of attempt N"; a later run of the same attempt and role is "(continued)". The header is a button with `aria-expanded` that shows the task class, model, model calls, tools, tokens in and out, duration (`startedAt` to `endedAt` for an implement block, first to last event otherwise), a red problem count, and the outcome badge and short commit, or for a review the verdict and "Not independent". The latest block and any block with a problem start open. Every block starts open while a search or filter is set. A block shows 50 rows, then `ShowMore`.
- Rows (`TraceRow`): one 44px line per event with the local clock time (ISO in `title`), an icon, a label, a detail, and right-aligned meta such as "1,616 in / 74 out". `trace.describe` gives every kind a template; an unknown kind reads as its name in sentence case. A tool row puts its `args` first, such as "src/x.ts · 120 lines", `"useSession" · 3 matches`, "exit 0 · npm test", or a commit's first line. A model row with `text` shows the reply's first line as its detail and the whole reply when open; without `text`, the model id and stop reason. Red rows are errors, failed tools, failed checks, and moves to Blocked; amber rows are refused calls and changes requested. A row is a button with `aria-expanded`; open, it shows the full detail, the long text, the kind, time, attempt id, and event id, and a "Raw event" toggle with the JSON.
- Long text (`TraceText`): control characters other than newline and tab are stripped, text is cut at 4,000 characters with "Show all (size)", and text that was mostly NUL bytes is hidden as binary output until "Show anyway". Every value renders as text.

The toolbar searches the kind and its label, attempt id, the row's label and detail, and payload text, and filters by kind and attempt. The timeline renders 50 entries at a time.

The toolbar's View switch (`Segmented`) picks Structured, the timeline above, or Terminal, kept in `?view=terminal` (RFC-078). The filters apply to both. The terminal view (`TerminalView`) is one `bg-ink` monospaced box with `role="log"`, `aria-label="Terminal output, oldest first"`, and `tabindex="0"`, at most 75% of the viewport high. `terminal.terminalLines` turns each event into a local clock time, a head line, and its text:

- A model call: `model  role  modelId  1.5k in / 20 out  stop tool_use`, then the reply. Sky.
- A tool call: `$ read_file  src/x.ts`, then the whole stored `summary`. An `mcp_<slug>_<tool>` row is labelled "MCP: slug / tool" in both views, with its byte count as the detail; `mcp_context` ("MCP servers") lists each server with its tools and short hashes, or its error, and is amber when a server failed; a database server is located as `host/database` (RFC-097). When `scope` is `mentioned` the detail says the servers come from the requirements' mentions; when it is `none` the row says "No MCP servers" with the `reason`, and each `missing` slug is listed as "not a server: slug"; either of those is amber (RFC-102). `assertion_failed` `mcp_changed` reads "MCP tool changed: slug / tool". A commit's head is its first line; `edit_file` adds "(N lines)"; `move_file` shows `from -> to`, a ranged read adds "from line N", and `run_command` shows its argv and is coloured like a check; `comment_pr` and `verdict` add the body; the two context tools list their items. A refused call adds `(refused)`. Failed and refused calls and a check not starting with `exit 0` are red, a passing check green.
- A status change: `transition  from -> to  evidence`, amber, or red into `blocked`. A job error: `! job_error` and the whole message, red.
- Any other kind: its name and `trace.describe` detail and text. An unknown kind shows its payload as JSON.
- `--- attempt N  taskClass  modelId  ---`, muted, where another attempt's events begin.

ANSI color codes and control characters are stripped before `cleanText`, with no length cap. Text past 200 lines (`foldLines`) shows "Show all N lines". The view renders the newest 300 entries (`pageEvents`), and "Show earlier events (N hidden)" adds 300. The view scrolls to the bottom when entries arrive while `follow` is on. Scrolling more than 24px above the bottom turns `follow` off and shows "Jump to latest". `follow` binds to the page's Follow latest checkbox.

`TerminalView` takes `pending`, the reply being written, and renders it after the entries through `terminal.pendingEntry`: the head `model  role  modelId  writing…` in sky, then the text so far with ANSI codes stripped, or "Waiting for the model…" before the first text, and a pulsing caret (`aria-hidden`, still under reduced motion). While it shows, the log has `aria-busy="true"`, so a screen reader is not sent each piece. The view keeps following the bottom as the text grows. The page passes no `pending` when the Kind filter is set to anything but Model call, the Attempt filter names another attempt, or the search does not match the text.

## Settings pages

`SecretField.svelte` is used for the caller's GitHub token, the company GitHub token, each LLM key, and the deploy token.

1. Input is a `PasswordInput` (masked, with the eye toggle), autocomplete `off`.
2. If the GET flag (`tokenSet`, `keySet`, or `deployTokenSet`) is true, placeholder "A value is saved".
3. Empty input on submit means the JSON omits that secret field.
4. A "Remove" button sets `clear: true` and omits the secret string.
5. The value is never written to the live message or the console.

Disable the submit button and set the live message while the PUT is in flight. On `422`, paint `fields` under the matching inputs and keep what the operator typed.

The four project settings pages (Repository, Linked projects, Deploy, Conventions) read `canEdit` from their GET body (RFC-059). When it is false, the fields sit in a disabled `fieldset`, or the pickers get `disabled`, and `SaveBar` gets `disabled`. Its note reads "Only a Lead Developer, a tribe admin, or an admin can change these settings." When it is true, the note names no role. The client never decides `canEdit` from the role itself.

### Account

`src/routes/account/+page.svelte`. `GET /api/me` and `GET /api/me/github`.

The first panel is Name. "Save name" calls `PUT /api/me` and is disabled while the name is unchanged. A `422` shows on the field. After a save the sidebar menu shows the new name. The page title is the name, or the email when unnamed.

Password fieldset: current password, new password, button "Change password", `PUT /api/me/password`.

GitHub fieldset: host, login, token. The help text says this is the caller's own account, and that cards built from requirements the caller brought in are committed, pushed, and opened as pull requests with this token (RFC-054). Clones and fetches still use the company account. The panel header shows a "Verified" or "Not verified" badge from `verified`. "Test connection" calls `POST /api/me/github/test`. Disable it when `tokenSet` is false and the token input is empty. Show `login` on success. Show `message` on `ok: false`. After the test, reload `GET /api/me/github` so the login and badge show what GitHub reported.

### Users and tribes

`src/routes/admin/users/+page.svelte` lists users. Each row shows the name with the email under it, or the email alone when unnamed. Create is `/admin/users/new` (name, email, password, role, admin) and calls `POST /api/users`. The name is required. Edit is `/admin/users/[id]` with a Name panel and calls `PUT /api/users/{id}`. It sends `name` unless the field is blank and the user never had one, so changing the role of an unnamed user still saves. A `403` sets the live message from `error.message`.

`src/routes/tribes/+page.svelte` lists tribes. Create is `/tribes/new` with a name. `/tribes/[id]` lists members from `GET /api/tribes/{id}/members` and adds one by picking a user from `GET /api/tribes/{id}/candidates` in a `SearchSelect` searched by name or email, labelled by name with the email and role as the detail line. Member rows show the name with the email under it. The request still sends `userId`. A tribe admin has a "Tribe admin" badge. "Make tribe admin" and "Remove tribe admin" call `PUT /api/tribes/{id}/members/{userId}`. Remove asks on the row, then calls `DELETE /api/tribes/{id}/members/{userId}`. A tribe admin who is not an admin sees no flag or remove control on their own row. The tribes list shows "Manage" and member counts only for tribes the user can manage. `src/routes/projects/+page.svelte` lists the active tribe's projects: those from `GET /api/projects` whose `tribeId` is the stored tribe. This holds for a company admin too, who receives every project from the API. The description reads "Projects in {tribe}. Switch tribe in the sidebar to see another tribe's projects." With no active tribe the page says "Choose a tribe to see its projects." with a link to `/choose-tribe`, and an active tribe with no project says "No projects in this tribe yet." Each row shows the name and repository. Each row has "Settings", which selects the project and opens its Repository settings, and "Open board". The page shows "New project" to an admin or a tribe admin. Create is `/projects/new` with tribe, name, a repository `SearchSelect` filled from `GET /api/settings/github/repos`, base branch, and "Linked projects". The tribe select lists only tribes the user can manage, and starts on the active tribe when the user can manage it, else on the first of them. Picking a repository fills the base branch with its default branch. The repository can only be picked from that list (RFC-060). Archived repositories are listed as disabled options. When the list cannot load, the picker stays empty and its error line says why. "Create project" is disabled until a repository is picked. "Linked projects" is a `MultiSearchSelect` of the chosen tribe's projects with the repository as the detail line. Changing the tribe drops picks from the old tribe. Shared paths are added later on Linked projects. "Create project" sends `links` with empty `sharedPaths`.

### Tribe MCP servers

`/tribes/[id]` has an "MCP servers" header button for those who can manage the tribe (RFC-092). `src/routes/tribes/[id]/mcp/+page.svelte` lists `GET /api/tribes/{id}/mcp-servers` in a table: the slug with a "Disabled" badge when off, where it is reached (the URL, `stdio · argv[0]`, or `postgres · host:port/database`), the tool count, and "Edit". The primary button is "Browse catalog" and the secondary one "Add manually" (RFC-096); with no servers the page shows "No MCP servers".

`mcp/catalog` searches `GET /api/tribes/{id}/mcp-catalog` (RFC-099, RFC-100). For each query, 300 ms after the last keystroke (at once on open), it sends `source=local` and `source=registry` with one `AbortController`; the next query aborts both. The page has three sections:

- **Featured** and **Databases**: tiles in a grid of up to three columns, from the local answer, shown as soon as it arrives. A tile has a letter mark, the title, two lines of description, a Website link, and a footer with badges and a secondary "Set up" link to `mcp/new?from=<id>` whose accessible name adds the title. A featured tile reads "Token needed" (amber) when a secret is required, "Optional token" when every secret is optional, and "No sign-in" (green) with none, plus its transport; a built-in tile reads "Built-in" (sky) and "Database". A section with no match is hidden.
- **MCP Registry**: a status line reads "Searching the MCP Registry…" while the registry request runs, with three pulsing placeholders when nothing is listed yet and the previous list dimmed otherwise, then "N servers". Each entry is a row with its title, name and version, description, a transport badge, an amber "No network" for a stdio entry, and "Set up", or "Cannot be set up: <reason>" with no link. "Load more" appears while there is a `nextCursor`. A `registryError` shows a warning notice ending ". The featured and built-in servers are still listed. Try again in a minute." A query with no registry match reads "No registry servers match “q”."

Website links open in a new tab with `rel="noreferrer noopener"`. When every section is empty and the registry has answered, the page shows "No servers found".

`mcp/new` reads `?from=`, loads that entry with `?id=`, and fills the form: slug from the name, transport, URL, image and argv, port, the secret names with "Required. " before a required hint, and a built-in entry's fixed tools. Without `from` the form is empty. `mcp/new` and `mcp/[serverId]` render `MCPServerForm`:

- Server: Slug (mono), Transport (`http`, `stdio`, "PostgreSQL, built in", or "SQL Server, built in"), then URL for http, or Image and Command (a textarea, one argument per line) for stdio, and an Enabled checkbox with the hint "Jobs offer the tools of enabled servers only." A stdio image that is not pinned shows a "Pull and pin image" button, which calls the pull route and replaces the image with the pinned one, or shows the `image` error.
- Connection (postgres and sqlserver, RFC-097): Host, Port (5432 or 1433 by default), Database, User, Password as a `SecretField` saved under `PASSWORD`, TLS (`require` by default, `verify-full`, or `disable`), and a "This login can only read" checkbox. `config.<field>` errors show under each input.
- Headers (http) or Environment (stdio): each saved name from `secretsSet` is a `SecretField` with "A value is saved" and Remove, which adds the name to `clear`. "Add header" or "Add variable" adds a Name and Value row. An empty value is left out of `secrets`.
- Tools: the allowlist, each with a "Description of <tool>" textarea and Remove. "Discover tools" calls the discover route and shows a "Discovered tools" table: a tool already allowed with the same hash reads "Allowed", one with a new hash offers "Update schema", a new one "Allow", and a name the allowlist cannot take has a red "Name not allowed" badge. A failed discovery shows its `message`.
- "Save" (or "Add server") is the one primary button. "Delete server" asks again with Cancel and Delete. A `422` paints `fields` under the inputs. After a create the page goes to `mcp/<id>?discover=1`, which runs Discover tools once without a click.

Values typed in a secret field are never shown again or logged.

### Token usage

`src/routes/admin/usage/+page.svelte` (RFC-089) calls `getUsage(from, to)` for the range and does the rest in the browser with the pure functions in `src/lib/usage.ts`. Every choice lives in the URL through `setParams`, and a default value leaves its parameter out:

- **Range**: a `Segmented` for 7, 30 (the default), or 90 days (`?range=`). `rangeWindow` turns it into UTC days ending today, and a change reads the route again.
- **Group by**: a `Segmented` for LLM, Model (the default), Project, User, Task, Source, or Card (`?by=`).
- **Filters**: a `FilterSelect` each for LLM, Model, Project, User, Task, Source, and Card (`?llm= &model= &project= &user= &task= &source= &card=`), and "Clear filters" while any is set. The value is the family, model, project, user, or card id, `intake` or `card` for Source, or `-` for a call with none ("Unknown family", "No owner", and "Intake" for a refine under Card) (RFC-091). Grouped by card, each name cell has the card id as its `title`, since titles can repeat. `usageOptions` lists each value with its total tokens, counting the rows the other filters keep, largest first.
- **Totals**: a strip in the Trace page style with total tokens, input, output, cached input, calls, and cache share (cached over input plus cached, since providers count cached input apart). Total is input plus output.
- **Tokens per day**: one CSS bar per day of the range, a day with no calls included, each with a `title`. The bars are `aria-hidden` and a visually hidden table gives the same numbers. No chart library.
- **Table**: the groups with calls, input, output, cached, total, and share of the filtered total as a bar and a percent. The name and number headers are 44px buttons with `aria-sort`; a first click sorts numbers largest first and names A to Z, and a second click reverses (`?sort= &dir=`, default total, largest first). `ShowMore` after 50 groups.

An empty range shows "No model calls in this range"; filters that keep nothing show "No calls match" with Clear filters. A `403` sets the live message from `error.message`.

The page body is `src/lib/components/UsageDashboard.svelte` (RFC-090), with props `title`, `description`, `load(from, to)`, `dimensions` (all five by default), and an optional `actions` snippet. A dimension not listed is left out of the filters and Group by, and its URL parameters are ignored; `?by=` falls back to model. The data is read again when `load` or the range changes. Three routes use it:

- `/admin/usage` with `getUsage`.
- `/tribes/[id]/usage` with `getTribeUsage(id, ...)`, titled "Token usage: {tribe}" from `getTribes()`, with all five dimensions and a "Back to tribe" link. The tribe page links to it with a "Token usage" button.
- `/account/usage`, "My token usage", with `getMyUsage` and the dimensions LLM, model, project, task, source, and card, and an "Account" link. The account page links to it with a "Token usage" button, and the user menu and the mobile menu with "My token usage".

### Company GitHub

`src/routes/admin/github/+page.svelte`. GET and PUT `/api/settings/github`. Admin only. Fields: host, organization, token. "Test connection" calls `POST /api/settings/github/test`, and shows "Signed in as LOGIN. Sees N repositories in ORG (M private)." on success or `message` on `ok: false` in the Bot account panel footer, next to Save. A non-empty `hint` shows in a `warning` `Notice` in the Bot account panel. The saved bot login shows as read-only text.

### Repository

`src/routes/settings/repository/+page.svelte`. GET and PUT `/api/projects/{id}/settings/repository`. Shows the company host and organization as text, the repository and base branch as fields, and whether the clone exists with its head SHA. The repository is a `SearchSelect` of `GET /api/settings/github/repos` with no free-text fallback, as on New project. When the list loads empty, the picker's error line says the company token sees no repositories in the organization, and an admin gets a "Check Company GitHub" link to `/admin/github`. New project shows the same line and link. "Sync" calls `POST /api/projects/{id}/sync`. It is enabled when `canEdit` is true or the caller is a Developer, Senior Developer, or Lead Developer, matching the server.

### Linked projects

`src/routes/settings/links/+page.svelte`. GET and PUT `/api/projects/{id}/settings/links`. Each link is a panel with the project name, its repository, a list of shared paths with "Add path" and a remove button per row, and "Remove link". "Add link" is a `SearchSelect` of projects in the same tribe that are not this project and not linked yet. Projects from other tribes that the user can see follow as disabled options with "Other tribe: name" as the detail, so a search explains why they cannot be picked. The hint names the tribe. The picker clears after a pick. When the projects do not load, a notice says so. The save note says links are one way. The page explains that linked context is read only.

### LLM APIs

`src/routes/admin/llm/+page.svelte` lists the company families from GET `/api/settings/llm` in a table: id, protocol, base URL, the first model ids, and whether a key is saved. Each row links to its edit page. "Add family" in the page header opens `/admin/llm/new`. A toolbar searches and filters the families (see Search and filters).

`src/lib/components/FamilyForm.svelte` is the form both pages use: id, protocol select with `anthropic` and `openai` only, base URL, model ids (a list plus "Add model" and a remove button per row), API key. Under each model id are its Thinking select (Provider default, On, Off, and Adaptive on anthropic only), a Budget number field shown for On on anthropic, an Effort select whose values follow the protocol, with Provider default first (RFC-079), and a History budget (KiB) number field on either protocol, empty for the 16 KiB default (RFC-081). They save in the family's `modelOptions`.

`src/routes/admin/llm/new/+page.svelte` creates a family. The id must be set, must not be `new`, and must not match an existing family. `src/routes/admin/llm/[family]/+page.svelte` edits one family. Its id is fixed after create, because the saved key is stored under the id. That page has Save, "Test connection", and "Delete family" behind an inline confirm.

The API has no per-family write. Create, Save, and Delete each GET the current list, change one entry, and PUT the whole families list. The other families are sent without `apiKey` or `clear`, so their saved keys stay.

"Test connection" calls `POST /api/settings/llm/{family}/test` and shows the result in the panel footer.

### Router

`src/routes/admin/router/+page.svelte`. GET and PUT `/api/settings/router`. Also GET `/api/settings/llm` so the selects can list saved model ids. The routes apply to every project.

A list of five rows: `plan`, `implement`, `implement_hard`, `review`, `triage`. Each row has a model `SearchSelect` and a fallback `SearchSelect`. Options are every model id from LLM APIs, each with its family id as the detail line, and the search matches either. Both pickers start with `None`, which saves an empty value. The review model picker also lists `different-family-from-author` first. A saved model that no family lists any more still shows, marked "Not in LLM APIs". From 640px the rows are three columns: class, model, fallback. Below that each row stacks and the picker labels show.

Four number inputs for the budgets. When the GET budgets are missing, show 3, 12, 200000, and 30.

Above the table, print the server sentence:

| `reviewIndependentPossible` | Text |
| --- | --- |
| `true` | Review can use a second family. |
| `false` | Only one family is configured, so review will record review.independent = false. |

The page does not calculate that flag itself.

The save result line includes "Applies to the next route decision."

### Conventions and skills

`src/routes/admin/conventions/+page.svelte` reads `GET /api/conventions` (RFC-055). The Rules tab groups rules under Global, then one table per stack by name: title, the first line of the body, category, version, and a Disabled badge. Each group has an "Add ... rule" link that opens `/admin/conventions/new` with the stack chosen. The Stacks tab (`?tab=stacks`) lists each stack with its markers and rule count. Rule and stack text renders as plain text.

`RuleForm.svelte` has "Applies to" (every project or one stack), the stack, category, title, the rule body as a monospace `Field type="textarea"`, and Disabled. `StackForm.svelte` has the name and the marker files, npm dependencies, and Python dependencies as comma separated fields. `/admin/conventions/[id]` picks the form from the id prefix (`stk_` is a stack), shows the version and "Last edited by" the email and time, saves with PUT, and has a delete panel behind an inline confirm. Delete says a stack still named by a rule or project is disabled instead.

`src/routes/admin/skills/+page.svelte` lists skills from the same GET (RFC-056): title, slug, "use when", version. `SkillForm.svelte` has slug, title, "Use when", the procedure as a monospace textarea, and Disabled. New and edit pages follow the rule pages. Field errors come from the API `fields`. A `403` sets the live message.

### Project conventions

`src/routes/settings/conventions/+page.svelte`. GET and PUT `/api/projects/{id}/settings/conventions`, and GET `/api/conventions` for the options. Detected stacks show as badges with a hint to Sync when there are none. "Added by hand" is a `MultiSearchSelect` of enabled stacks that are not detected. "Enabled skills" is a `MultiSearchSelect` of enabled skills, with the slug as the detail line. Both allow 30. A read-only table lists the rules that apply: title, Global or the stack name, category, version. The `SaveBar` note says saved rules apply from the next attempt. A disabled `MultiSearchSelect` shows its picks without remove buttons, or "None".

### Deploy and checks

`src/routes/settings/deploy/+page.svelte`. GET and PUT `/api/projects/{id}/settings/deploy`. These settings belong to the selected project (RFC-050). Any member can read them. Save is for a Lead Developer member, and a `403` shows in the live message.

Fields, in three fieldsets:

| Fieldset | Fields |
| --- | --- |
| Dev server | Dev URL, dev hosts (one per line), deploy command, rollback command, health URL, deploy token |
| Checks | Local check, dev check, allowed executables (one per line), allowed commands (one per line, `commandExecutables`, RFC-084), sandbox engine select (`podman`, `docker`), sandbox image |
| Board | Implementation cap (number, default 2), PRD path, RFC path, Solo approval (checkbox, default off, RFC-067) |

Static help text:

| Under | Text |
| --- | --- |
| Deploy and rollback command | Absolute path to a script outside the target repo and the data directory. Not group or world writable. |
| Dev hosts | Health URL and dev URL hosts must be listed here. |
| Sandbox image | Pin by digest, for example `name@sha256:...`. |
| Allowed commands | Programs the implementer may run with run_command, one bare name per line, such as go or npx. Each runs in the sandbox on a throwaway copy with no network, and never counts as a check. Shells and programs that run another program, such as bash, env, or xargs, are refused. Leave empty to turn run_command off. |
| Fieldset legend for dev server | Empty values can be saved. Deploy stops with deploy_config until they are set. |

## Styles

The product name is Marauder. `src/app.css` imports Tailwind CSS 4 and defines the theme: ground `#f4f1ea`, surface `#fffcf8`, sidebar `#efeae1`, ink `#1c1915`, line `#e6dfd4`, accent `#1f4d3a`. `tailwindcss` and `@tailwindcss/vite` are dev dependencies. There is no runtime CSS library and no React component kit.

Shared controls live in `src/lib/ui/`. One primary button on a page. `.danger` is for blocked status. `.error` is for a field error. Inputs and buttons are at least 44px tall. `:focus-visible` keeps a 2px ink outline. Do not remove outlines. Do not put layout in a `style` attribute.

The theme also defines the layout scale. Page padding is 24px, and 32px from 1024px. Every signed-in page body is full width at every breakpoint, so `Page` has no width prop. A page description stays at `max-w-2xl` for line length, and the login card stays centered. Field widths are `sm` 10rem, `md` 20rem, `lg` 32rem, and `full`. A `Field` textarea with `grow` uses the `autogrow` action, which sets its height from `scrollHeight` through the CSSOM on mount, input, and value change, and is `resize-none`. `Field`, `Select`, and `SecretField` take a `width` prop that defaults to `md`. Inside a two-column grid a field uses `full`. Every password or secret input is a `PasswordInput`: `Field type="password"` and `SecretField` both render it. It starts masked, and an eye button inside the right edge (a 44px `type="button"` with `aria-pressed` and "Show {label}" or "Hide {label}") reveals only what was typed. It masks again when the value is cleared or the input is disabled.

Every page body starts with `Page`: the `h1`, an optional description, an optional `meta` snippet beside the title, and an `actions` snippet aligned right on the same row. A hairline separates the header from the content. The top bar is a 48px strip with the menu button below 1024px, the current project, and the live status line. It holds no page actions.

Buttons size to their label and never stretch. `full` is only for the login submit. `Panel` has a title row with an optional `actions` snippet and an optional `footer` snippet. The footer is a right-aligned action bar. Admin settings pages (Company GitHub, Router, and an LLM family) put the primary Save in the last panel's footer, after "Test connection" where the page has one. Project settings pages and the user edit page end in `SaveBar`, a bar that sticks to the bottom of the viewport with the primary Save. `FieldGroup` groups related controls under one legend. `Row` is one entry in a bordered list, such as one model id, with a remove button. Tables use `Table` and the `th`, `tr`, and `td` classes from `src/lib/ui/control.ts`: an 11px uppercase header and 48px rows.

`SearchSelect` is the shared searchable select for long lists: the project switcher, Router models, and tribe member search. It is an ARIA combobox. Typing filters options by label and detail, arrow keys move, Enter picks, and Escape closes and restores the selected label. An optional `emptyLabel` adds a first option that clears the value. `onchange` fires only when a pick changes the value. Short fixed lists, such as protocol or role, keep the native `Select`.

`MultiSearchSelect` picks several values from a long list, such as requirement reviewers. It wraps `SearchSelect`: a pick adds a chip and clears the input, and picked values leave the option list. Each chip has a "Remove {label}" button. An optional `max` hides the input at the limit. `disabled` shows the chips without remove buttons.

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
| `CardActions` with `retry_review` | "Retry AI review" renders only when listed, and a click POSTs `retry-review` |
| `CardActions` on a blocked card with `unblockTo: null` | No Unblock button. The Intake sentence renders |
| `CardActions` Request changes | The menu item opens the reason form. A `422` shows `fields.reason` under it. A later submit POSTs `{ reason }` and closes the form |
| `CardNotes` | No notes render nothing. The newest review note renders its bold and code as elements, with "AI review" and "· attempt 2". Earlier notes wait behind "Show 1 earlier note", and a local check note stays plain text in a `pre`. An approver note has no attempt number. An instructions note reads "Instructions" and renders markdown, and a blank one reads "Instructions cleared". Current instructions and the newest other note are both open, and older notes wait behind the button |
| `CardActions` Run | Run opens the instructions box prefilled with `instructions`. Unchanged text POSTs `{}`. A `422` shows `fields.instructions`. Changed text POSTs `{ instructions }` and closes the form |
| `CardInstructions` | No instructions show the empty sentence. Saved instructions render markdown. Edit prefills the box, a `422` shows `fields.instructions`, and Save PUTs `{ instructions }` and shows the returned text |
| `api` run body | `runCard(id)` sends `{}`, `runCard(id, '')` sends `{ instructions: '' }`, and `setInstructions` PUTs to the card's `instructions` route |
| `CardChanges` | Lists files with counts and "binary". A file button opens its lines, with the deleted line on red and the added line on green, and the compare link points at `compareUrl`. A cut diff shows the warning. A missing commit shows "Commit {short} is not in the project clone yet." |
| `IntakeActions` | Ingest and Add requirement render, and no accept button. `toLines` drops blanks, markers, and non-item lines when a list exists, and keeps every line without one. `fromLines` loads saved checks as a `- ` list that saves back unchanged, and a list item added later keeps them |
| `markdown` | `<script>` stays text. `javascript:`, `data:`, relative, and non-GitHub links read as `label (url)`, and a GitHub link parses. An unclosed fence runs to the end. Nested emphasis parses; `**open`, `snake_case_name`, and `2 * 3 * 4` stay text, and so do `#123` and `####### x`. Ordered lists keep `start`, one nested level holds deeper items, and a marker switch starts a new list. Rules, hard breaks, CRLF, and NUL are handled. `plainText` drops markers. 20k `*` or `[` parse within a second. `listItems` keeps only the items of a pasted definition of done, joins wrapped items, keeps nested items, skips fenced code, does not join a label or heading onto an item, and is null without a list || `Markdown` | No `script` or `img` element renders. A GitHub link has `target="_blank"` and `rel="noreferrer noopener"`, and a `javascript:` link has no `<a>`. `# x` is an `h3`. A `pre` keeps its text. `a **b** c` reads "a b c" |
| `RequirementForm` | Statement has 8 rows and Acceptance 6, both growing. Preview shows the formatted heading and bold text, Write brings the textarea back, acceptance previews as a checklist of formatted lines with the list markers dropped and its error kept, a pasted document previews only its list items and counts the lines left out, and an empty statement or acceptance has nothing to preview |
| `requirements` mentions | `mentions` keeps `@tickets` and `(@db)`, drops `a@b.co`, `@scope/pkg`, `@pytest.fixture`, `@Override`, and mentions in backticks and fences, keeps first-use order without repeats, and allows a trailing `.`. `mentionAt` finds the `@` and query before the caret, and is null after a space or inside an email |
| `RequirementForm` mentions | Typing `@ti` opens a listbox with `tickets`, ArrowDown moves `aria-activedescendant`, Enter inserts `@tickets `, and Escape closes the list. With no server the list says so. Chips show a known server, a disabled one with its warning, and an unknown slug with its warning |
| Tribe selector | `/choose-tribe` lists two tribes with their project counts. Picking one stores the tribe and its first project and loads `/`; a tribe with no project stores an empty project. With no tribes a member reads "Ask an admin", and an admin also gets "Create a tribe" |
| Layout tribe scope | The switcher lists only the selected tribe's projects, "Switch tribe" links to `/choose-tribe`, a stored project from another tribe is replaced by the tribe's first project, and a stored tribe the API no longer returns sends the browser to `/choose-tribe` |
| Projects by tribe | An admin with the active tribe `tribe_a` sees only `tribe_a`'s projects and no Tribe filter. With no active tribe the page links to `/choose-tribe`, and an empty tribe says "No projects in this tribe yet." New project starts its tribe select on the active tribe |
| Layout load | With a bearer and no tribe, `/` and `/intake` redirect to `/choose-tribe`, `/admin/users` and `/tribes/new` do not, and no bearer still redirects to `/login`. Signing in goes to `/choose-tribe` |
| Intake pages mentions | `/intake/[id]` shows a parsed row's stored mentions, and `/intake/new` passes `mcpServers` to the form so typing `@` lists them |
| `RequirementForm` refine | Without `refine` there are no icons. Each field has its own 44px icon, and opening one does not open the other. Apply on the statement changes only the statement, and Apply on acceptance changes only the acceptance lines. An edit after asking is flagged, Discard keeps the text, and Preview hides the icons |
| `RefinePanel` | Refine sends the field, the trimmed draft, and the guidance. Deltas appear in order in the live region, done shows the comparison, and Apply calls back with the text. Stop aborts the signal and changes nothing. An error event or a rejected request shows the message and Try again asks again. A blank statement sends nothing. `reset` empties the text and `tool` shows "Looking up tickets / lookup" until the next delta (RFC-095) |
| Tribe MCP pages | The list shows each server with its tool count and a Disabled badge. The form posts slug, transport, URL or image and argv lines, secrets, and the allowlist; a saved secret shows only its name; Discover lists tools with Allow, Update schema, and Name not allowed; a failed discovery shows its message (RFC-092) |
| MCP catalog | Featured and built-in tiles show with "Token needed", "No sign-in", and "Built-in" while the registry is still "Searching the MCP Registry…"; the registry rows then show No network, an unsupported entry's reason with no Set up link, and Load more with the cursor. Typing shows the local matches, aborts the previous registry request, and a registry notice keeps the featured tiles (RFC-099, RFC-100). Postgres from the catalog fills the slug, port, and fixed tools, paints `config.readOnlyLogin`, and posts `config`, `PASSWORD`, and the three tools. A registry entry fills its URL and header names with "Required." A stdio entry's image is pulled and pinned. A new server discovers its tools once without a click. A database server is listed as `postgres · host:port/database` (RFC-096 to RFC-098) |
| `RefineCompare` | The statement shows `<ins>` and `<del>`, acceptance lines are marked unchanged, removed, and added with counts, Before and After render each version, Apply and Discard call back, a stale field is flagged, and unchanged text says so |
| `api` `streamRefine` | Events split across chunks parse in order, with the bearer and the project path. A `409` before the stream throws `ApiError` and sends no event. An abort rejects with `AbortError` |
| `api` `streamTrace` | `trace`, `turn`, `delta` split across chunks, `reset`, `turn_end`, and `done` parse in order from the card's `trace/stream` path with `since` and the bearer; a ping comment is skipped. A `429` throws `ApiError`. `parseEvents` keeps the event name when the data has its own `kind` (RFC-088) |
| `textdiff` | A word diff rebuilds both sides, including newlines; identical and empty texts; a line diff marks added and removed lines; a very large change still rebuilds both sides |
| Edit page refine | Refining acceptance and Apply do not save, Save PUTs the new acceptance with the statement unchanged and says it needs acceptance again, and a PRD row has no refine icons |
| `ReviewersPanel` | `canSetSelfReview` is true for a Lead Developer or an admin only. A row with `canSetSelfReview` shows a checked Self review box and Save reviewers. Otherwise a flagged row shows the read-only "Self review: on" line, and an unflagged row says nothing about Self review (RFC-070). A lead's row shows the AI review mode with the saved option pressed, and choosing the other one enables Save reviewers. Others see the read-only line only for a `manual` row (RFC-072) |
| Reviewers before GitHub | `reviewerChoices` labels a member without a login "No verified GitHub login". `/intake/new` sends an unverified member picked as a reviewer. `ReviewersPanel` shows the hint. A `reviewers_requested` row lists the `waiting` reviewers (RFC-103) |
| `intake` filters | Search matches id, statement, heading, source path, and acceptance lines, ignoring case. A markdown statement or acceptance line also matches by its `plainText` words. Status and source filters narrow the list. Only unaccepted rows are offered for acceptance. The status defaults to "Needs acceptance" while any row is unaccepted |
| `filter` | `matchText` ignores case and a blank query matches. `withParams` sets and removes parameters. `pickParam` falls back on an unknown value |
| `board` filters | `ownerChoices` lists each owner once, the signed-in user first in any email case, then by name, then `none` when a card has no owner. `parseOwners` reads the comma list and maps `mine`. Selected owners keep cards of any of them, and `none` keeps unowned cards. Blocked only keeps blocked cards. Search matches title, id, status and its label, requirement id, and the owner's and reviewers' name, login, and email |
| `OwnerFilter` | Clicking an avatar adds its owner and clicking again removes it. Owners past the limit are in the "+n" checklist |
| `status` labels | Every card status has a label in words. An unknown code reads in sentence case. Reasons such as `owner_github` and `attempt_cap` have labels, and free text is kept as is |
| `usage` lib | Groups by LLM, model, project, user, and task, with "Unknown family" and "No owner". Filters use `-` for a missing value. Options count the rows the other filters keep and keep a chosen value with none. Groups sort by any column either way, ties by name. Daily totals fill every day of the range. The summary's cache share is cached over all input read. `rangeWindow` ends today in UTC |
| Token usage page | With a mocked `getUsage` and URL: the 30-day range loads, the totals, a hidden day table, and models by total render with `aria-sort`. `?llm=-` narrows every total and the Model choices, and picking one sets `model`. `?by=user` regroups and Group by sets `by`. `?sort=name&dir=desc` reverses names, and header buttons set `sort` and `dir`. 7 days sets `range`. An empty range shows the empty state |
| `usage` source and card | Source groups Card and Intake; Card groups by id with refines as "Intake" and falls back to the id without a title. `source=intake` and `card=-` keep only refines, and card options list each card once, largest first (RFC-091) |
| Admin usage by card | `?by=card` lists Ana card, Intake, and Loose card with the card id as each title, Group by offers Source and Card, `?source=intake` narrows the total to the refine, and Clear filters drops `source` |
| `usage` dimension choices | `dimensionOptionsFor` keeps the allowed dimensions in the usual order |
| Tribe usage page | It reads `getTribeUsage` with the route's tribe id and 30 days, titles the page and the shell "Token usage: Gadget tribe", links back to the tribe, and offers the User filter |
| Account usage page | It reads `getMyUsage`, has no User filter, and Group by offers LLM, Model, Project, Task, Source, and Card. `?by=user&user=...` is ignored: models group and every row counts |
| User menu | Opening the menu shows "My token usage" linking to `/account/usage` for a member who is not an admin |
| Nav admin items | Token usage is the first Admin link for an admin and is absent for a member who is not one |
| `people` display name | `displayName` is the name, then the login, then the email. `describePerson` adds the login or email in parentheses. User search matches the name |
| `people` filters | Role, access, and email search narrow users, ignoring case. Each access option counts its users |
| `conventions` filters | Rules filter by text, category, Global, or one stack. A stack is found by a marker file |
| `trace` filters | Events filter by kind, attempt, payload text, and the row's words. Kind and attempt options come from the events present, labelled "Status change (1)" and "Attempt 1 (1)" |
| `trace` timeline | `buildTimeline` splits the implement and review blocks of one attempt with milestones between, and marks a later run as continued. `describe` covers each kind and an unknown kind, an `mcp_` tool row ("MCP: tickets / lookup"), `mcp_context` with a failed server, with a database server, with scope `none` and with `missing` slugs, and `mcp_changed`. `cleanText` strips NUL bytes, reports the byte size, and cuts at the cap |
| `TraceRow` | A row opens to its details and the raw JSON. An error row is red, a long message offers Show all, and binary output stays hidden until Show anyway |
| `TraceBlock` | A clean attempt starts closed with its outcome, commit, and duration, and opens to its rows. A block with an error starts open. A review block shows the verdict and "Not independent". An open block that showed every row stays open and shows rows that arrive later |
| `trace` live | `mergeEvents` keeps one row per id, ordered by `ts` then `seq`. Tool rows show their `args` ("src/x.ts · 3 lines", "exit 0 · npm test", a commit's first line), and a model row shows its reply text (RFC-074) |
| `pager` follow | When every entry was shown, `follow` grows the limit to the new total. When some were hidden, the limit stays |
| Trace page stream | With fake timers and a mocked `streamTrace`: a live trace opens the stream with `since` set to the newest `ts`, merges `trace` frames, shows the reply as `delta` frames arrive with "Waiting for the model…" before them and "The model is writing…" in Now running, drops it on `reset`, replaces it with the stored model call, and on `done` announces "Attempt 1 ended: Checks passed" with no poll. A trace that is not live is read once, opens no stream, and shows no Now running strip. When the stream fails the page polls every 3 seconds and opens the stream again after 5 seconds; a hidden tab closes it; `reconnect` and `resync` open it again (RFC-088) |
| `terminal` lines | Each kind has its head, text, and tone. A failed or refused tool is red, a check is green only on `exit 0`, ANSI codes are stripped, an unknown kind shows JSON, an attempt line starts each attempt, and `clipLines` folds past 200 lines (RFC-078) |
| `TerminalView` | Heads and output render in the log. Output past 200 lines shows "Show all N lines". Past 300 entries, "Show earlier events" pages in the rest. Scrolling up shows "Jump to latest", which scrolls to the bottom and hides again. A pending reply is the last entry with its `writing…` head, "Waiting for the model…" until text arrives, and a caret, and the log is `aria-busy` while it shows |
| Trace page terminal view | With `?view=terminal` the log renders, the Kind filter applies, and Terminal is pressed. A Kind filter other than Model call hides the reply being written. While the stream is down the terminal view polls every second |
| `llm` model options | `toDraft`, `fromDraft`, and `keepKey` keep `modelOptions`. `fromDraft` drops a model's empty settings, the settings of a removed model, and a budget unless thinking is `enabled` on anthropic (RFC-079) |
| `FamilyForm` model options | Adaptive is offered on anthropic only, the budget field shows only for On on anthropic, and the effort list follows the protocol |
| Deploy settings allowed commands | Allowed commands load one per line, save as `commandExecutables` with blank lines dropped, and a refused name shows its field error |
| `FamilyForm` history budget | Every model row has a History budget field on either protocol. `toDraft` and `fromDraft` round-trip `historyBudgetKiB` and drop an empty one |
| `SecretField` saved with an empty input | The emitted body omits the secret field |
| `api` on `401` | The bearer is cleared |
| `api` AI review mode | `addRequirement` sends `aiReviewMode` in the POST body, as the New requirement page does, and `setAIReviewMode` PUTs `{ aiReviewMode }` to `requirements/{id}/ai-review-mode` in the selected project (RFC-072) |
| Built `build/index.html` | Contains a CSP meta tag with `script-src 'self'` and a hash, and no `unsafe-inline` |
