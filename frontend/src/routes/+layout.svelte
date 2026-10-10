<script lang="ts">
  import { page } from '$app/stores';
  import type { Snippet } from 'svelte';
  import '../app.css';

  let { children }: { children: Snippet } = $props();
</script>

<a class="skip" href="#main">Skip to content</a>
{#if $page.url.pathname !== '/login'}
  <nav class="nav" aria-label="Sections">
    <a href="/">Board</a>
    <a href="/intake">Intake</a>
    <a href="/settings/github">GitHub</a>
    <a href="/settings/llm">LLM APIs</a>
    <a href="/settings/router">Router</a>
    <a href="/settings/deploy">Deploy</a>
  </nav>
{/if}
<main id="main" class="wrap">
  {@render children()}
</main>
