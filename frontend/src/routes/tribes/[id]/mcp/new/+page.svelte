<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getMCPCatalogEntry, unreachable } from '$lib/api';
  import MCPServerForm from '$lib/components/MCPServerForm.svelte';
  import { showShell } from '$lib/shell.svelte';
  import type { MCPCatalogEntry } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Page from '$lib/ui/Page.svelte';

  let message = $state('New MCP server');
  let entry = $state<MCPCatalogEntry | null>(null);
  let ready = $state(false);
  const id = $derived($page.params.id ?? '');
  const from = $derived($page.url.searchParams.get('from') ?? '');

  showShell(() => ({ title: 'New MCP server', message }));

  async function load(tribeId: string, entryId: string) {
    try {
      entry = await getMCPCatalogEntry(tribeId, entryId);
      if (entry) message = `Set up ${entry.title}`;
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      ready = true;
    }
  }

  $effect(() => {
    if (!id) return;
    if (from) void load(id, from);
    else ready = true;
  });
</script>

<Page
  title={entry ? `Set up ${entry.title}` : 'New MCP server'}
  description="Reach a server over HTTPS, run one in the sandbox image with no network, or read a database."
>
  {#snippet actions()}
    <Button href={`/tribes/${id}/mcp/catalog`}>Catalog</Button>
    <Button href={`/tribes/${id}/mcp`}>All servers</Button>
  {/snippet}
  {#if ready}
    {#key entry?.id}
      <MCPServerForm tribeId={id} {entry} onmessage={(text) => (message = text)} />
    {/key}
  {/if}
</Page>
