<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getMCPServers, unreachable } from '$lib/api';
  import MCPServerForm from '$lib/components/MCPServerForm.svelte';
  import { showShell } from '$lib/shell.svelte';
  import type { MCPServer } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';

  let server = $state<MCPServer | null>(null);
  let missing = $state(false);
  let message = $state('Loading MCP server');
  const id = $derived($page.params.id ?? '');
  const serverId = $derived($page.params.serverId ?? '');
  const autoDiscover = $derived($page.url.searchParams.get('discover') === '1');
  const title = $derived(server ? `MCP server: ${server.slug}` : 'MCP server');

  showShell(() => ({ title, message }));

  async function load(tribeId: string, wanted: string) {
    try {
      const body = await getMCPServers(tribeId);
      server = body.servers.find((item) => item.id === wanted) ?? null;
      missing = server === null;
      message = server ? title : 'MCP server not found';
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    }
  }

  $effect(() => {
    if (id && serverId) void load(id, serverId);
  });
</script>

<Page {title} description="Changes apply to jobs that start after you save.">
  {#snippet actions()}
    <Button href={`/tribes/${id}/mcp`}>All servers</Button>
  {/snippet}
  {#if server}
    {#key server.id}
      <MCPServerForm tribeId={id} {server} {autoDiscover} onmessage={(text) => (message = text)} />
    {/key}
  {:else if missing}
    <EmptyState title="MCP server not found" description="It may have been deleted." />
  {/if}
</Page>
