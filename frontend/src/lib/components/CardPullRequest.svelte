<script lang="ts">
  import type { CardPullRequest } from '$lib/types';
  import Icon from '$lib/ui/Icon.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Markdown from './Markdown.svelte';

  let {
    pullRequest,
    prUrl,
    branch,
    base = ''
  }: { pullRequest: CardPullRequest | null; prUrl: string | null; branch: string | null; base?: string } = $props();

  /** A longer description starts clamped behind "Show full description". */
  const clampAt = 1200;

  let expanded = $state(false);

  const link = $derived(prUrl && (prUrl.startsWith('https://github.com/') || prUrl.startsWith('http://127.0.0.1')) ? prUrl : '');
  const number = $derived(prUrl?.match(/\/pull\/(\d+)/)?.[1] ?? '');
  const long = $derived((pullRequest?.body.length ?? 0) > clampAt);
</script>

{#snippet openLink()}
  {#if link}
    <a class="inline-flex min-h-11 items-center gap-1.5 text-sm font-medium text-accent hover:underline" href={link}>
      <Icon name="link" class="size-4" />Open on GitHub
    </a>
  {/if}
{/snippet}

<Panel title={number ? `Pull request #${number}` : 'Pull request'} actions={link ? openLink : undefined}>
  {#if pullRequest}
    <div class="flex flex-col gap-2">
      <h3 class="break-words text-lg font-semibold leading-snug text-ink">{pullRequest.title}</h3>
      <p class="flex flex-wrap items-center gap-x-3 gap-y-1 text-[13px] text-muted">
        {#if branch}
          <span class="inline-flex items-center gap-1.5">
            <Icon name="git" class="size-4" />
            <span class="font-mono text-ink">{branch}</span>
            {#if base}<span aria-hidden="true">→</span><span class="sr-only">into</span><span class="font-mono text-ink">{base}</span>{/if}
          </span>
        {/if}
        {#if pullRequest.sha}
          <span>Commit <span class="font-mono text-ink" title={pullRequest.sha}>{pullRequest.sha.slice(0, 7)}</span></span>
        {/if}
        {#if pullRequest.attemptN > 0}<span>Attempt {pullRequest.attemptN}</span>{/if}
      </p>
    </div>
    {#if pullRequest.body}
      <div class="relative border-t border-line pt-4">
        <div id="pr-description" class={long && !expanded ? 'max-h-80 overflow-hidden' : ''}>
          <Markdown text={pullRequest.body} headingBase={4} />
        </div>
        {#if long && !expanded}
          <div class="pointer-events-none absolute inset-x-0 bottom-0 h-16 bg-linear-to-t from-surface"></div>
        {/if}
      </div>
      {#if long}
        <button
          type="button"
          class="inline-flex min-h-11 w-fit items-center gap-1.5 text-sm font-medium text-accent hover:underline"
          aria-expanded={expanded}
          aria-controls="pr-description"
          onclick={() => (expanded = !expanded)}
        >
          <Icon name="chevron" class="size-4 transition-transform {expanded ? 'rotate-180' : ''}" />
          {expanded ? 'Show less' : 'Show full description'}
        </button>
      {/if}
    {:else}
      <p class="text-sm text-muted">The pull request has no description.</p>
    {/if}
  {:else if prUrl}
    <p class="text-sm text-muted">
      This card published before the orchestrator stored pull request text. Its title and description are on GitHub.
    </p>
  {:else}
    <p class="text-sm text-muted">No pull request yet. The title and description show here after the card publishes.</p>
  {/if}
</Panel>
