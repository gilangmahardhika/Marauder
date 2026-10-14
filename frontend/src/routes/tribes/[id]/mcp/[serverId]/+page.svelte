<script lang="ts">
  import { page } from '$app/stores';
  import { getMCPServers } from '$lib/api';
  import MCPServerForm from '$lib/components/MCPServerForm.svelte';
  import { failureText } from '$lib/forms';
  import { showShell } from '$lib/shell.svelte';
  import type { MCPServer } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Page from '$lib/ui/Page.svelte';

  let server = $state<MCPServer | null>(null);
  let loading = $state('Loading MCP server');
  let loadError = $state('');
  const id = $derived($page.params.id ?? '');
  const serverId = $derived($page.params.serverId ?? '');
  const autoDiscover = $derived($page.url.searchParams.get('discover') === '1');
  const title = $derived(server ? `MCP server: ${server.slug}` : 'MCP server');

  showShell(() => ({ title }));

  async function load(tribeId: string, wanted: string) {
    loading = 'Loading MCP server';
    loadError = '';
    try {
      const body = await getMCPServers(tribeId);
      server = body.servers.find((item) => item.id === wanted) ?? null;
      if (!server) loadError = 'MCP server not found. It may have been deleted.';
    } catch (error) {
      loadError = failureText(error);
    } finally {
      loading = '';
    }
  }

  $effect(() => {
    if (id && serverId) void load(id, serverId);
  });
</script>

<Page
  {title}
  description="Changes apply to jobs that start after you save."
  {loading}
  error={loadError}
  onretry={() => void load(id, serverId)}
>
  {#snippet actions()}
    <Button href={`/tribes/${id}/mcp`} icon="arrow-left">All servers</Button>
  {/snippet}
  {#if server}
    {#key server.id}
      <MCPServerForm tribeId={id} {server} {autoDiscover} />
    {/key}
  {/if}
</Page>
