<script lang="ts">
  import { page } from '$app/stores';
  import { getMCPServers, getTribes } from '$lib/api';
  import { failureText } from '$lib/forms';
  import { showShell } from '$lib/shell.svelte';
  import type { MCPServer } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Table from '$lib/ui/Table.svelte';

  let tribeName = $state('');
  let servers = $state<MCPServer[]>([]);
  let loaded = $state(false);
  let loading = $state('Loading MCP servers');
  let loadError = $state('');
  const id = $derived($page.params.id ?? '');
  const title = $derived(tribeName ? `MCP servers: ${tribeName}` : 'MCP servers');

  showShell(() => ({ title }));

  async function load(tribeId: string) {
    loading = 'Loading MCP servers';
    loadError = '';
    try {
      const [body, tribes] = await Promise.all([getMCPServers(tribeId), getTribes()]);
      servers = body.servers;
      tribeName = tribes.tribes.find((tribe) => tribe.id === tribeId)?.name ?? '';
      loaded = true;
    } catch (error) {
      loadError = failureText(error);
    } finally {
      loading = '';
    }
  }

  $effect(() => {
    if (id) void load(id);
  });

  function reachedAt(server: MCPServer): string {
    if (server.config) return `${server.transport} · ${server.config.host}:${server.config.port}/${server.config.database}`;
    return server.transport === 'http' ? server.url : `stdio · ${server.argv[0] ?? ''}`;
  }
</script>

<Page
  {title}
  description="Read-only tools the implementer and requirement refine of this tribe's projects can call."
  {loading}
  error={loadError}
  onretry={() => void load(id)}
>
  {#snippet actions()}
    <Button href={`/tribes/${id}`} icon="arrow-left">Back to tribe</Button>
    <Button href={`/tribes/${id}/mcp/new`} icon="plus">Add manually</Button>
    <Button variant="primary" href={`/tribes/${id}/mcp/catalog`}>Browse catalog</Button>
  {/snippet}
  {#if servers.length > 0}
    <Table label="MCP servers">
      <thead>
        <tr class="border-b border-line bg-ground/50">
          <th class={th} scope="col">Server</th>
          <th class={th} scope="col">Reached at</th>
          <th class={th} scope="col">Tools</th>
          <th class={th} scope="col"><span class="sr-only">Actions</span></th>
        </tr>
      </thead>
      <tbody>
        {#each servers as server (server.id)}
          <tr class={tr}>
            <td class={td}>
              <span class="flex items-center gap-2">
                <span class="font-mono text-ink">{server.slug}</span>
                {#if !server.enabled}<Badge>Disabled</Badge>{/if}
              </span>
            </td>
            <td class="{td} break-all font-mono text-xs text-muted">
              {reachedAt(server)}
            </td>
            <td class="{td} text-muted">{server.tools.length}</td>
            <td class="{td} text-right">
              <a
                class="inline-flex min-h-11 items-center px-2 text-sm text-accent hover:underline"
                href={`/tribes/${id}/mcp/${server.id}`}>Edit</a
              >
            </td>
          </tr>
        {/each}
      </tbody>
    </Table>
  {:else if loaded}
    <EmptyState title="No MCP servers" description="Pick a server or database from the catalog, then allow the read-only tools jobs may call." />
  {/if}
</Page>
