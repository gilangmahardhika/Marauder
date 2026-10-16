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
  src/routes/tribes/[id]/overview/+page.svelte
  src/routes/overview/+page.svelte
  src/routes/account/usage/+page.svelte
  src/routes/admin/users/+page.svelte
  src/routes/admin/users/new/+page.svelte
  src/routes/admin/users/[id]/+page.svelte
  src/routes/admin/github/+page.svelte
  src/routes/admin/llm/+page.svelte
  src/routes/admin/llm/new/+page.svelte
  src/routes/admin/llm/[family]/+page.svelte
  src/routes/admin/router/+page.svelte
  src/routes/admin/agent/+page.svelte
  src/routes/admin/runners/+page.svelte
  src/routes/admin/runners/new/+page.svelte
  src/routes/admin/runners/guide/+page.svelte
  src/routes/admin/conventions/+page.svelte
  src/routes/admin/conventions/new/+page.svelte
  src/routes/admin/conventions/[id]/+page.svelte
  src/routes/admin/skills/+page.svelte
  src/routes/admin/skills/new/+page.svelte
  src/routes/admin/skills/[id]/+page.svelte
  src/routes/tribes/+page.svelte
  src/routes/tribes/new/+page.svelte
  src/routes/tribes/[id]/+page.svelte
  src/routes/tribes/[id]/conventions/+page.svelte
  src/routes/projects/+page.svelte
  src/routes/projects/new/+page.svelte
  src/routes/settings/repository/+page.svelte
  src/routes/settings/links/+page.svelte
  src/routes/settings/deploy/+page.svelte
  src/routes/settings/workflow/+page.svelte
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
  src/lib/ui/ChipSelect.svelte
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
  src/lib/toast.svelte.ts
  src/lib/forms.ts
  src/lib/ui/Toaster.svelte
  src/lib/ui/FormAlert.svelte
  src/lib/ui/Spinner.svelte
  src/lib/components/SecretField.svelte
  src/lib/components/CardActions.svelte
  src/lib/components/CardChanges.svelte
  src/lib/components/CardPullRequest.svelte
  src/lib/components/CardNotes.svelte
  src/lib/components/CardInstructions.svelte
  src/lib/components/CardContext.svelte
  src/lib/components/CardQuestions.svelte
  src/lib/components/QuestionModeField.svelte
  src/lib/components/CardPlan.svelte
  src/lib/components/PlanModeField.svelte
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
  src/lib/diff.ts
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
  src/lib/components/CardPullRequest.test.ts
  src/lib/diff.test.ts
  src/routes/cards/[id]/page.test.ts
  src/lib/components/CardNotes.test.ts
  src/lib/components/CardInstructions.test.ts
  src/lib/components/CardContext.test.ts
  src/lib/components/CardQuestions.test.ts
  src/lib/components/QuestionModeField.test.ts
  src/lib/components/CardPlan.test.ts
  src/lib/components/PlanModeField.test.ts
  src/routes/tribes/[id]/conventions/page.test.ts
  src/routes/settings/conventions/page.test.ts
  src/lib/components/TraceRow.test.ts
  src/lib/components/TraceBlock.test.ts
  src/lib/components/TerminalView.test.ts
  src/lib/pager.svelte.test.ts
  src/routes/cards/[id]/trace/page.test.ts
  src/routes/settings/deploy/page.test.ts
  src/routes/settings/workflow/page.test.ts
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
  src/routes/overview/page.test.ts
  src/routes/tribes/[id]/overview/page.test.ts
  src/lib/components/SecretField.test.ts
  src/lib/ui/SearchSelect.test.ts
  src/lib/ui/MultiSearchSelect.test.ts
  src/lib/ui/ChipSelect.test.ts
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
| `/account` | Name, password, the caller's GitHub settings, and browser notifications |
| `/account/usage` | My token usage (RFC-090) |
| `/notifications` | The caller's newest 50 notifications, All or Unread (RFC-113) |
| `/admin/usage` | Token usage across every project (RFC-089) |
| `/admin/users` | Admin user list |
| `/admin/users/new` | Create a user |
| `/admin/users/[id]` | Edit role, admin, disabled, and an optional password |
| `/admin/github` | Company GitHub account |
| `/admin/llm` | LLM families for the company |
| `/admin/llm/new` | Create an LLM family |
| `/admin/llm/[family]` | Edit, test, or delete one LLM family |
| `/admin/router` | Router for the company |
| `/admin/agent` | The implement agent for the company: built-in or Claude Code, with the Claude Code options (RFC-120) |
| `/admin/runners` | Remote runners with their status, and where implement jobs run (RFC-106, RFC-107) |
| `/admin/runners/new` | Mint a registration token and show the install command once (RFC-106) |
| `/admin/runners/guide` | How to register, update, pause, and unregister a runner (RFC-109) |
| `/admin/conventions` | Coding rules by Global and by stack, and a Stacks tab |
| `/admin/conventions/new` | Add a rule, or a stack with `?kind=stack` |
| `/admin/conventions/[id]` | Edit or delete one rule or stack |
| `/admin/skills` | Integration skills |
| `/admin/skills/new` | Add a skill |
| `/admin/skills/[id]` | Edit or delete one skill |
| `/tribes` | Tribes |
| `/tribes/new` | Create a tribe |
| `/tribes/[id]` | Tribe members |
| `/tribes/[id]/overview` | Tribe WIP and range overview (RFC-127) |
| `/tribes/[id]/usage` | Token usage of the tribe's projects (RFC-090) |
| `/tribes/[id]/mcp` | The tribe's MCP servers (RFC-092) |
| `/tribes/[id]/mcp/catalog` | Add a server from the catalog (RFC-096) |
| `/tribes/[id]/mcp/new` | Add a server by hand |
| `/tribes/[id]/mcp/[serverId]` | Edit one server |
| `/tribes/[id]/conventions` | The tribe's stacks, skills, and rules switched off (RFC-104) |
| `/projects` | Projects of the active tribe |
| `/projects/new` | Create a project |
| `/overview` | Project WIP and range overview (RFC-127) |
| `/settings/repository` | Project repository, base branch, PRD and RFC paths, and Sync |
| `/settings/links` | Linked projects and their shared paths |
| `/settings/deploy` | Deploy and checks for the project |
| `/settings/workflow` | Implementation cap and Solo approval for the project (RFC-126) |
| `/settings/conventions` | The project's stacks, skills, and rules over its tribe (RFC-104) |

`+layout.svelte` renders the Marauder shell on every page except `/login` and `/choose-tribe`. The product name in the shell is Marauder. The sidebar groups run from daily work outward, in this order: Workspace (Board, Intake), "Project · {project name}" (Overview, Deploy, Workflow, Repository, Conventions, Linked projects), "Tribe · {tribe name}" (Projects, Overview, Members, MCP servers, Conventions), Admin (Users, Tribes, Runners, Token usage), and Company setup (Company GitHub, LLM APIs, Router, Conventions, Skills) (REQ-091). The Tribe links are `/projects`, `/tribes/{tribe}/overview`, `/tribes/{tribe}`, `/tribes/{tribe}/mcp`, and `/tribes/{tribe}/conventions`; Overview, Members, and MCP servers show only when `canManageTribe(tribe)`, and Members is current only on `/tribes/{tribe}` itself. With no stored tribe the group is "Tribe" with Projects only. The Project group shows only when a project is selected. The mobile menu shows the same group headings. LLM APIs, Router, Conventions, and Skills are company settings (RFC-052, RFC-055), so they do not wait for a selected project. The Admin and Company setup groups render when `GET /api/me` has `admin: true`. Those links are not a card permission check. A `403` or `404` from the API is still shown.

The sidebar shows the selected tribe's name with a "Switch tribe" link to `/choose-tribe`, then a project `SearchSelect` bound to `orchestrator.project`. Options come from `GET /api/projects`, only those whose `tribeId` is the selected tribe, each with its repository name as the detail line, and the search matches name or repository. Choosing a different project stores the id and reloads the board. A stored project outside the selected tribe is replaced by the tribe's first project, or cleared when the tribe has none. A stored tribe that `GET /api/tribes` no longer returns is cleared and the browser goes to `/choose-tribe`. The signed-in user menu shows the caller's name (or email when unnamed) and initials from it, links to Account and My token usage, and signs out with `DELETE /api/session`. Below 1024px the same links open from a menu button. The top bar shows the current tribe and project as `Tribe / Project`, and the notification bell. With no project in the tribe, the board shows "This tribe has no projects yet." with a link to Projects. The page title and page actions sit in the body, in the `Page` header.

A card link with `?project={id}` (from a notification) that names a project other than the stored one makes `+layout.ts` read `GET /api/projects`, then store that project and its tribe before the page loads. An id the caller cannot see is left alone, and the card page shows its `404`.

### Live stream

`src/lib/stream.svelte.ts` exports `live`, one stream per tab (RFC-113). `+layout.svelte` calls `live.start(projectId)` when the shell shows and a bearer is stored, so it restarts when the project changes, and `live.wake()` when the tab becomes visible. `openStream` in `api.ts` reads `GET /api/stream` through `fetch` with the bearer header and `parseEvents`. `live.connected` turns true on the `ready` frame. A dropped stream reconnects after 1 second, doubling up to 30; `wake` reconnects at once. After a reconnect, `live` hands handlers a `resync` before the next frames. Pages subscribe with `live.on(handler)`, which returns the unsubscribe.

`src/lib/notifications.svelte.ts` holds `inbox` (the newest rows, the unread count, and a load failure), shared by the bell and `/notifications`, so reading a row on either lowers the bell's count. `NotificationRow.svelte` renders one row as a link to the row's `url` when it is a path starting with a single `/`, else to `/`. It shows a round icon for the kind (review and question in sky; ship, approved, and done in green; changes requested and a review with no verdict in amber; blocked and owner GitHub in red; others neutral), the title over the body, and the relative time ("Just now", "5m ago", "3h ago", "2d ago", then the date after a week) with the full time as its `title`. Unread rows are tinted and bold with a dot and a screen-reader "Unread". Following a row marks it read; a row in another project does a full page load so the project switches.

`NotificationBell.svelte` sits at the right end of the top bar. The 44px button's label is "Notifications, N unread", with a badge showing the count (99+ past 99). It opens a panel, a group named "Notifications", that loads `GET /api/notifications` each time it opens and on `resync`. The header shows "N new" while any are unread and "Mark all read", which posts `{ "all": true }`. The list, named "Latest notifications", shows the newest 8 rows. The footer link "Open all notifications" goes to `/notifications` and closes the panel. A `notification` frame sets the count, adds the row to the list, and shows an info toast "title: body" with an "Open" link to the row's `url`. Escape (focus returns to the bell) and a click outside close the panel.

`/notifications` lists the newest 50 rows from `GET /api/notifications`, grouped under "Today", "Yesterday", or the date. A `Segmented` "Show" switches between All and Unread (with the unread count); Unread reads `?unread=1`, and a row read there stays listed until the filter changes. "Mark all read" is a secondary button, disabled with nothing unread. An empty list says "No notifications yet" for All and "You're all caught up" for Unread.

Create and edit forms are their own routes. They are not dialogs.

Unknown paths use SvelteKit's default error page with a link to `/`.

## API wrapper

`src/lib/api.ts` exports one function per route in 0002. Each function:

1. Sends `Authorization: Bearer` from `session.bearer()`.
2. Sends `Content-Type: application/json` on PUT and POST bodies.
3. Parses `error` on non-2xx and throws `ApiError` with `status`, `code`, `message`, and `fields`.
4. On `401`, clears the stored bearer and sends the browser to `/login`.
5. Does not log the request body. Secret fields must not reach `console`.

Base URL is `http://127.0.0.1:8787` unless the build is given the cloud API origin. A failed `fetch` throws a network error with code `network`, which `formFailure` shows as "Can't reach the API at http://127.0.0.1:8787. Check that it is running.", using the configured origin in cloud mode. The page does not invent cards or settings in that case. Project routes include the selected project id. A `404` on those routes shows "That project is not available." in the page's error panel.

## Markup rules

Svelte text `{value}` is the way API strings hit the page. `{@html}` is forbidden. A pull request URL becomes an `<a href>` only when it starts with `https://github.com/` or `http://127.0.0.1`. Any other value is text.

A requirement statement is markdown, and so is a card note with source `review` or `approver`. `$lib/markdown` parses it into a token tree and `Markdown.svelte` renders that tree as elements with text nodes, never as an HTML string. The subset is paragraphs, ATX headings (rendered from `h3` down), bullet and ordered lists with one nested level, fenced code, blockquotes, rules, inline code, bold, italic, links, autolinks, and hard breaks. Tables, images, raw HTML, and setext headings stay literal text. A link becomes `<a target="_blank" rel="noreferrer noopener">` only under the same URL rule, `https://github.com/`, or `http://127.0.0.1` with an optional numeric port followed by `/`, `?`, `#`, or the end; any other link, including `http://127.0.0.1:1@host`, reads as `label (url)`. Each acceptance line is inline markdown: `Markdown` with `inline` renders inline code, bold, italic, links, and autolinks, and `AcceptanceList` shows the lines as a checklist. `plainText` gives the one-line excerpt for clamped cells.

Buttons that submit a form use `type="submit"`. Buttons that do not submit use `type="button"`. Every input has a `<label for>`.

### Feedback (REQ-096)

Feedback appears where the person acted. The top bar has no status line.

| What happened | Where it shows |
| --- | --- |
| A page is loading | `Page` with `loading`: a spinner row "Loading {thing}" (`role="status"`) in the body |
| A page failed to load | `Page` with `error`: a danger panel with the message and "Try again" (`onretry`) |
| A request is in flight | The button that started it has `loading`: a spinner, disabled, `aria-busy`. The label does not change |
| A form submit failed | `FormAlert` at the top of the form, field errors under their inputs, focus on the first invalid input, else on the alert |
| An action with no form failed (Run, Deploy, Remove, a 409) | An error toast, or an info toast for "Card changed. Reloaded." |
| Something succeeded | A success toast, such as "Name saved" or "Queued 3 cards." |
| A notification arrived | An info toast "title: body" with an "Open" link |

`src/lib/toast.svelte.ts` holds the toasts: `toast.success(text)`, `toast.error(text)`, `toast.info(text, { href, label })`, `toast.dismiss(id)`, and `toast.items`. Success and info toasts close after 5 seconds and errors after 8; hovering or focusing a toast pauses its timer. At most 3 show, the oldest leaves first, and the same text and tone as the newest toast only restarts its timer. `Toaster.svelte`, rendered once by `+layout.svelte` on every page including `/login`, keeps two regions in the page: `role="status"` for success and info and `role="alert"` for errors. Each toast has a tone icon, the text, an optional link, and a 44px "Dismiss" button. It slides in from below, without motion when `prefers-reduced-motion` is set. It sits bottom right, and full width at the bottom below 640px.

Field errors (`Field`, `Select`, `SearchSelect`, `MultiSearchSelect`, `MentionTextarea`, `SecretField`, `Checkbox`, `AIReviewModeField`) render in one order: label, hint, control, error. The error is `<p id="{id}-error">` with the alert icon in red. The control gets `aria-invalid="true"`, a red border and focus ring, and `aria-describedby` with the hint id and the error id. `FormAlert.svelte` is a red panel with the alert icon, `role="alert"`, and `tabindex="-1"`. It shows the general message and lists any field error whose key no control on the form shows, so no message from the API is lost. `src/lib/forms.ts` has `formFailure(error, shown)`, which returns `{ message, fields, extra }` (the message is "Check the fields marked below." when a shown field is blamed) and turns a network failure into "Can't reach the API at {origin}. Check that it is running."; `failureText(error)`, the text for a toast or page error, which uses the field errors when the request carried any, since a row action has no input to show them; and `focusInvalid(form)`, which after `tick()` focuses the first `[aria-invalid="true"]` control, else the form's alert.

A status line inside a panel, such as the Refine stream or the MCP Registry search, is a `role="status"` paragraph in that panel.

### Buttons

`Button` takes `icon` (a leading `IconName`), `trailing` (an icon after the label, for a chevron or external link), `loading`, `aria-label`, and `title`. Icons are `aria-hidden`, so they never change the accessible name. Primary and secondary have a hairline shadow; every variant eases its colours and moves 1px down while pressed; disabled shows `cursor-not-allowed` at half opacity. Danger is red text on an outline that fills pale red on hover. The primary hover is `--color-accent-strong`.

Only action buttons carry an icon. Cancel, Show more, Expand all, and filter toggles stay text only.

| Action | Icon |
| --- | --- |
| Save, Save X, Change password | `save` |
| Create, Add, New | `plus` |
| Delete, Remove | `trash` |
| Run, Run ready cards | `play` |
| Approve | `check` |
| Request changes | `message` |
| Deploy approved SHA | `upload` |
| Squash and merge | `git` |
| Unblock | `unlock` |
| Won't fix, Close as won't fix | `ban` |
| Reopen, Retry AI review, Sync, Try again | `refresh` |
| Test connection | `zap` |
| Discover tools | `search` |
| Copy | `copy` |
| Refine with AI | `sparkles` |
| Refine (send), Stop | `arrow-up`, `stop` |
| Turn on | `bell` |
| Turn off | `bell-off` |
| Send a test | `send` |
| Edit | `pencil` |
| Sign in | `log-in` |
| Sign out | `log-out` |
| Back, "All X" links | `arrow-left` |
| Open pull request | `external`, trailing |
| Clear filters, Clear search | `close` |

### Scrollbars

Scrollbars are thin and use the theme: `scrollbar-color` is `--color-line-strong` on a transparent track, and WebKit gets a 10px bar whose rounded thumb sits 2px in from the edge and turns `--color-muted` on hover. `.scrollbar-dark` is the same shape in stone grey for the dark terminal view. `html` has `scrollbar-gutter: stable`.

## Search and filters

A list page that can grow long has a `Toolbar` above it, and every page filters the same way. Filtering runs in the browser on the list the page already loaded. The API takes no filter parameters.

- `SearchBox` is a search input with a screen-reader-only label. It writes `q` to the URL 200 ms after the last keystroke, and Enter does not submit a surrounding form. `matchText` in `src/lib/filter.ts` matches when any searched field contains the query, ignoring case. A blank query matches everything.
- `Segmented` is a button group for a few fixed choices, with `aria-pressed` and an optional count on each button. `FilterSelect` is a compact native select for longer or data-driven choices. Both have screen-reader-only labels and are 44px tall. `ChipSelect` is a select-only combobox: a 28px chip inside a 44px `<button role="combobox">` with an `aria-label`, and a `rounded-xl` popover `listbox` under it. The chip shows the `display` text, an optional muted `prefix` and `icon`, and a chevron that turns over while open; it draws no outline, and focus tints it like hover. The list takes `options` (`value`, `label`, optional `detail`, `tag`, and `group`), shows options of one group under an uppercase header in a `role="group"`, marks the selected option with a check, and shows a `tag` as a small muted pill. ArrowUp and ArrowDown move, Home and End jump, a letter jumps to the next option starting with it, Enter or Space picks, Escape closes, and Tab or a click outside closes. Use it for compact toolbar choices; use `Select` or `FilterSelect` in forms and filters.
- Filter state lives in the URL, so a reload, Back, or a return from an edit page keeps it. `setParams` in `src/lib/params.ts` updates the URL with `replaceState`, `keepFocus`, and `noScroll`, and an empty value removes the parameter. `pickParam` ignores a value that is not a known option.
- A long list renders 50 rows, then `ShowMore`: "Showing x of y" and a "Show n more" button. `pager` in `src/lib/pager.svelte.ts` goes back to 50 rows when a filter changes, and not when a poll replaces the data.
- When filters hide every row, an `EmptyState` titled "No ... match" offers "Clear filters", which removes the page's filter parameters. When the list itself is empty, the page keeps its own empty state and shows no toolbar.

| Page | Search | Filters | Show more |
| --- | --- | --- | --- |
| Board | Card id, title, status, owner and reviewer names, emails, and logins, requirement ids | `owner` avatars (comma list of user ids, `none` for no owner), `blocked=1` Blocked only, `wontfix=1` Show won't fix | No |
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

`src/routes/login/+page.svelte`. Email and password fields, and a submit button "Sign in". Submit calls `POST /api/session` with `{ email, password }`. Success stores the bearer and `goto('/choose-tribe')`. An empty email or password shows "Enter your email" or "Enter your password" under the field and focuses the first empty one, without a request. A `401` shows `FormAlert` "Email or password is wrong.", marks neither field, clears the password, and focuses the alert. A `429` shows "Too many attempts. Wait one minute." in the alert, and a network failure the unreachable text. The submit button shows a spinner while the request runs. The password input is a `Field type="password"` with `autocomplete="current-password"`; its eye toggle shows or hides the typed password.

`src/routes/choose-tribe/+page.svelte` opens after every sign-in, even for a member of one tribe, without the sidebar. A decorative inline SVG (`TribeIllustration`, `aria-hidden`) of a requirements, code, CI, and deploy loop sits above the heading, drawn with the theme colors and the `highlight` token. It lists the tribes from `GET /api/tribes` (an admin gets every tribe) as buttons at least 44px tall, each with its name and "N projects" counted from `GET /api/projects`, and marks the stored tribe as current. Picking a tribe stores its id and the id of its first project, or an empty project when it has none, and loads `/`. With no tribe it says "You are not in a tribe yet. Ask an admin to add you.", and an admin also gets a "Create a tribe" link to `/tribes/new`. A ghost "Sign out" button ends the session. A failed load shows the API error in a danger panel with "Try again".

## Board

`src/routes/+page.svelte` calls `GET /api/cards` from `onMount` (or `$effect` on first run).

Render seven sections in this order: Waiting, Needs answer (group `question`, RFC-116), Running, Review, Shipping, Finished, Stopped. Place each card by `group`. A Needs answer tile has the amber stripe. Print the status as words from `statusLabel` in `src/lib/status.ts`, such as "Waiting for review" for `human_review`, with the raw status in `title`. Below it, the row shows the owner's initials avatar and display name, `name`, then `githubLogin`, then the email, or a dashed avatar and "No owner" (RFC-054, RFC-058). Every page that lists a person uses that order (`displayName` in `src/lib/people.ts`) and the `Avatar` component. When `reviewers` is not empty, the same line ends with up to three overlapping reviewer avatars and "+n" for the rest. Their names are in the `title` and `aria-label` ("Reviewers: ..."). The name clamps to one line. The tile is `src/lib/components/BoardCard.svelte`. Its body links to `/cards/{id}`.

Below the link, outside it, a ready card has a footer row (RFC-061). It shows a 44px "Run" button when `actions` includes `run`, a "Queued" pill when `queued` is true, or "Waiting on N cards" when `waitingOn` is not empty, with the ids in `title`. Run POSTs `/api/projects/{id}/cards/{card}/run`, swaps the returned card into the list, and sets the message to "Queued {title}." A `409` reloads the board with "Card changed. Reloaded." The card page shows Run through `CardActions`, where it first opens the instructions box (RFC-073).

The primary button reads "Run ready cards (N)", where N is `readyCount`. It is enabled when `canRun` is true and N is above zero. Its `title` says why when it is disabled: "All ready cards are queued", "No ready card can start", or "Your role cannot start cards". It POSTs `/api/projects/{id}/runs`, then reloads the list, and the message reads "Queued N cards." The page says "Queued" and never "Running" for these cards, because the worker loop is not built yet.

The board listens to the live stream (RFC-113, see Live stream). A `cards` frame merges into the list with `mergeCards` in `src/lib/board.ts`: a `full` frame replaces the list, otherwise each card replaces the one with its id or is added, and `removed` ids leave. A `board` frame sets `readyCount`, `running`, and `canRun`. Only while the stream is not connected, and `running` is true or any card is in the `running` or `shipping` group or in `agent_review`, an `$effect` starts a 3 second `setInterval` that calls `GET /api/cards` again. The effect cleanup clears the timer when the condition becomes false, when the stream connects, and when the page is destroyed.

Above the lanes, a toolbar searches the cards, filters them by owner, and has Blocked only (see Search and filters). The owner filter is a row of overlapping 36px avatars in 44px toggle buttons, one per distinct card owner (`ownerChoices` in `src/lib/board.ts`). The signed-in user comes first, matched by `owner.email` ignoring case (RFC-058). The others follow by display name. A dashed "No owner" circle comes last when a card has no owner. Each button has `aria-pressed`, the label "Cards owned by {name}", and the name and card count in `title`. Several owners can be on at once, and the board keeps cards owned by any of them. Past six owners, the rest sit behind a "+n" button that opens a checklist. It has a ring while one of its owners is selected. The selection is `?owner=` with user ids separated by commas. An old `?owner=mine` link maps to the signed-in user's id, or is dropped when that user owns no card. Clear filters also clears the owners. While a filter is on, each lane header counts "x of y". The poll keeps the filter.

The board hides cards in the `closed` group (`wont_fix`, RFC-112) and leaves them out of the page's card count and the owner filter. A "Show won't fix" checkbox after Blocked only sets `?wontfix=1`, which shows them in a "Won't fix" lane after Stopped. Clear filters also clears it.

Empty `cards`: one paragraph, "No cards yet", and a link "Ingest requirements" to `/intake`. No table of placeholder rows.

## Card

`src/routes/cards/[id]/+page.svelte` loads `GET /api/cards/{id}` using the `params.id` from the page. It loads the card again, without clearing the page, when a `full` `cards` frame arrives or a frame names its id.

The header shows the title and a badge with `statusLabel`, with the raw status in `title`. A line under the title shows the owner's avatar and display name ("No owner" when `null`), "Attempt {attemptsUsed} of {maxAttempts}", and `branch` in monospace when set. The header buttons are "Open pull request" (secondary) when `prUrl` passes the link rule, and "View trace" (secondary, with the terminal icon) (RFC-110).

From 1024px wide the page is two columns, a main column and a 340px sidebar `aside` labelled "Card details". Below 1024px the sidebar follows the main column. The `owner_github` notice sits above both. The main column holds, in order: Next step, Pull request, Changes, Notes, Instructions, and Context. The sidebar holds, in order: Details, People, and Intake.

Next step is always shown. When `actions` includes `accept_plan`, it renders `CardPlan` (RFC-129) instead of `CardActions` or `CardQuestions`. When `actions` includes `answer` and not `accept_plan`, it renders `CardQuestions` (RFC-114) instead of `CardActions`. Otherwise it renders `CardActions` when `actions` lists one of its controls or the card is blocked with `unblockTo` `null`. Otherwise it says "Queued. The card starts when the worker picks it up." when `queued` is true, or "No action is open for you right now."

Pull request (`CardPullRequest`, RFC-110): the panel title is "Pull request #N" with the number from `prUrl`, or "Pull request". The header links "Open on GitHub" when `prUrl` passes the link rule. With `pullRequest`, it shows the title as an `h3`, then `branch` → the diff's base, the short `sha` with the full value in `title`, and "Attempt N". The body renders with `Markdown`, headings from `h4`. A body over 1,200 characters is clamped behind "Show full description", a button with `aria-expanded`. A `null` `pullRequest` reads "No pull request yet. The title and description show here after the card publishes.", or, when `prUrl` is set, says the card published before the text was stored and its title and description are on GitHub.

The Instructions panel: when `actions` includes `instructions`, `CardInstructions` shows an Instructions panel (RFC-073) with the hint "Every later attempt reads these with the newest note, until you change them. An attempt that already started keeps the prompt it started with." It renders `instructions` with `Markdown`, or "No instructions. Attempts follow the requirements and the newest note." when it is `null`. The panel's Edit button opens a textarea "Instructions for later attempts", prefilled, growing, with the hint "Leave it empty to clear them. Markdown works.", and Save and Cancel buttons. Save PUTs `{ instructions }` to `/api/projects/{id}/cards/{card}/instructions`, replaces the card with the response, and closes the form. A `422` shows `fields.instructions` under the textarea. The "newest note" wording here means the newest other failure note; the pack also always carries the newest AI review note when one exists (RFC-131).

Questions (`CardQuestions`, RFC-114): the section has `id="questions"`, so a notification link with `#questions` scrolls to it. With open questions and the `answer` action it reads "The agent has a question" (or "The agent has N questions") and "Answer every question to queue the next attempt. It reads your answers." Each open question is a `fieldset` whose `legend` is the question as plain text, never markdown or HTML. `single` renders a radio group of its options, `multiple` a checkbox group, and with `allowOther` both add an "Other" choice that shows a text input "Your answer" (for `single`, picking Other clears the option and the input is the answer). `text` renders a growing textarea "Your answer". Under the choices a muted line reads "Agent suggests: {options and text}" and, when `why` is set, "Why: {why}"; nothing is preselected. Buttons: "Submit answers" (primary, with the send icon and a loading state) and "Let the agent decide" (secondary), which POSTs `{ useSuggestions: true }`. Submit checks locally that each question has an answer and shows "Answer this question." under the ones that do not, without a request. Submit POSTs `{ answers }` to `/api/projects/{id}/cards/{card}/answers`, replaces the card, and shows the toast "Answers sent. The next attempt is queued." A `422` shows `fields["answers.{n}"]` under question `n` and the rest in `FormAlert`. Answered questions show below under "Earlier questions", collapsed behind "Show N earlier questions": each lists the question, the answer, and a badge "You" (person, with the person's name), "Agent's suggestion" (suggestion), or "Auto" (auto). Without open questions the panel shows only the earlier questions, in the main column after Instructions, and only when there are any.

The answer mode (`QuestionModeField`, RFC-115) is the first row of Details. It is a `Segmented` control labelled "When the agent has a question" with "A person answers" (`manual`) and "Agent decides" (`auto`), and a hint under it: "The card waits and the owner is notified." or "The agent picks its own suggested answer and keeps going." With the `question_mode` action a change PUTs `{ questionMode }` to `.../cards/{id}/question-mode`, replaces the card, and shows a success toast; a failure shows an error toast and keeps the old value. Without the action the row reads "Questions: A person answers" or "Questions: Agent decides".

Plan mode (`PlanModeField`, RFC-128) follows: "Plan before coding" (`required`) and "Skip planning" (`off`). With `plan_mode` it PUTs `{ planMode }`. Without the action it reads "Plan: Plan before coding" or "Plan: Skip planning".

Plan (`CardPlan`, RFC-129, RFC-130): section `id="plan"`. Title and an ordered editable checklist (reorder, edit text/detail, add, remove). Primary "Accept plan". When `skip_plan` is listed, secondary "Skip plan". Below, a chat card like refine chat: message, model choices, stream, Apply revision into the checklist.

`CardNotes` shows a Notes panel when `notes` is not empty, with the hint "Newest first. The next attempt gets the newest AI review note and the newest other note, plus instructions." Each note shows its source as a badge (AI review, Approver, Local check, CI checks, Dev check, No commit, or Instructions, and the raw source when unknown), its local time, and "attempt N" when `attemptN` is above 0. A `review`, `approver`, or `instructions` note renders with `Markdown` and headings from `h4`, and a blank `instructions` note reads "Instructions cleared". Any other note is check output and renders with `TraceText`. The notes the next attempt reads are open: the newest `review` note when one exists, the newest note that is neither `instructions` nor `review`, and the newest `instructions` note when it is not blank (RFC-131). When none of those apply, the newest note is open. The rest wait behind "Show N earlier notes".

The other panels:

- People: the owner and each reviewer with avatar, display name, email, and "GitHub @login" or "No verified GitHub login" on its own line, labelled Owner or Reviewer. With no reviewers, a line says reviewers come from the card's requirements and links "Edit reviewers" to `/intake/{first requirement id}`.
- Intake: each of `requirements` with its id, heading, the statement as a `plainText` excerpt clamped to three lines, and a Draft badge when not accepted. Each links to `/intake/{id}`.
- Context (`CardContext`, RFC-105): three lists from `context`, Rules, Skills, and MCP servers. Each row shows the item, an origin badge (Project, Planner, Scope, or Card), and, when `actions` has `context`, "Detach" for an inherited item that is on, "Remove" for one the card attached, and "Restore" for one the card detached. A rule row also shows its category, a skill row its `useWhen`, and a server row its tool count. With `context`, "Attach skill" and "Attach rule" `SearchSelect`s list the enabled catalog skills and rules the card does not carry, from `GET /api/conventions`. "Attach server" lists the tribe's enabled servers that are off, only when `actions` has `attach-mcp`. Each change calls `PUT .../cards/{id}/context` and replaces the card. The hint reads "Changes apply from the next attempt." The footer links to `/settings/conventions`.
- Details: one label and value per row, in this order: blocked reason, failure reason, AI review, whether review was independent, "Approval: Self review: the owner approves alone" when `selfReview` is true (RFC-070), "After AI review: A person reviews" when `aiReviewMode` is `manual` (RFC-072), head SHA, reviewed SHA, approved SHA, deployed SHA, attempts used and max attempts, local check, dev check, risk, and decision ids. A SHA shows its first 7 characters in monospace with the full value in `title`. The branch and pull request link are in the header and the Pull request panel. Reasons read as words from `reasonLabel`, with unknown codes in sentence case. Omit a row when the value is null. Do not replace null with a fake SHA. When `blockedReason` is `owner_github`, a notice says the owner saves a token on the Account page and runs Test connection, then the card can be unblocked.
- Changes (`CardChanges`, RFC-068, RFC-110): loads `GET /api/projects/{id}/cards/{card}/diff` once per card id and `headSha`. The header shows the file count and the total added and deleted lines. Under it are "Expand all", "Collapse all", and a "Wrap lines" checkbox, kept in component state only. With more than one file, a `nav` "Changed files" groups the files by folder ("Repository root" for none); each is a link that opens its file, scrolls to it, and focuses its header. Each file is a section whose header is a button with `aria-expanded`, sticky under the shell bar, that shows the folder muted and the file name bold, a "cut" badge when truncated, and `+a -d` or "binary". A diff of 10 files or fewer and 400 changed lines or fewer opens every file; a larger one starts closed. Open state is keyed by the commit, so a new head starts from that default. Each hunk starts with "Lines a to b" from its header (`$lib/diff`) and the function context. Each line is a row with its old and new line number, a sign, and the code as a text node: added rows have a green background and `+`, removed rows a red background and `-`, and each carries screen-reader text "Added line" or "Removed line". Numbers and signs are `select-none`. Code scrolls sideways, or wraps when Wrap lines is on. After 50 files, Show more reveals the rest. A cut diff shows "The diff is large, so it is cut", a missing commit shows "Commit {short} is not in the project clone yet.", and a card with no commit says there is nothing to show. The footer links "View these changes on GitHub" to `compareUrl` when it is set. A load error shows in the panel and does not hide the rest of the page.

Render a control only when its name is in `actions`:

| Name | Control |
| --- | --- |
| `run` | Button "Run" with "Queues this card for the worker. You can add instructions first." It opens a form: textarea "Instructions for the next attempt", 4 rows, growing, prefilled with `instructions`, hint "Optional. Every later attempt reads these with the newest note, until you change them. Markdown works." Buttons "Run" and "Cancel". The POST body is `{ instructions }` only when the trimmed text differs from `instructions`, else `{}` (RFC-073). The board's Run stays one click |
| `retry_review` | Button "Retry AI review" with "The AI review gave no verdict. The worker runs it again on this commit." It POSTs `/api/projects/{id}/cards/{card}/retry-review` with `{}` (RFC-076) |
| `approve` | Form. The primary button reads "Approve {first 7 characters of the SHA}", or "Approve SHA" when it is empty. The SHA defaults from `headSha`. Above it, when `reviewVerdict` is set, a badge "AI review: {verdictLabel}" and the short `reviewedSha`, with "The head moved after the review." when it differs from `headSha`. A button "Approve a different SHA" with `aria-expanded` opens the SHA input; with no `headSha` the input is always shown. The body is `{ sha }` (RFC-110) |
| `request_changes` | Secondary button "Request changes", beside Approve or Deploy, opens a form. Textarea "What should change?", 4 rows, growing, prefilled from the newest `notes` entry with `source` `review` when the reason is empty, hint "Optional. Prefills from the newest AI review when one exists. The next attempt gets this note and the AI review feedback. Markdown works." Buttons "Request changes" and "Cancel". The body is `{ reason }` |
| `reject` | Item "Reject" in the "More actions" menu |
| `deploy` | Button "Deploy approved SHA". |
| `merge` | Button "Squash and merge" with the git icon, primary when `approve` is absent. It posts to `merge`; a refusal shows GitHub's reason in an error toast (RFC-111). |
| `unblock` | Form. Checkbox "Reset attempt counter", unchecked by default. Button "Unblock". Text above the button from `unblockTo` |
| `wont_fix` | Secondary button "Won't fix" beside Unblock, or alone when release is refused, so Unblock stays primary. It opens a form: textarea "Why won't this be fixed?", 3 rows, growing, hint "Shown in the trace. An open pull request gets this as a comment and is closed." Buttons "Close as won't fix" (danger, disabled while the trimmed reason is empty) and "Cancel". The body is `{ reason }` trimmed; a `fields.reason` error shows on the textarea (RFC-112) |
| `reopen` | Primary button "Reopen" with "Reopen returns this card to Blocked." It POSTs `reopen` with `{}` (RFC-112) |

`unblockTo` text:

| `unblockTo` | Text |
| --- | --- |
| `ready` | Unblock returns this card to Ready. |
| `human_review` | Unblock returns this card to human review. Approve the head again to redeploy. |
| `null` on a blocked card | Accept the changed requirements in Intake before this card can be released. |

These controls live in `CardActions.svelte`, which takes the card object as its only prop.

`{#if actions.includes('approve')}` is the pattern. Do not also check `status` in the template. The API list is the source.

After a successful POST, replace the page state with the returned card. On `422`, keep the form values and show `fields.sha` on the SHA input (which opens it), `fields.reason` under the reason of the form that sent it, `fields.instructions` under the instructions, and `error.message` in the form's `FormAlert`. An action with no form shows its error in an error toast. A successful Request changes closes and clears the reason form. A successful Run closes the instructions form. On `409`, reload the card and show the info toast "Card changed. Reloaded."

A link "Trace" goes to `/cards/{id}/trace`.

`in_progress` has an empty `actions` array. The page shows the fields and no approve form. An `approved` card shows Deploy only when `actions` includes `deploy`, plus Request changes and Reject when those names are present, and no Approve. Those are the checks the verification guide looks for. The page does not show Deploy because the status is `approved`.

## Intake

`GET /api/projects/{projectId}/intake`.

The page has two tabs, Requirements and Decisions, each with its count. `?tab=decisions` selects Decisions. The header badge says how many requirements are accepted.

A toolbar sits above each table. The search box matches id, statement, heading, source path, and acceptance lines, ignoring case. On Decisions it matches id, statement, and source path. The Requirements tab also has a status filter, "Needs acceptance", "Accepted", or "All", each with its count, and a source select: All sources, PRD, RFC, Manual, or Draft. The filters live in the URL as `q`, `status`, and `source`, so returning from `/intake/[id]` keeps them. Without `status`, the page shows "Needs acceptance" while any requirement is unaccepted, and "All" otherwise. When nothing matches, an empty state offers "Clear filters".

Requirements are a table: a select checkbox, id, statement as a `plainText` excerpt clamped to two lines with the source path under it ("Added by" with `createdBy` for a manual row), source badge ("Draft" in amber), reviewer count, the first of `linkedCards` as a link to `/cards/[id]` with "+n" for the rest, Accepted or Not accepted, and a details button with `aria-expanded`. The checkbox column renders only when a shown row can be accepted. Under 1280px wide the reviewer column is hidden, and under 720px the source and card columns are hidden too. Clicking the statement or the details button opens a row under it with the full statement rendered as markdown, the acceptance lines each rendered as inline markdown, the source path or "Added by" with `createdBy`, "Ingested by" for a parsed row with `createdBy`, the `reviewers` as badges by name, and the card links. A manual row has an "Edit requirement" link to `/intake/[id]`. A parsed row whose `canEditReviewers` is true has an "Edit reviewers" link to the same page (RFC-054). Decisions are a table of id, statement, and source path.

Each table renders 50 rows, then a "Show 50 more" button. Changing the tab, search, or a filter goes back to 50 rows and drops selected ids that are no longer shown.

Button "Ingest" POSTs `ingest` and replaces the list. When the response has `skipped`, a notice names those ids and says the manual text was kept. Button "Add requirement" links to `/intake/new`.

Acceptance needs `canAccept` (RFC-053). When it is false, there are no checkboxes and no accept buttons. When it is true, each unaccepted row has a checkbox, and the header checkbox selects every unaccepted row that the filter shows, including rows past the first 50. While any row is selected, a sticky bar at the bottom says how many, with Clear and the page's primary button, "Accept selected", which POSTs `accept` with the selected ids. "Accept all shown (n)" in the toolbar POSTs `accept` with every unaccepted id the filter shows. After an accept, the selection is cleared and the page shows the returned intake.

`/intake/new` has an optional id field with the hint "Leave blank to use the next free REQ- number.", a statement, and acceptance lines, one per line. `RequirementForm` puts one Write/Preview `Segmented` control, labelled "Requirement view", above the statement and acceptance. Write shows a statement textarea of 8 rows with the hint "Markdown works: headings, lists, code, bold, italic, and links." and an acceptance textarea of 6 rows with the hint "One check per line. If you paste a markdown list, only its items are saved. Markdown works inside a line: code, bold, italic, and links." Preview renders the statement with `Markdown` and the acceptance lines, split by `toLines` as they will be saved, with `AcceptanceList`; an empty one shows "Nothing to preview", and each field keeps its API error under its preview. `toLines` (through `acceptanceChecks`) reads the textarea the way ingest reads an Acceptance section: when the text has a markdown list, `listItems` keeps only its items, each as its own inline markdown, a wrapped item joined with a space and a nested item as its own check, and leaves out headings, bold labels, prose, rules, and fenced code. Preview then says "N lines are not list items, so they are not saved." Without a list, each non-blank line is a check. A leading list or checkbox marker is dropped either way, the same rule as the API. `/intake/[id]` loads saved checks with `fromLines` as a `- ` list, so adding a list item later keeps them. Both textareas grow with their text (`Field` `grow`), so the page scrolls instead of the box. It POSTs `requirements` and returns to `/intake`. `/intake/[id]` shows the id read-only, edits the statement and acceptance with `PUT`, and deletes after a confirm step. For a row that is not manual it says to edit the requirement in the repository and shows no requirement form. Field errors come from the API `fields`.

Both pages pass `refine` to `RequirementForm` (RFC-086, RFC-087): a `RefineStream` that calls `streamRefine` with the requirement id (the typed id, or blank, on `/intake/new`). They also load `getLLM` and `getRouter` and pass `models` (`{ id, protocol, family }` for each listed model) and `implementModel` (RFC-124); when either load fails they pass no models and the refine box shows no model selects. In Write, the form then puts a "Refine with AI" button (`Icon name="sparkles"`) at the bottom right inside the Statement and the Acceptance textareas, through `MentionTextarea`'s `cornerAction` snippet; the textarea keeps `pb-14` so text never runs under it (RFC-123). Each is a text button in accent with the sparkles icon and a small chevron, with no fill or border, inside a 44px hit area. Hover and keyboard focus underline it, with no outline, and the chevron turns over while its panel is open. It is labelled "Refine statement with AI" or "Refine acceptance with AI", with `aria-expanded` and `aria-controls`, and toggles a `RefinePanel` under its own field. Both panels may be open and streaming at once; each keeps its own message draft (RFC-125), and the panel's close button collapses it the same way. Applying a refined statement shows "Statement updated." under it with "Refine acceptance to match" (or "Draft acceptance from it" when there are no lines) and a dismiss button; that opens the acceptance panel with `autostart`, which starts the refine at once. The offer clears when used, dismissed, when the acceptance panel is opened by hand, or when the statement is edited. Preview shows no buttons and no panels, and leaving Write aborts a running stream.

MCP mentions (RFC-101). Both pages pass the intake's `mcpServers` to `RequirementForm` as `servers`, `[]` when the field is missing. The statement and acceptance are `MentionTextarea` fields, which keep `Field`'s label, hint, error, and growth, and add the hint "Type @ to name an MCP server." Typing `@` at the start of the text, or after a space or an opening bracket or quote, opens a listbox under the field with the tribe's slugs that start with what follows the `@`, each with "N tools" or "Disabled" as the detail; with no server it shows "No MCP servers in this tribe." While the list is open, the arrow keys move, Enter or Tab inserts `@slug ` in place of the typed text, and Escape closes it; while it is closed, the keys do what they do in any textarea, and keys during IME composition are ignored. The textarea keeps its role and sets `aria-autocomplete="list"`, `aria-controls`, `aria-expanded`, and `aria-activedescendant`; options are 44px tall. Under each field a list labelled "MCP servers mentioned" shows the field's mentions from `mentions` in `$lib/requirements`: a known enabled server as a plain badge, a disabled one amber with "disabled: jobs will not call it", and a slug the tribe does not have red with "no server with this slug". These never block Save. On `/intake/[id]` a row that is not manual shows its stored `mentions` the same way, and the expanded row on `/intake` adds "MCP servers" with the row's mentions as badges.

`RefinePanel` handles one field as a chat card (RFC-125): a header, a thread, and a composer, in a `rounded-panel` bordered surface.

- Header: a sparkles mark, "Refine statement" or "Refine acceptance", and a line saying what else the model reads: "Reads your acceptance: N lines" (or "1 line", or "No acceptance yet") for the statement, and "Reads your statement: N words" for the acceptance (RFC-123). On the right, a ghost "New chat" button once the thread has a turn, and an icon button "Close refine" that calls `onclose`.
- Thread: each turn is the member's message as a right-aligned bubble (an empty message reads "Refine this statement" or "Refine these acceptance lines") and the model's reply on the left under a small sparkles avatar and the model id. While a reply streams it shows "Waiting for the implement model…" (or "Waiting for <model>…" when one is chosen), then "Writing with <model>" after `start`, and the text as the deltas arrive in a region labelled "Refined statement so far" (or acceptance) with `aria-live="polite"`, `aria-busy`, and a blinking caret (`motion-safe:animate-pulse`). A `reset` event empties the text so far, because that turn ended in MCP tool calls, and a `tool` event shows "Looking up <server> / <tool>" in place of the model line until the next delta (RFC-095). "Stop" aborts the fetch and the turn says "Stopped. Nothing changed." An `error` event, a rejected request, or a stream that ends without `done` shows the message in an alert in that turn with a "Try again" button that sends the turn again. The latest finished turn shows `RefineCompare` from the field as it was when the thread began to that reply; earlier replies show as plain text. Apply passes the latest reply to `onapply`; Discard and New chat empty the thread.
- Composer: a `rounded-2xl` bordered box with a textarea labelled "What to improve (optional)" (`sr-only`), three rows growing to about twelve, at most 1,000 characters, placeholder "Ask for a change, or press Enter to refine as is". Enter sends and Shift+Enter starts a new line; keys during IME composition are ignored. The textarea draws no outline of its own; the composer box shows focus with a darker border. Under it, when the page passes `models`, three `ChipSelect`s: Model (options grouped under their family, the implement route's model tagged "Implement"), Thinking, and Effort, whose choices come from `thinkingChoices` and `effortChoices` for the selected model's protocol with "Model default" first, effort levels capitalised ("Medium") (RFC-124). The form holds the three values and binds them into both panels, starting on the implement route's model, and a model change drops a thinking or effort value the new protocol does not take. Each chip shows a short value and opens its own menu: the Model chip a `cpu` icon and the model id, and the Thinking and Effort chips their muted name and the choice, with "Default" for "Model default". Focus tints the chip, with no outline. The chips are disabled while streaming. On the right a round 44px send button (`bg-ink text-surface`, `arrow-up` icon) named "Refine" becomes "Stop" (`stop` icon) while streaming.
- Sending: with a blank statement it says "Write a statement first, then refine it." (or, for acceptance, "Write a statement first. Acceptance is refined against it.") above the composer and sends nothing. The first turn calls the stream with `field`, the trimmed statement, `toLines(acceptance)`, the message as `instructions`, `model`, `thinking`, and `effort`, and an `AbortController` of its own. A follow-up sends the latest finished reply in place of the refined field (the statement, or the acceptance lines), and the other field from the form. `autostart` sends an empty first message on mount.

`RefineCompare` takes `field`, `before` and `after` (for acceptance, the lines joined by newlines), and `modelId`. It has a "Refined by <model>" heading, a "<Field> comparison view" `Segmented` control with Changes, Before, and After, the statement as a word diff from `diffWords` in `$lib/textdiff` with `<ins>` and `<del>` and screen-reader "Added:" and "Removed:" labels, or the acceptance lines as a line diff from `diffLines` marked `+` and `−` with the added and removed counts. Before and After render each version with `Markdown` or `AcceptanceList`. The view sits in a card whose footer holds the hint "Nothing is saved until you press Save." and, on the right, Discard as a quiet text button and Apply as an ink button with a check icon. When that field changed after the request, the footer says "You edited this field after asking. Apply replaces those edits." in place of the hint. "Apply" sets only that field (the statement, or `fromLines` of the lines) and closes the panel; "Discard" returns the panel to its input. Nothing is saved until the page's Save or Add requirement. The edit page's panel hint is "Changing the text, by hand or with the refine buttons, clears acceptance when you save." Rows that are not manual have no form, so no refine buttons.

`streamRefine(body, onEvent, signal)` in `api.ts` uses `fetch` with the session bearer, because `EventSource` cannot send an `Authorization` header. It shares `send` with `request`: the bearer, the JSON content type, and the 401 handling. A non-OK reply becomes `ApiError` from its JSON body. On `200` it reads `response.body.getReader()` with a streaming `TextDecoder` (`readStream`, shared with `streamTrace`), and `parseEvents` splits the buffer on blank lines, keeps an unfinished event for the next chunk, skips comment lines, and turns each `event:` and `data:` pair into an object whose `kind` is the event name, set after the data so a payload's own `kind` cannot replace it. An abort rejects with the fetch's `AbortError`, not a network error.

Both pages name PR reviewers with a `MultiSearchSelect` (at most 15). Options are `reviewerOptions` without the row's creator, labelled by name with the login and email as the detail line, or "No verified GitHub login" and the email for a member without a login (RFC-103). The search placeholder is "Search by name, login, or email", or "No other tribe members yet" when there is no option. A hint under the picker reads "Someone without a verified GitHub login is asked to review on GitHub after they verify." On `/intake/[id]` a Reviewers panel shows for every row: editable with "Save reviewers" when `canEditReviewers` is true, which PUTs `requirements/{id}/reviewers` with `{ userIds }`, and read only otherwise. On `/intake/new` the picker sits under the acceptance lines and leaves out the caller. After the POST, a non-empty pick is saved with the same PUT. If that PUT fails, the page opens `/intake/[id]` for the new row so the reviewers can be saved again.

Self review (RFC-070) is a `Checkbox` under the reviewer picker. On `/intake/new` it shows only when `canSetSelfReview(shell.role, shell.admin)` from `$lib/requirements` is true, with the hint "You review and approve this requirement's cards yourself. The approval is recorded as not independent.", and the POST sends `selfReview`. In the Reviewers panel on `/intake/[id]` it shows when the row's `canSetSelfReview` is true. "Save reviewers" then PUTs `reviewers` when the picks changed and `self-review` with `{ selfReview }` when the box changed. A caller who cannot change it sees "Self review: on. The card owner approves alone." when the flag is on, and nothing when it is off.

The AI review mode (RFC-072) sits under Self review in both places and follows the same rule for who sees it. `AIReviewModeField` is a `Segmented` control labelled "When the AI review asks for changes" with "Run the next attempt" (`continue`) and "A person reviews" (`manual`), from `reviewModes` in `$lib/requirements`, and a hint under it that explains the chosen option. On `/intake/new` the POST sends `aiReviewMode`, and `continue` when the caller cannot choose. In the Reviewers panel "Save reviewers" also PUTs `ai-review-mode` with `{ aiReviewMode }` when the choice changed. A caller who cannot change it sees "When the AI review asks for changes, a person reviews before another attempt." when the row is `manual`, and nothing for `continue`.

When `unnumbered` is not empty, a notice says "N PRD files have no REQ- headings, so ingest read no requirements from them.", lists the first 5 paths and "and N more" for the rest, and ends "Add a heading such as `## REQ-001 Title` to each requirement, add requirements by hand, or change the PRD path in Repository settings." with a link to `/settings/repository`. The page does not poll and the accept buttons stay enabled. When `planError` is set, show it under the heading as text. Ingest and Accept show a spinner on their button; Accept ends with the success toast "Accepted. Cards appear on the board shortly."

## Trace

`GET /api/projects/{projectId}/cards/{id}/trace`, `GET /api/projects/{projectId}/cards/{id}/trace/stream` while the card works, and `GET /api/projects/{projectId}/cards/{id}` for the title and status. No model call from this page.

While the response's `live` is true and the tab is visible, the page holds one trace stream (RFC-088, RFC-132): `streamTrace(id, since, onEvent, signal)` in `api.ts`, with `since` set to the newest event's `ts`, read with `fetch` and the bearer like `streamRefine`. A `trace` frame merges its events by `id` (`trace.mergeEvents`, ordered by `ts` then `seq`) and sets `attempts` and `live`. `turn` starts the reply being written (`pending`: attempt, turn, role, model, and text) and clears live thinking, `delta` text for that turn is added once per animation frame, `thinking` appends to `pendingThinking` for that turn (terminal view only; never stored), `reset` and `turn_end` clear thinking, and a merged `model_call` of the same attempt and role, or `live` turning false, clears both. `done` with `reconnect` and `resync` open the stream again; `resync` first reads the whole trace. The stream closes while the tab is hidden and opens again when it is shown. When the stream fails or ends without `done`, the page reads the trace route every 3 seconds (every second in the terminal view) with `since`, as RFC-074 did, and tries the stream again after 5 seconds. A poll is skipped while a request is in flight, a response for another card is dropped, and a failed poll keeps the rows already shown. The card is read again when `live` changes or an attempt ends. Events are kept in `$state.raw`.

The page reads as a timeline, oldest first:

- Header: "Trace", the card title as the description, and the status badge in `meta`, with a sky "Live" badge while `live` is true, titled "This page shows output as it arrives".
- Now running: while `live` is true, a strip above the summary shows "Attempt N" (or "Waiting for the worker" before an attempt opens), the phase from the newest status change, the elapsed time from `startedAt` ticking each second, "Turn X of Y" from the attempt's implementer model calls and `maxTurns`, the attempt's tokens, and the newest event's label, detail, and age, or "The model is writing…" while a reply streams. An info toast announces only when an attempt ends, such as "Attempt 2 ended: Checks passed".
- Follow latest: a toolbar checkbox shown while `live` is true, on by default. The toolbar (View, search, filters, and Follow latest) is sticky under the shell header (`top-12`) so it stays reachable while scrolling. When a poll adds rows, the page scrolls the newest entry into view. When every timeline entry was shown, new entries show too (`pager.follow`); a block that showed every row keeps showing every row as rows arrive, and a block keeps its open state.
- Summary: a strip shows attempts, events, total model tokens, errors (a link to `?kind=job_error` when above zero), and the first and last event time.
- Day headers split the timeline where the local day changes.
- Milestones: events without an attempt, such as status changes, refused status changes, errors, and reviewer requests. Consecutive milestones share one bordered list.
- Attempt blocks (`TraceBlock`): a run of consecutive events with one `attemptId`. The reviewer's events carry the implement attempt's id, so they form a block titled "Review of attempt N"; a later run of the same attempt and role is "(continued)". The header is a button with `aria-expanded` that shows the task class, model, model calls, tools, tokens in and out, duration (`startedAt` to `endedAt` for an implement block, first to last event otherwise), a red problem count, and the outcome badge and short commit, or for a review the verdict and "Not independent". The latest block and any block with a problem start open. Every block starts open while a search or filter is set. A block shows 50 rows, then `ShowMore`.
- Rows (`TraceRow`): one 44px line per event with the local clock time (ISO in `title`), an icon, a label, a detail, and right-aligned meta such as "1,616 in / 74 out". `trace.describe` gives every kind a template; an unknown kind reads as its name in sentence case. A tool row puts its `args` first, such as "src/x.ts · 120 lines", `"useSession" · 3 matches`, "exit 0 · npm test", or a commit's first line. A model row with `text` shows the reply's first line as its detail and the whole reply when open; without `text`, the model id and stop reason. Red rows are errors, failed tools, failed checks, and moves to Blocked; amber rows are refused calls and changes requested. A row is a button with `aria-expanded`; open, it shows the full detail, the long text, the kind, time, attempt id, and event id, and a "Raw event" toggle with the JSON.
- Long text (`TraceText`): control characters other than newline and tab are stripped, text is cut at 4,000 characters with "Show all (size)", and text that was mostly NUL bytes is hidden as binary output until "Show anyway". Every value renders as text.

The toolbar searches the kind and its label, attempt id, the row's label and detail, and payload text, and filters by kind and attempt. The timeline renders 50 entries at a time.

The toolbar's View switch (`Segmented`) picks Structured, the timeline above, or Terminal, kept in `?view=terminal` (RFC-078). The filters apply to both. The terminal view (`TerminalView`) is one `bg-ink` monospaced box with `role="log"`, `aria-label="Terminal output, oldest first"`, and `tabindex="0"`, at most 75% of the viewport high. `terminal.terminalLines` turns each event into a local clock time, a head line, and its text:

- A model call: `model  role  modelId  1.5k in / 20 out  stop tool_use`, then the reply. Sky.
- A tool call: `$ read_file  src/x.ts`, then the whole stored `summary`. When `edit_file` or `replace_in_file` args include capped `old`/`new` (RFC-132), the terminal renders a dark edit card (`TerminalEditCard`) with path, `+N -M`, a line diff, and light syntax highlighting from `codeHighlight` instead of the summary body; older events without snippets keep the summary (`edit_file` still adds "(N lines)" in the head path). An `mcp_<slug>_<tool>` row is labelled "MCP: slug / tool" in both views, with its byte count as the detail; `mcp_context` ("MCP servers") lists each server with its tools and short hashes, or its error, and is amber when a server failed; a database server is located as `host/database` (RFC-097). When `scope` is `mentioned` the detail says the servers come from the requirements' mentions; when it is `none` the row says "No MCP servers" with the `reason`, and each `missing` slug is listed as "not a server: slug"; either of those is amber (RFC-102). `assertion_failed` `mcp_changed` reads "MCP tool changed: slug / tool". A commit's head is its first line; `move_file` shows `from -> to`, a ranged read adds "from line N", and `run_command` shows its argv and is coloured like a check; `comment_pr` and `verdict` add the body; the two context tools list their items. A refused call adds `(refused)`. Failed and refused calls and a check not starting with `exit 0` are red, a passing check green.
- A status change: `transition  from -> to  evidence`, amber, or red into `blocked`. A job error: `! job_error` and the whole message, red.
- Any other kind: its name and `trace.describe` detail and text. An unknown kind shows its payload as JSON.
- `--- attempt N  taskClass  modelId  ---`, muted, where another attempt's events begin.

ANSI color codes and control characters are stripped before `cleanText`, with no length cap. Text past 200 lines (`foldLines`) shows "Show all N lines". The view renders the newest 300 entries (`pageEvents`), and "Show earlier events (N hidden)" adds 300. The view scrolls to the bottom when entries arrive while `follow` is on. Scrolling more than 24px above the bottom turns `follow` off and shows "Jump to latest". `follow` binds to the page's Follow latest checkbox.

`TerminalView` takes `pending`, the reply being written, and optional live `thinking` (RFC-132). Thinking renders before the reply as a muted "thinking" head and the text so far. The reply uses `terminal.pendingEntry`: the head `model  role  modelId  writing…` in sky, then the text so far with ANSI codes stripped, or "Waiting for the model…" before the first text, and a pulsing caret (`aria-hidden`, still under reduced motion). While either shows, the log has `aria-busy="true"`, so a screen reader is not sent each piece. The view keeps following the bottom as the text grows. The page passes no `pending` or `thinking` when the Kind filter is set to anything but Model call, the Attempt filter names another attempt, or the search does not match the text.

## Settings pages

`SecretField.svelte` is used for the caller's GitHub token, the company GitHub token, each LLM key, and the deploy token.

1. Input is a `PasswordInput` (masked, with the eye toggle), autocomplete `off`.
2. If the GET flag (`tokenSet`, `keySet`, or `deployTokenSet`) is true, placeholder "A value is saved".
3. Empty input on submit means the JSON omits that secret field.
4. A "Remove" button sets `clear: true` and omits the secret string.
5. The value is never written to a toast, an alert, or the console.

The submit button shows a spinner while the PUT is in flight. On `422`, paint `fields` under the matching inputs, show the message in `FormAlert`, focus the first invalid input, and keep what the operator typed.

The five project settings pages (Repository, Linked projects, Deploy, Workflow, Conventions) read `canEdit` from their GET body (RFC-059). When it is false, the fields sit in a disabled `fieldset`, or the pickers get `disabled`, and `SaveBar` gets `disabled`. Its note reads "Only a Lead Developer, a tribe admin, or an admin can change these settings." When it is true, the note names no role. The client never decides `canEdit` from the role itself.

### Account

`src/routes/account/+page.svelte`. `GET /api/me` and `GET /api/me/github`.

The first panel is Name. "Save name" calls `PUT /api/me` and is disabled while the name is unchanged. A `422` shows on the field. After a save the sidebar menu shows the new name. The page title is the name, or the email when unnamed.

Password fieldset: current password, new password, button "Change password", `PUT /api/me/password`. A wrong current password shows "Current password is wrong." under that field and keeps the person signed in.

GitHub fieldset: host, login, token. The help text says this is the caller's own account, and that cards built from requirements the caller brought in are committed, pushed, and opened as pull requests with this token (RFC-054). Clones and fetches still use the company account. The panel header shows a "Verified" or "Not verified" badge from `verified`. "Test connection" calls `POST /api/me/github/test`. Disable it when `tokenSet` is false and the token input is empty. Show `login` on success. Show `message` on `ok: false`. After the test, reload `GET /api/me/github` so the login and badge show what GitHub reported.

Browser notifications (`NotificationsPanel.svelte`, RFC-113) is the last panel. `src/lib/push.ts` holds the browser calls:

- `syncPush` returns `unsupported` without a service worker, `PushManager`, or `Notification`; `blocked` when permission is `denied`; `on` when the browser holds a subscription and permission is `granted`, after sending that subscription again with `POST /api/push/subscriptions`, since the server deletes failing ones; else `off`.
- `enablePush` asks for permission, reads `GET /api/push/key`, registers `/service-worker.js` (as a module in dev), waits for `ready`, replaces a subscription made with another key, subscribes with `userVisibleOnly: true`, and posts it.
- `disablePush` unsubscribes and then sends `DELETE /api/push/subscriptions` with the endpoint.

The header badge reads On, Off, or "Blocked in browser settings". Off shows "Turn on". On shows "Send a test" (`POST /api/push/test`; `sent: 0` says no browser took it) and "Turn off". Blocked tells the person to allow notifications in the browser's site settings and shows no button. The hint says a notification shows the card title and project only. Results go to toasts: success for on, off, and a sent test, and an error toast for a failure or a test no browser took.

`src/service-worker.ts` is built by SvelteKit with `serviceWorker.register: false`, so only the panel registers it. On `push` it shows the payload's `title` and `body` with `tag` and `data.url`. On `notificationclick` it focuses a window of this origin and navigates it to the URL, or opens a new window. A URL that is not a single-slash path becomes `/`. It caches nothing, handles no `fetch`, and calls no API.

### Users and tribes

`src/routes/admin/users/+page.svelte` lists users. Each row shows the name with the email under it, or the email alone when unnamed. Create is `/admin/users/new` (name, email, password, role, admin) and calls `POST /api/users`. The name is required. Edit is `/admin/users/[id]` with a Name panel and calls `PUT /api/users/{id}`. It sends `name` unless the field is blank and the user never had one, so changing the role of an unnamed user still saves. A `403` shows `error.message` in the page's error panel or the form's alert.

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

### Project and tribe overview

`src/lib/components/SummaryDashboard.svelte` (RFC-127) loads a summary for a UTC range (`?range=` 7/30/90, default 30) and shows totals, pipeline bars by board group, attention (including blocked reasons), range insights (done, rework, budget stops, answer sources), and optionally a projects table and MCP health. `/overview` calls `getProjectSummary` for the selected project and lists aging cards with links to `/cards/{id}`. `/tribes/[id]/overview` calls `getTribeSummary`, shows projects and MCP health, and links to Token usage. A tribe admin's members page also links to Overview.

### Token usage

`src/routes/admin/usage/+page.svelte` (RFC-089) calls `getUsage(from, to)` for the range and does the rest in the browser with the pure functions in `src/lib/usage.ts`. Every choice lives in the URL through `setParams`, and a default value leaves its parameter out:

- **Range**: a `Segmented` for 7, 30 (the default), or 90 days (`?range=`). `rangeWindow` turns it into UTC days ending today, and a change reads the route again.
- **Group by**: a `ChipSelect` for LLM, Model (the default), Project, User, Task, Source, or Card (`?by=`).
- **Filters**: not a permanent row. "Add filter" is a `ChipSelect` of dimensions that are not already open; picking one opens that dimension's `FilterSelect`. A dimension already set in the URL (`?llm= &model= &project= &user= &task= &source= &card=`) opens its select on load. "Clear filters" shows while any filter is set or open and clears the URL params and the open set. The value is the family, model, project, user, or card id, `intake` or `card` for Source, or `-` for a call with none ("Unknown family", "No owner", and "Intake" for a refine under Card) (RFC-091). Grouped by card, each name cell has the card id as its `title`, since titles can repeat. `usageOptions` lists each value with its total tokens, counting the rows the other filters keep, largest first.
- **Totals**: a strip with each metric as a muted label above a bold value (total tokens, input, output, cached input, calls, and cache share). Cache share is cached over input plus cached, since providers count cached input apart. Total is input plus output.
- **Tokens per day**: one CSS bar per day of the range, a day with no calls included, each with a `title`. The bars are `aria-hidden` and a visually hidden table gives the same numbers. No chart library.
- **Table**: the groups with calls, input, output, cached, total, and share of the filtered total as a bar and a percent. The name and number headers are 44px buttons with `aria-sort`; a first click sorts numbers largest first and names A to Z, and a second click reverses (`?sort= &dir=`, default total, largest first). `ShowMore` after 50 groups.

An empty range shows "No model calls in this range"; filters that keep nothing show "No calls match" with Clear filters. A `403` shows `error.message` in the page's error panel or the form's alert.

The page body is `src/lib/components/UsageDashboard.svelte` (RFC-090), with props `title`, `description`, `load(from, to)`, `dimensions` (all five by default), and an optional `actions` snippet. A dimension not listed is left out of the filters and Group by, and its URL parameters are ignored; `?by=` falls back to model. The data is read again when `load` or the range changes. Three routes use it:

- `/admin/usage` with `getUsage`.
- `/tribes/[id]/usage` with `getTribeUsage(id, ...)`, titled "Token usage: {tribe}" from `getTribes()`, with all five dimensions and a "Back to tribe" link. The tribe page links to it with a "Token usage" button.
- `/account/usage`, "My token usage", with `getMyUsage` and the dimensions LLM, model, project, task, source, and card, and an "Account" link. The account page links to it with a "Token usage" button, and the user menu and the mobile menu with "My token usage".

### Company GitHub

`src/routes/admin/github/+page.svelte`. GET and PUT `/api/settings/github`. Admin only. Fields: host, organization, token. "Test connection" calls `POST /api/settings/github/test`, and shows "Signed in as LOGIN. Sees N repositories in ORG (M private)." on success or `message` on `ok: false` in the Bot account panel footer, next to Save. A non-empty `hint` shows in a `warning` `Notice` in the Bot account panel. The saved bot login shows as read-only text.

### Repository

`src/routes/settings/repository/+page.svelte`. GET and PUT `/api/projects/{id}/settings/repository`. Shows the company host and organization as text, the repository and base branch as fields, and whether the clone exists with its head SHA. The repository is a `SearchSelect` of `GET /api/settings/github/repos` with no free-text fallback, as on New project. When the list loads empty, the picker's error line says the company token sees no repositories in the organization, and an admin gets a "Check Company GitHub" link to `/admin/github`. New project shows the same line and link. "Sync" calls `POST /api/projects/{id}/sync`. It is enabled when `canEdit` is true or the caller is a Developer, Senior Developer, or Lead Developer, matching the server. A Docs panel has PRD path and RFC path as monospace fields, hinted "Ingest reads requirements from this folder of the repository" and "Ingest reads decisions from this folder of the repository", saved with the repository by the same Save (RFC-126).

### Linked projects

`src/routes/settings/links/+page.svelte`. GET and PUT `/api/projects/{id}/settings/links`. Each link is a panel with the project name, its repository, a list of shared paths with "Add path" and a remove button per row, and "Remove link". "Add link" is a `SearchSelect` of projects in the same tribe that are not this project and not linked yet. Projects from other tribes that the user can see follow as disabled options with "Other tribe: name" as the detail, so a search explains why they cannot be picked. The hint names the tribe. The picker clears after a pick. When the projects do not load, the page's error panel says so with "Try again". The save note says links are one way. The page explains that linked context is read only.

### LLM APIs

`src/routes/admin/llm/+page.svelte` lists the company families from GET `/api/settings/llm` in a table: id, protocol, base URL, the first model ids, and whether a key is saved. Each row links to its edit page. "Add family" in the page header opens `/admin/llm/new`. A toolbar searches and filters the families (see Search and filters).

`src/lib/components/FamilyForm.svelte` is the form both pages use: id, protocol select with `anthropic` and `openai` only, base URL, model ids (a list plus "Add model" and a remove button per row), API key. Under each model id are its Thinking select (Provider default, On, Off, and Adaptive on anthropic only), a Budget number field shown for On on anthropic, an Effort select whose values follow the protocol, with Provider default first (RFC-079), a History budget (KiB) number field on either protocol, empty for the 16 KiB default (RFC-081), and a Context window (tokens) number field on either protocol, empty for the 128,000 default, with the hint "The agent compacts its work at 75% of this." (RFC-117). They save in the family's `modelOptions`.

The trace page words the RFC-117 and RFC-118 events: `context_compacted` reads "Context compacted" with the input tokens, `handoff_saved` reads "Handoff saved" with the file count and whether the model or the harness wrote the summary, and `handoff_applied` reads "Continued from the last attempt" with the file count, or "Earlier work could not be re-applied" in amber with the reason. `agent_run` (RFC-121) reads "Claude Code" with the model and the turn count, red when `exitCode` is not 0.

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

### Agent

`src/routes/admin/agent/+page.svelte` (RFC-120). GET and PUT `/api/settings/agent`, GET `/api/settings/llm` for the family and model selects, and GET `/api/settings/router` for the turn budget placeholder. Admin only, like Router.

- A Segmented control "Implement agent" with "Built-in" (`builtin`) and "Claude Code" (`claude_code`). It changes the draft; the Save button saves.
- With Claude Code, a Panel "Claude Code" shows:
  - "Model family", a Select of the families whose protocol is `anthropic`. A family without a saved key is a disabled option marked "No API key", and a hint links to LLM APIs.
  - "Model" and "Small model (optional)", Selects of the chosen family's models; Small model starts with "Same as model".
  - "Max turns", a number with the router's turn budget as the placeholder.
  - "Auth header", a Select: Auto, `x-api-key`, and Bearer.
  - A checkbox "Allow web fetch and search".
  - "Extra instructions", a textarea, at most 8 KiB.
  - "Claude binary on this server", optional, monospace, with the hint "Empty uses claude on PATH. Runners use their own."
  - A ghost Button "Check" that calls POST `/api/settings/agent/test` and shows the version, or the message in red.
- A `422` puts each `claudeCode.<field>` error under its control. Saving shows a toast.

### Runners

`src/routes/admin/runners/+page.svelte` (RFC-106, RFC-107). GET `/api/runners`, polled every 5 seconds while the page is open.

- A Segmented control "Implement jobs run on" with "This server" (`local`) and "Runners" (`runners`) saves through PUT `/api/settings/jobs`. With `runners` and no runner `idle` or `busy`, a note says queued cards wait until a runner connects.
- A Table: name with labels, status Badge (`idle`, `busy`, `draining`, `offline`), slots as `busy/slots`, version with "Update required" when `updateRequired`, last heartbeat as a relative time, and the card links of its leases. Each row shows one text button per entry in the runner's `actions` from the API and nothing else; `remove` asks to confirm. The buttons sit in the row, not in a dropdown, because the Table's horizontal scroll clips a dropdown.
- An "Agents" column lists each runner's `agents` as "Built-in" and "Claude Code" (RFC-120). While `agent` is `claude_code` and no `idle` or `busy` runner lists `claude_code`, a warning note says "No runner has Claude Code. Queued cards wait." The note shows only when `executor` is `runners`.
- Search by name or label, and a status FilterSelect, both in the URL through `setParams`.
- The header Button "Add runner" goes to `/admin/runners/new`, and a ghost Button "Runner guide" goes to `/admin/runners/guide`.

`src/routes/admin/runners/new/+page.svelte` calls POST `/api/runners/tokens` once on a button press, then shows the install command with the API origin, the token, and the expiry, with a copy button. Leaving the page drops the token; it is never stored in the browser.

`src/routes/admin/runners/guide/+page.svelte` is static and calls no route. It covers the prerequisites, registering with the install script, registering by hand with `orchestrator runner register` and `run`, updating by rerunning the script without a token, Drain and Resume, and unregistering: Remove on the Runners page first, then the commands that disable the unit and delete the `orchestrator` user, its home, and the binary. Commands use the API origin and `<token>` as a placeholder, each with a copy button.

```bash
curl -fsSL <api>/runner/install.sh | sudo bash -s -- --url <api> --token <token> --name <name> --slots 4
```

### Conventions and skills

`src/routes/admin/conventions/+page.svelte` reads `GET /api/conventions` (RFC-055). The Rules tab groups rules under Global, then one table per stack by name: title, the first line of the body, category, version, and a Disabled badge. Each group has an "Add ... rule" link that opens `/admin/conventions/new` with the stack chosen. The Stacks tab (`?tab=stacks`) lists each stack with its markers and rule count. Rule and stack text renders as plain text.

`RuleForm.svelte` has "Applies to" (every project or one stack), the stack, category, title, the rule body as a monospace `Field type="textarea"`, and Disabled. `StackForm.svelte` has the name and the marker files, npm dependencies, and Python dependencies as comma separated fields. `/admin/conventions/[id]` picks the form from the id prefix (`stk_` is a stack), shows the version and "Last edited by" the email and time, saves with PUT, and has a delete panel behind an inline confirm. Delete says a stack still named by a rule or project is disabled instead.

`src/routes/admin/skills/+page.svelte` lists skills from the same GET (RFC-056): title, slug, "use when", version. `SkillForm.svelte` has slug, title, "Use when", the procedure as a monospace textarea, and Disabled. New and edit pages follow the rule pages. Field errors come from the API `fields`. A `403` shows in the form's alert.

### Tribe conventions

`src/routes/tribes/[id]/conventions/+page.svelte`. GET and PUT `/api/tribes/{id}/conventions`, and GET `/api/conventions` for the options (RFC-104). "Stacks for every project" and "Skills for every project" are `MultiSearchSelect`s of enabled stacks and skills, at most 30 each. A rules table lists every enabled rule with its title, Global or the stack name, category, and an On/Off `Segmented` control; Off adds it to `rulesOff`. Without `canEdit` the controls are disabled and the `SaveBar` is hidden. The `SaveBar` note says saved conventions apply to the tribe's projects from their next attempt.

### Project conventions

`src/routes/settings/conventions/+page.svelte`. GET and PUT `/api/projects/{id}/settings/conventions`, and GET `/api/conventions` for the options. Stacks, Skills, and Rules are three tables. Each row is an item that applies or that the project switched, with an origin badge (Detected, Tribe, Catalog, or Project) and an Inherit/On/Off `Segmented` control that edits `choices`. "Add stack", "Add skill", and "Add rule" `SearchSelect`s add an enabled item as On. Each list allows 30. A link opens the tribe's conventions. The `SaveBar` note says saved rules apply from the next attempt. A disabled `MultiSearchSelect` shows its picks without remove buttons, or "None".

### Deploy and checks

`src/routes/settings/deploy/+page.svelte`. GET and PUT `/api/projects/{id}/settings/deploy`. These settings belong to the selected project (RFC-050). Any member can read them. Save is for a Lead Developer member, and a `403` shows in the form's alert.

Fields, in three fieldsets:

| Fieldset | Fields |
| --- | --- |
| Dev server | Dev URL, dev hosts (one per line), deploy command, rollback command, health URL, deploy token |
| Checks | Local check, dev check, allowed executables (one per line), allowed commands (one per line, `commandExecutables`, RFC-084), sandbox engine select (`podman`, `docker`), sandbox image |
| Release | Deploy approved cards (checkbox `deployEnabled`, default on; off gives approved cards Squash and merge, RFC-111) |

The doc paths are on the Repository page, and the implementation cap and Solo approval on the Workflow page (RFC-126). The Workflow page shows the cap as a 1 to 8 segmented picker and Solo approval as two choice cards, "Second reviewer" and "Solo", each in one panel row with its explanation beside it.

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

Shared controls live in `src/lib/ui/`. One primary button on a page. `.danger` is for blocked status. `.error` is for a field error. Inputs and buttons are at least 44px tall. `:focus-visible` keeps a 2px ink outline on buttons, links, tabs, checkboxes, and radios. Text fields (`controlBase`: inputs, textareas, selects, search boxes) draw no outline; focus darkens their border to `--color-muted`, or red when invalid. The refine composer box does the same, and the refine chips and the "Refine with AI" trigger show focus by a tint or an underline. Do not put layout in a `style` attribute.

The theme also defines the layout scale. Page padding is 24px, and 32px from 1024px. Every signed-in page body is full width at every breakpoint, so `Page` has no width prop. A page description stays at `max-w-2xl` for line length, and the login card stays centered. Field widths are `sm` 10rem, `md` 20rem, `lg` 32rem, and `full`. A `Field` textarea with `grow` uses the `autogrow` action, which sets its height from `scrollHeight` through the CSSOM on mount, input, and value change, and is `resize-none`. `Field`, `Select`, and `SecretField` take a `width` prop that defaults to `md`. Inside a two-column grid a field uses `full`. Every password or secret input is a `PasswordInput`: `Field type="password"` and `SecretField` both render it. It starts masked, and an eye button inside the right edge (a 44px `type="button"` with `aria-pressed` and "Show {label}" or "Hide {label}") reveals only what was typed. It masks again when the value is cleared or the input is disabled.

Every page body starts with `Page`: the `h1`, an optional description, an optional `meta` snippet beside the title, and an `actions` snippet aligned right on the same row. A hairline separates the header from the content. The top bar is a 48px strip with the menu button below 1024px, the current project, and the notification bell. It holds no page actions or status text.

Buttons size to their label and never stretch. `full` is only for the login submit. `Panel` has a title row with an optional `actions` snippet and an optional `footer` snippet. The footer is a right-aligned action bar. Admin settings pages (Company GitHub, Router, and an LLM family) put the primary Save in the last panel's footer, after "Test connection" where the page has one. Project settings pages and the user edit page end in `SaveBar`, a bar that sticks to the bottom of the viewport with the primary Save. `FieldGroup` groups related controls under one legend. `Row` is one entry in a bordered list, such as one model id, with a remove button. Tables use `Table` and the `th`, `tr`, and `td` classes from `src/lib/ui/control.ts`: an 11px uppercase header and 48px rows.

`SearchSelect` is the shared searchable select for long lists: the project switcher, Router models, and tribe member search. It is an ARIA combobox. Typing filters options by label and detail, arrow keys move, Enter picks, and Escape closes and restores the selected label. An optional `emptyLabel` adds a first option that clears the value. `onchange` fires only when a pick changes the value. Short fixed lists, such as protocol or role, keep the native `Select`.

`MultiSearchSelect` picks several values from a long list, such as requirement reviewers. It wraps `SearchSelect`: a pick adds a chip and clears the input, and picked values leave the option list. Each chip has a "Remove {label}" button. An optional `max` hides the input at the limit. `disabled` shows the chips without remove buttons.

System font stack: `system-ui, sans-serif`. Below 720px board groups stack in one column. From 720px they sit in one row of lanes at least 15rem wide that scrolls sideways when the window is narrow.

## Page states

| Situation | UI |
| --- | --- |
| No bearer | Redirect to `/login` |
| Board empty | "No cards yet" and the intake link |
| Loading | `Page` spinner row names the page |
| Load failed | `Page` danger panel with "Try again" |
| Project settings page with no project | `Page` danger panel "Choose a project in the sidebar first." |
| In flight | The button that started it shows a spinner and is disabled |
| `422` | `FormAlert`, field errors under their inputs, focus on the first invalid input, typed secrets kept |
| `409` | Reload, info toast "Card changed. Reloaded." |
| `401` | Bearer cleared, redirect to `/login` |
| `429` on login | `FormAlert` "Too many attempts. Wait one minute." |
| Network failure | "Can't reach the API at" the base URL, in the form's alert, the page's error panel, or an error toast |
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
| `CardActions` on an approved card | Request changes renders as a button and Reject under "More actions". Approve does not |
| `CardActions` Approve | The button reads "Approve ae5d441" beside "AI review: Recommend approve" and the short reviewed SHA, with "The head moved after the review." when they differ. "Approve a different SHA" opens the SHA input prefilled with `headSha`, and the POST sends the edited `{ sha }`. With no `headSha` the input shows and the button reads "Approve SHA" (RFC-110) |
| `CardActions` with `retry_review` | "Retry AI review" renders only when listed, and a click POSTs `retry-review` |
| `CardActions` on a blocked card with `unblockTo: null` | No Unblock button. The Intake sentence renders |
| `CardActions` Won't fix | Unblock stays primary and "Won't fix" is secondary. It opens the reason form, "Close as won't fix" is disabled until the reason has text, and the POST to `wont-fix` sends the trimmed `{ reason }` (RFC-112) |
| `CardActions` Reopen | "Reopen" renders with its sentence on a `wont_fix` card and POSTs `reopen` |
| `CardActions` Request changes | The button opens the reason form. A `422` shows `fields.reason` under it. A later submit POSTs `{ reason }` and closes the form |
| `CardNotes` | No notes render nothing. The newest review note renders its bold and code as elements, with "AI review" and "· attempt 2". Earlier notes wait behind "Show 1 earlier note", and a local check note stays plain text in a `pre`. An approver note has no attempt number. An instructions note reads "Instructions" and renders markdown, and a blank one reads "Instructions cleared". Current instructions and the newest other note are both open, and older notes wait behind the button |
| `CardActions` Run | Run opens the instructions box prefilled with `instructions`. Unchanged text POSTs `{}`. A `422` shows `fields.instructions`. Changed text POSTs `{ instructions }` and closes the form |
| `CardInstructions` | No instructions show the empty sentence. Saved instructions render markdown. Edit prefills the box, a `422` shows `fields.instructions`, and Save PUTs `{ instructions }` and shows the returned text |
| `api` run body | `runCard(id)` sends `{}`, `runCard(id, '')` sends `{ instructions: '' }`, and `setInstructions` PUTs to the card's `instructions` route |
| `CardChanges` | A small diff opens with "Lines 67 to 69", the function context, and numbered rows: the removed row on red with "Removed line" and the added row on green with "Added line". The compare link points at `compareUrl`. Collapse all and Expand all toggle every file, and a file button toggles one. Wrap lines switches the code to wrapping. Twelve files start closed, and a link in "Changed files" opens its file. A cut diff shows the warning. A missing commit shows "Commit {short} is not in the project clone yet." (RFC-110) |
| `diff` | Hunk headers parse with and without counts, lines get old and new numbers, a bad header leaves them blank, hunks are labelled by their new or deleted lines, files group by folder in first-seen order, and binary files count no lines |
| `CardPullRequest` | The panel reads "Pull request #7" with the title, branch → base, short SHA, attempt, the rendered description, and "Open on GitHub". A long description toggles with `aria-expanded`. No text shows the empty sentence, or the not-stored sentence with a link when `prUrl` is set. A non-GitHub URL is not linked |
| Card page | The main column starts with Next step, Pull request #7, and Changes. The "Card details" sidebar holds Details, People, and Intake, with SHAs shortened and the full value in `title`. The header links "Open pull request" and "View trace", and Next step leads with "Approve ae5d441". A card with no control says "No action is open for you right now." |
| `IntakeActions` | Ingest and Add requirement render, and no accept button. `toLines` drops blanks, markers, and non-item lines when a list exists, and keeps every line without one. `fromLines` loads saved checks as a `- ` list that saves back unchanged, and a list item added later keeps them |
| `markdown` | `<script>` stays text. `javascript:`, `data:`, relative, and non-GitHub links read as `label (url)`, and a GitHub link parses. An unclosed fence runs to the end. Nested emphasis parses; `**open`, `snake_case_name`, and `2 * 3 * 4` stay text, and so do `#123` and `####### x`. Ordered lists keep `start`, one nested level holds deeper items, and a marker switch starts a new list. Rules, hard breaks, CRLF, and NUL are handled. `plainText` drops markers. 20k `*` or `[` parse within a second. `listItems` keeps only the items of a pasted definition of done, joins wrapped items, keeps nested items, skips fenced code, does not join a label or heading onto an item, and is null without a list || `Markdown` | No `script` or `img` element renders. A GitHub link has `target="_blank"` and `rel="noreferrer noopener"`, and a `javascript:` link has no `<a>`. `# x` is an `h3`. A `pre` keeps its text. `a **b** c` reads "a b c" |
| `RequirementForm` | Statement has 8 rows and Acceptance 6, both growing. Preview shows the formatted heading and bold text, Write brings the textarea back, acceptance previews as a checklist of formatted lines with the list markers dropped and its error kept, a pasted document previews only its list items and counts the lines left out, and an empty statement or acceptance has nothing to preview |
| `requirements` mentions | `mentions` keeps `@tickets` and `(@db)`, drops `a@b.co`, `@scope/pkg`, `@pytest.fixture`, `@Override`, and mentions in backticks and fences, keeps first-use order without repeats, and allows a trailing `.`. `mentionAt` finds the `@` and query before the caret, and is null after a space or inside an email |
| `RequirementForm` mentions | Typing `@ti` opens a listbox with `tickets`, ArrowDown moves `aria-activedescendant`, Enter inserts `@tickets `, and Escape closes the list. With no server the list says so. Chips show a known server, a disabled one with its warning, and an unknown slug with its warning |
| Tribe selector | `/choose-tribe` lists two tribes with their project counts. Picking one stores the tribe and its first project and loads `/`; a tribe with no project stores an empty project. With no tribes a member reads "Ask an admin", and an admin also gets "Create a tribe" |
| Tribe and Project sections | The sidebar headings read "Workspace", "Project · Shop", then "Tribe · Alpha". For a tribe manager the links read Board, Intake, Deploy, Repository, Conventions, Linked projects, Projects, Members, MCP servers, Conventions. Members and MCP servers show only to a tribe manager, and Members is not current on `/tribes/{id}/mcp`. With no project the Project group is gone. The mobile menu shows the headings in the same order |
| Layered conventions pages | The tribe page saves `stackIds`, `skillIds`, and `rulesOff`, and is read only without `canEdit`. The project page shows origins and saves on/off `choices` |
| `CardQuestions` | A `single` question renders radios, a `multiple` one checkboxes, a `text` one a textarea, and "Other" shows the text input only with `allowOther`. The suggestion and why show as text and nothing is preselected. Submit with an unanswered question shows "Answer this question." and sends nothing. A complete submit POSTs `{ answers }` with the chosen options and text. A `422` shows `fields["answers.0"]` under the first question. "Let the agent decide" POSTs `{ useSuggestions: true }`. Earlier questions wait behind the button and show "You", "Agent's suggestion", or "Auto" (RFC-114) |
| `QuestionModeField` | With the `question_mode` action the control PUTs `{ questionMode: 'auto' }` and the hint changes; a failure keeps `manual`. Without the action it reads "Questions: A person answers" (RFC-115) |
| Card page questions | `CardQuestions` replaces `CardActions` in Next step only when `actions` includes `answer`, and Details starts with the answer mode (RFC-114, RFC-115) |
| `PlanModeField` | With `plan_mode` PUTs `{ planMode: 'off' }`; without it reads "Plan: Plan before coding" (RFC-128) |
| `CardPlan` | Reorders and edits steps, Accept POSTs `plan/accept`, Skip when listed, chat Apply updates the checklist only (RFC-129, RFC-130) |
| Card page plan | `CardPlan` replaces Next step when `actions` includes `accept_plan`, ahead of questions (RFC-129) |
| Card context | `CardContext` shows controls only with the `context` action, Detach and Attach call `setCardContext`, and Attach server shows only with `attach-mcp`. The trace renders `context_changed` and the `card` MCP scope |
| Layout tribe scope | The switcher lists only the selected tribe's projects, "Switch tribe" links to `/choose-tribe`, a stored project from another tribe is replaced by the tribe's first project, and a stored tribe the API no longer returns sends the browser to `/choose-tribe` |
| Projects by tribe | An admin with the active tribe `tribe_a` sees only `tribe_a`'s projects and no Tribe filter. With no active tribe the page links to `/choose-tribe`, and an empty tribe says "No projects in this tribe yet." New project starts its tribe select on the active tribe |
| Layout load | With a bearer and no tribe, `/` and `/intake` redirect to `/choose-tribe`, `/admin/users` and `/tribes/new` do not, and no bearer still redirects to `/login`. Signing in goes to `/choose-tribe` |
| Intake unnumbered notice | An intake with `unnumbered` paths shows the notice with the first 5 paths, "and N more", and the Repository settings link. There is no spinner, `getIntake` is not called again after 10 seconds, and "Accept all shown" is enabled |
| Intake pages mentions | `/intake/[id]` shows a parsed row's stored mentions, and `/intake/new` passes `mcpServers` to the form so typing `@` lists them |
| `RequirementForm` refine | Without `refine` there are no icons. Each field has its own 44px icon, and opening one does not open the other. Apply on the statement changes only the statement, and Apply on acceptance changes only the acceptance lines. An edit after asking is flagged, Discard keeps the text, and Preview hides the icons |
| `RefinePanel` | Refine sends the field, the trimmed draft, and the guidance. Deltas appear in order in the live region, done shows the comparison, and Apply calls back with the text. Stop aborts the signal and changes nothing. An error event or a rejected request shows the message and Try again asks again. A blank statement sends nothing. `reset` empties the text and `tool` shows "Looking up tickets / lookup" until the next delta (RFC-095) |
| Tribe MCP pages | The list shows each server with its tool count and a Disabled badge. The form posts slug, transport, URL or image and argv lines, secrets, and the allowlist; a saved secret shows only its name; Discover lists tools with Allow, Update schema, and Name not allowed; a failed discovery shows its message (RFC-092) |
| MCP catalog | Featured and built-in tiles show with "Token needed", "No sign-in", and "Built-in" while the registry is still "Searching the MCP Registry…"; the registry rows then show No network, an unsupported entry's reason with no Set up link, and Load more with the cursor. Typing shows the local matches, aborts the previous registry request, and a registry notice keeps the featured tiles (RFC-099, RFC-100). Postgres from the catalog fills the slug, port, and fixed tools, paints `config.readOnlyLogin`, and posts `config`, `PASSWORD`, and the three tools. A registry entry fills its URL and header names with "Required." A stdio entry's image is pulled and pinned. A new server discovers its tools once without a click. A database server is listed as `postgres · host:port/database` (RFC-096 to RFC-098) |
| `RefineCompare` | The statement shows `<ins>` and `<del>`, acceptance lines are marked unchanged, removed, and added with counts, Before and After render each version, Apply and Discard call back, a stale field is flagged, and unchanged text says so |
| `ChipSelect` | A click opens the listbox with grouped options and a check on the selected one. ArrowDown then Enter picks the next option and calls back, Escape closes with focus on the chip, a letter jumps to the matching option, and a disabled chip does not open |
| `api` `streamRefine` | Events split across chunks parse in order, with the bearer and the project path. A `409` before the stream throws `ApiError` and sends no event. An abort rejects with `AbortError` |
| `api` `streamTrace` | `trace`, `turn`, `delta` split across chunks, `reset`, `turn_end`, and `done` parse in order from the card's `trace/stream` path with `since` and the bearer; a ping comment is skipped. A `429` throws `ApiError`. `parseEvents` keeps the event name when the data has its own `kind` (RFC-088) |
| `textdiff` | A word diff rebuilds both sides, including newlines; identical and empty texts; a line diff marks added and removed lines; a very large change still rebuilds both sides |
| Edit page refine | Refining acceptance and Apply do not save, Save PUTs the new acceptance with the statement unchanged and says it needs acceptance again, and a PRD row has no refine icons |
| `ReviewersPanel` | `canSetSelfReview` is true for a Lead Developer or an admin only. A row with `canSetSelfReview` shows a checked Self review box and Save reviewers. Otherwise a flagged row shows the read-only "Self review: on" line, and an unflagged row says nothing about Self review (RFC-070). A lead's row shows the AI review mode with the saved option pressed, and choosing the other one enables Save reviewers. Others see the read-only line only for a `manual` row (RFC-072) |
| Reviewers before GitHub | `reviewerChoices` labels a member without a login "No verified GitHub login". `/intake/new` sends an unverified member picked as a reviewer. `ReviewersPanel` shows the hint. A `reviewers_requested` row lists the `waiting` reviewers (RFC-103) |
| `intake` filters | Search matches id, statement, heading, source path, and acceptance lines, ignoring case. A markdown statement or acceptance line also matches by its `plainText` words. Status and source filters narrow the list. Only unaccepted rows are offered for acceptance. The status defaults to "Needs acceptance" while any row is unaccepted |
| `filter` | `matchText` ignores case and a blank query matches. `withParams` sets and removes parameters. `pickParam` falls back on an unknown value |
| `board` filters | `ownerChoices` lists each owner once, the signed-in user first in any email case, then by name, then `none` when a card has no owner. `parseOwners` reads the comma list and maps `mine`. Selected owners keep cards of any of them, and `none` keeps unowned cards. Blocked only keeps blocked cards. `wont_fix` cards are dropped unless `wontFix` is set. Search matches title, id, status and its label, requirement id, and the owner's and reviewers' name, login, and email |
| `OwnerFilter` | Clicking an avatar adds its owner and clicking again removes it. Owners past the limit are in the "+n" checklist |
| `status` labels | Every card status has a label in words, `wont_fix` reads "Won't fix", and `waiting_answer` reads "Needs answer". An unknown code reads in sentence case. Reasons such as `owner_github` and `attempt_cap` have labels, and free text is kept as is |
| `usage` lib | Groups by LLM, model, project, user, and task, with "Unknown family" and "No owner". Filters use `-` for a missing value. Options count the rows the other filters keep and keep a chosen value with none. Groups sort by any column either way, ties by name. Daily totals fill every day of the range. The summary's cache share is cached over all input read. `rangeWindow` ends today in UTC |
| Token usage page | With a mocked `getUsage` and URL: the 30-day range loads, the totals, a hidden day table, and models by total render with `aria-sort`. `?llm=-` narrows every total and opens the LLM filter; Add filter opens Model, and picking one sets `model`. `?by=user` regroups and the Group by ChipSelect sets `by`. `?sort=name&dir=desc` reverses names, and header buttons set `sort` and `dir`. 7 days sets `range`. An empty range shows the empty state |
| `usage` source and card | Source groups Card and Intake; Card groups by id with refines as "Intake" and falls back to the id without a title. `source=intake` and `card=-` keep only refines, and card options list each card once, largest first (RFC-091) |
| Admin usage by card | `?by=card` lists Ana card, Intake, and Loose card with the card id as each title, the Group by ChipSelect offers Source and Card, `?source=intake` opens the Source filter and narrows the total to the refine, and Clear filters drops `source` |
| `usage` dimension choices | `dimensionOptionsFor` keeps the allowed dimensions in the usual order |
| Tribe usage page | It reads `getTribeUsage` with the route's tribe id and 30 days, titles the page and the shell "Token usage: Gadget tribe", links back to the tribe, and offers the User filter |
| Account usage page | It reads `getMyUsage`, has no User in Group by or Add filter, and Group by offers LLM, Model, Project, Task, Source, and Card. `?by=user&user=...` is ignored: models group and every row counts |
| User menu | Opening the menu shows "My token usage" linking to `/account/usage` for a member who is not an admin |
| Nav admin items | For an admin the last groups are Admin (Users, Tribes, Runners, Token usage) and Company setup (Company GitHub, LLM APIs, Router, Conventions, Skills). Neither group shows to a member who is not an admin |
| Project overview | `/overview` loads `getProjectSummary` for 30 days, titles "Overview: {project}", shows Ready to run and blocked reasons, and links aging cards to `/cards/{id}` (RFC-127) |
| Tribe overview | `/tribes/[id]/overview` loads `getTribeSummary`, shows the projects table and MCP health, and links back to the tribe and to Token usage |
| Layout Overview links | With a project selected the Project group starts with Overview; a tribe manager also gets Overview under Tribe |
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
| Workflow settings | The page loads `implementationCap` and `soloApproval`, saves both with `putWorkflow`, and shows a `422` on the cap under its field (RFC-126) |
| Repository doc paths | The Docs panel loads `prdPath` and `rfcPath` and saves them with the repository (RFC-126) |
| `FamilyForm` history budget | Every model row has a History budget field on either protocol. `toDraft` and `fromDraft` round-trip `historyBudgetKiB` and drop an empty one |
| `FamilyForm` context window | Every model row has a Context window field on either protocol. `toDraft` and `fromDraft` round-trip `contextTokens` and drop an empty one (RFC-117) |
| Agent page | The Claude Code panel shows only for Claude Code; a family without a key is disabled; `422` field errors show under their controls (RFC-120) |
| Runners agents | The Agents column, and the note when the agent is Claude Code and no online runner has it (RFC-120) |
| `trace` agent words | `agent_run` reads "Claude Code" with the model and turns, red on a non-zero exit (RFC-121) |
| `trace` handoff words | `context_compacted`, `handoff_saved`, and `handoff_applied` have words; a `handoff_applied` with `ok: false` is amber and shows its reason (RFC-117, RFC-118) |
| `SecretField` saved with an empty input | The emitted body omits the secret field |
| `api` on `401` | The bearer is cleared |
| `api` AI review mode | `addRequirement` sends `aiReviewMode` in the POST body, as the New requirement page does, and `setAIReviewMode` PUTs `{ aiReviewMode }` to `requirements/{id}/ai-review-mode` in the selected project (RFC-072) |
| `stream` | `createLive` opens one stream for the project, is `connected` after `ready`, hands frames to every handler, reconnects after 1 second doubling to 30 with a `resync` first, reconnects at once on `wake`, closes on `stop` and on a project change, and drops a handler after `off` (RFC-113) |
| Board live | With a mocked stream: a `cards` frame replaces one card and drops a `removed` id, a `full` frame replaces the list, a `board` frame updates "Run ready cards (N)", and no `GET /api/cards` poll runs while connected (RFC-113) |
| Board Needs answer | A `waiting_answer` card in group `question` shows in the "Needs answer" lane, which sits between Waiting and Running (RFC-116) |
| Card page live | A `cards` frame naming the card reloads it quietly, and a frame for another card does not |
| Layout load `?project=` | `/cards/x?project=proj_b` stores `proj_b` and its tribe from `GET /api/projects`; an unknown id changes nothing |
| `NotificationBell` | The label counts unread, opening lists rows with unread marked, the panel lists at most 8 and links "Open all notifications" to `/notifications`, a row links to its path and marks it read, "Mark all read" posts `{ all: true }`, a `notification` frame raises the count and shows the toast, and a URL that is not a single-slash path links to `/` (RFC-113) |
| Notifications page | Rows are grouped by day, newest first; Unread reads `?unread=1`; opening a row marks it read and lowers the shared count; "Mark all read" posts `{ all: true }` and disables itself; an off-site URL links to `/`; an empty list says "No notifications yet" (RFC-113) |
| `push` | `enablePush` subscribes with the 65-byte server key and `userVisibleOnly`, saves the subscription, and replaces one made with another key. A denied permission is `blocked` and subscribes nothing. `syncPush` sends a held subscription again. `disablePush` unsubscribes and deletes the endpoint (RFC-113) |
| `NotificationsPanel` | Off shows "Turn on", which turns it On with "Send a test" and "Turn off". A test reports a send, or that no browser took it, in a toast. Blocked shows the site settings hint and no button. Unsupported says so (RFC-113) |
| `toast` | `success` and `info` close after 5 seconds and `error` after 8; `pause` holds the timer and `resume` restarts it; a fourth toast drops the oldest; the same text and tone as the newest toast adds nothing; `dismiss` removes one (REQ-096) |
| `Toaster` | Success and info render in the `status` region and errors in the `alert` region, both present while empty; a toast with `href` shows its link; "Dismiss" removes it (REQ-096) |
| `Page` states | `loading` shows "Loading {thing}" in a `role="status"` row instead of the body; `error` shows the message and "Try again", which calls `onretry` (REQ-096) |
| `forms` | `formFailure` returns the message and fields of a `422`, lists fields not in `shown` as `extra`, and turns a network error into "Can't reach the API at" the origin; `focusInvalid` focuses the first `aria-invalid` control, else the `[role=alert]` (REQ-096) |
| `FormAlert` | Shows the message and one line per extra field, has `role="alert"` and `tabindex="-1"`, and renders nothing when both are empty (REQ-096) |
| `Field` error | The error paragraph follows the control with id `{id}-error`; the control has `aria-invalid="true"` and `aria-describedby` naming the hint and the error (REQ-096) |
| Login page | An empty submit shows "Enter your email" and focuses email without a request; a `401` shows "Email or password is wrong." in the alert, clears the password, and focuses the alert; a `429` shows the wait message (REQ-096) |
| `Button` | `icon` renders an `aria-hidden` svg before the label without changing the name; `loading` disables the button, sets `aria-busy`, and keeps the label; `aria-label` passes through (REQ-096) |
| Built `build/index.html` | Contains a CSP meta tag with `script-src 'self'` and a hash, and no `unsafe-inline` |
