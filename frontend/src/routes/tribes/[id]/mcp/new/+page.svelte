<script lang="ts">
  import { page } from '$app/stores';
  import { getMCPCatalogEntry } from '$lib/api';
  import MCPServerForm from '$lib/components/MCPServerForm.svelte';
  import { failureText } from '$lib/forms';
  import { showShell } from '$lib/shell.svelte';
  import type { MCPCatalogEntry } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Page from '$lib/ui/Page.svelte';

  let entry = $state<MCPCatalogEntry | null>(null);
  let loading = $state($page.url.searchParams.get('from') ? 'Loading catalog entry' : '');
  let loadError = $state('');
  const id = $derived($page.params.id ?? '');
  const from = $derived($page.url.searchParams.get('from') ?? '');
  const title = $derived(entry ? `Set up ${entry.title}` : 'New MCP server');

  showShell(() => ({ title }));

  async function load(tribeId: string, entryId: string) {
    loading = 'Loading catalog entry';
    loadError = '';
    try {
      entry = await getMCPCatalogEntry(tribeId, entryId);
    } catch (error) {
      loadError = failureText(error);
    } finally {
      loading = '';
    }
  }

  $effect(() => {
    if (id && from) void load(id, from);
  });
</script>

<Page
  {title}
  description="Reach a server over HTTPS, run one in the sandbox image with no network, or read a database."
  {loading}
  error={loadError}
  onretry={() => void load(id, from)}
>
  {#snippet actions()}
    <Button href={`/tribes/${id}/mcp/catalog`}>Catalog</Button>
    <Button href={`/tribes/${id}/mcp`} icon="arrow-left">All servers</Button>
  {/snippet}
  {#key entry?.id}
    <MCPServerForm tribeId={id} {entry} />
  {/key}
</Page>
