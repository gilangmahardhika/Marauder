<script lang="ts">
  import { page } from '$app/stores';
  import { getMCPCatalog } from '$lib/api';
  import { failureText } from '$lib/forms';
  import { showShell } from '$lib/shell.svelte';
  import type { MCPCatalogEntry } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Field from '$lib/ui/Field.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Page from '$lib/ui/Page.svelte';

  const debounceMs = 300;
  const kinds: Record<string, string> = { postgres: 'Database', sqlserver: 'Database', http: 'HTTP', stdio: 'Stdio' };
  const heading = 'text-[11px] font-semibold uppercase tracking-wider text-muted';

  let q = $state('');
  let local = $state<MCPCatalogEntry[]>([]);
  let localError = $state('');
  let registry = $state<MCPCatalogEntry[]>([]);
  let nextCursor = $state('');
  let registryError = $state('');
  let registryBusy = $state(true);
  let registryLoaded = $state(false);
  let first = true;
  let current: AbortController | null = null;
  const id = $derived($page.params.id ?? '');
  const featured = $derived(local.filter((e) => e.source === 'featured'));
  const builtins = $derived(local.filter((e) => e.source === 'builtin'));

  showShell(() => ({ title: 'MCP catalog' }));

  async function loadLocal(tribeId: string, query: string, signal: AbortSignal) {
    try {
      const body = await getMCPCatalog(tribeId, 'local', query, '', signal);
      if (signal.aborted) return;
      local = body.entries;
      localError = '';
    } catch (error) {
      if (!signal.aborted) localError = failureText(error);
    }
  }

  async function loadRegistry(tribeId: string, query: string, cursor: string, signal: AbortSignal) {
    registryBusy = true;
    try {
      const body = await getMCPCatalog(tribeId, 'registry', query, cursor, signal);
      if (signal.aborted) return;
      registry = cursor ? [...registry, ...body.entries] : body.entries;
      nextCursor = body.nextCursor;
      registryError = body.registryError ?? '';
    } catch (error) {
      if (signal.aborted) return;
      registryError = failureText(error);
    } finally {
      if (!signal.aborted) {
        registryBusy = false;
        registryLoaded = true;
      }
    }
  }

  function more() {
    if (current) void loadRegistry(id, q.trim(), nextCursor, current.signal);
  }

  function monogram(title: string) {
    return title.replace(/[^A-Za-z0-9]/g, '').slice(0, 2).toUpperCase() || 'M';
  }

  function auth(entry: MCPCatalogEntry): { tone: 'green' | 'amber' | 'default'; label: string } {
    if (entry.secrets.some((s) => s.required)) return { tone: 'amber', label: 'Token needed' };
    if (entry.secrets.length > 0) return { tone: 'default', label: 'Optional token' };
    return { tone: 'green', label: 'No sign-in' };
  }

  function setupHref(entry: MCPCatalogEntry) {
    return `/tribes/${id}/mcp/new?from=${encodeURIComponent(entry.id)}`;
  }

  $effect(() => {
    const tribeId = id;
    const query = q.trim();
    if (!tribeId) return;
    const controller = new AbortController();
    current = controller;
    registryBusy = true;
    const delay = first ? 0 : debounceMs;
    first = false;
    const timer = setTimeout(() => {
      void loadLocal(tribeId, query, controller.signal);
      void loadRegistry(tribeId, query, '', controller.signal);
    }, delay);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  });
</script>

{#snippet tile(entry: MCPCatalogEntry)}
  <li class="flex flex-col rounded-control border border-line bg-surface hover:border-line-strong hover:shadow-sm">
    <div class="flex flex-1 gap-3 p-4">
      <span class="flex size-10 shrink-0 items-center justify-center rounded-control bg-sidebar text-sm font-semibold text-ink" aria-hidden="true"
        >{monogram(entry.title)}</span
      >
      <div class="flex min-w-0 flex-col gap-1">
        <h3 class="text-sm font-semibold text-ink">{entry.title}</h3>
        <p class="line-clamp-2 text-sm text-muted">{entry.description}</p>
        {#if entry.websiteUrl}
          <a class="inline-flex min-h-11 w-fit items-center text-sm text-accent hover:underline" href={entry.websiteUrl} rel="noreferrer noopener" target="_blank"
            >Website <span class="sr-only">of {entry.title}</span></a
          >
        {/if}
      </div>
    </div>
    <div class="flex items-center justify-between gap-2 border-t border-line px-4 py-2">
      <span class="flex flex-wrap gap-1.5">
        {#if entry.source === 'builtin'}
          <Badge tone="sky">Built-in</Badge>
          <Badge>Database</Badge>
        {:else}
          {@const a = auth(entry)}
          <Badge tone={a.tone}>{a.label}</Badge>
          <Badge>{kinds[entry.transport] ?? entry.transport}</Badge>
        {/if}
      </span>
      <Button href={setupHref(entry)}>Set up <span class="sr-only">{entry.title}</span></Button>
    </div>
  </li>
{/snippet}

<Page title="MCP catalog" description="Featured servers and built-in database connectors, then the official MCP Registry.">
  {#snippet actions()}
    <Button href={`/tribes/${id}/mcp/new`} icon="plus">Add manually</Button>
    <Button href={`/tribes/${id}/mcp`} icon="arrow-left">All servers</Button>
  {/snippet}
  <div class="flex flex-col gap-8">
    <Field label="Search" id="mcp-catalog-q" width="lg" placeholder="github, docs, postgres, payments" bind:value={q} />
    {#if localError}
      <Notice tone="danger">{localError}</Notice>
    {/if}

    {#if featured.length > 0}
      <section class="flex flex-col gap-3" aria-labelledby="mcp-featured">
        <h2 id="mcp-featured" class={heading}>Featured</h2>
        <ul class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3" aria-label="Featured servers">
          {#each featured as entry (entry.id)}
            {@render tile(entry)}
          {/each}
        </ul>
      </section>
    {/if}

    {#if builtins.length > 0}
      <section class="flex flex-col gap-3" aria-labelledby="mcp-databases">
        <h2 id="mcp-databases" class={heading}>Databases</h2>
        <ul class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3" aria-label="Database connectors">
          {#each builtins as entry (entry.id)}
            {@render tile(entry)}
          {/each}
        </ul>
      </section>
    {/if}

    <section class="flex flex-col gap-3" aria-labelledby="mcp-registry" aria-busy={registryBusy}>
      <div class="flex flex-wrap items-baseline justify-between gap-3">
        <h2 id="mcp-registry" class={heading}>MCP Registry</h2>
        <p class="text-xs text-muted" role="status">
          {registryBusy ? 'Searching the MCP Registry…' : registryLoaded && !registryError ? `${registry.length} ${registry.length === 1 ? 'server' : 'servers'}` : ''}
        </p>
      </div>
      {#if registryError && !registryBusy}
        <Notice tone="warning">{registryError}. The featured and built-in servers are still listed. Try again in a minute.</Notice>
      {/if}
      {#if registryBusy && registry.length === 0}
        <ul class="flex flex-col gap-3" aria-hidden="true">
          {#each [0, 1, 2] as n (n)}
            <li class="h-20 rounded-control border border-line bg-surface motion-safe:animate-pulse"></li>
          {/each}
        </ul>
      {:else if registry.length > 0}
        <ul class="flex flex-col gap-3 {registryBusy ? 'opacity-60' : ''}" aria-label="Registry servers">
          {#each registry as entry (entry.id)}
            <li class="flex flex-col gap-3 rounded-control border border-line bg-surface p-4 sm:flex-row sm:items-start sm:justify-between">
              <div class="flex min-w-0 flex-col gap-1.5">
                <div class="flex flex-wrap items-center gap-2">
                  <h3 class="text-sm font-semibold text-ink">{entry.title}</h3>
                  <Badge>{kinds[entry.transport] ?? entry.transport}</Badge>
                  {#if entry.supported && entry.note}<Badge tone="amber">No network</Badge>{/if}
                </div>
                <span class="break-all font-mono text-xs text-muted">{entry.name}{entry.version ? ` ${entry.version}` : ''}</span>
                {#if entry.description}<p class="text-sm text-muted">{entry.description}</p>{/if}
                {#if !entry.supported && entry.reason}<p class="text-xs text-muted">Cannot be set up: {entry.reason}</p>{/if}
                {#if entry.websiteUrl}
                  <a class="inline-flex min-h-11 w-fit items-center text-sm text-accent hover:underline" href={entry.websiteUrl} rel="noreferrer noopener" target="_blank"
                    >Website <span class="sr-only">of {entry.title}</span></a
                  >
                {/if}
              </div>
              {#if entry.supported}
                <div class="shrink-0">
                  <Button href={setupHref(entry)}>Set up <span class="sr-only">{entry.title}</span></Button>
                </div>
              {/if}
            </li>
          {/each}
        </ul>
        {#if nextCursor}
          <div>
            <Button disabled={registryBusy} onclick={more}>Load more</Button>
          </div>
        {/if}
      {:else if registryLoaded && !registryError && q.trim()}
        <p class="text-sm text-muted">No registry servers match “{q.trim()}”.</p>
      {/if}
    </section>

    {#if local.length === 0 && registry.length === 0 && registryLoaded && !registryBusy && !registryError && !localError}
      <EmptyState title="No servers found" description="Try another word, or add a server manually." />
    {/if}
  </div>
</Page>
