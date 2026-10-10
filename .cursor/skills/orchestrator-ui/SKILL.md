---
name: orchestrator-ui
description: Builds the orchestrator SvelteKit client. Use when editing frontend pages, components, api.ts, session handling, settings forms, or board and card actions.
---

# Orchestrator UI

Spec: `docs/tech/0004-frontend.md`. The client calls `http://127.0.0.1:8787`. It does not decide card status.

## App shape

SvelteKit 2, Svelte 5, `ssr` and `prerender` false, `@sveltejs/adapter-static` with `fallback: 'index.html'`. Dev and preview bind `127.0.0.1:4173`. No `+server.ts`, no `hooks.server.ts`, no form actions, no React, no runtime npm dependencies beyond the toolchain listed in the guide.

CSP stays hash mode. Do not add `'unsafe-inline'` or `'unsafe-eval'`. Put styles in `src/app.css`.

## Data

`src/lib/api.ts` has one function per route in `docs/tech/0002-data-and-api.md`. It sends the bearer, parses `error` into `ApiError`, and on `401` clears `sessionStorage` key `orchestrator.bearer` and goes to `/login`. It does not log bodies. Login is email and password. There is no UI token field.

`src/lib/types.ts` matches the JSON. A new API field is added there in the same change.

`sessionStorage` holds the bearer and the selected project id (`orchestrator.project`). GitHub tokens, passwords, model keys, `databaseUrl`, `tokenKey`, and the deploy token are not stored in the browser. Card buttons still come only from `actions`. Admin links come from `GET /api/me`, not from a client-side role matrix.

## Pages

Render a control only when its name is in `actions`. Do not also check `status`.

```svelte
<!-- BAD -->
{#if card.status === 'human_review'}

<!-- GOOD -->
{#if card.actions.includes('approve')}
```

Use `{value}` for API strings. `{@html}` is forbidden. A link is allowed only when the URL starts with `https://github.com/` or `http://127.0.0.1`.

`SecretField` omits an empty secret from the JSON. A saved secret shows the placeholder "A value is saved" and is never copied into the live region or `console`.

`Live.svelte` is one `<p role="status">`. Set it to the page name while loading and to the action name while a submit is in flight. Disable the submit button until the request finishes.

On `409`, reload the card and set the live message to "Card changed. Reloaded." On a failed `fetch`, show "API unreachable at http://127.0.0.1:8787". Do not invent cards.

The board poll is a 3 second interval only while work is running. Clear it when the page is destroyed.

## Style

The product name in the shell is Marauder. Tailwind CSS 4 is a dev dependency, imported from `src/app.css`, with the theme tokens in that file. Shared controls live in `src/lib/ui/`. One primary button per page. `.danger` only for blocked status. Inputs and buttons are at least 44px tall. Keep `:focus-visible` outlines. Board groups become one column under 720px. Do not add a runtime CSS library or React.
