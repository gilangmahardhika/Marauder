<script lang="ts">
  import { mentionState } from '$lib/requirements';
  import type { MCPServerMention } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';

  let { mentions, servers }: { mentions: string[]; servers: MCPServerMention[] } = $props();
</script>

{#if mentions.length > 0}
  <ul aria-label="MCP servers mentioned" class="flex flex-wrap gap-x-3 gap-y-1.5">
    {#each mentions as slug (slug)}
      {@const state = mentionState(slug, servers)}
      <li class="flex items-center gap-1.5 text-xs">
        <Badge tone={state === 'known' ? 'default' : state === 'disabled' ? 'amber' : 'red'}>
          <span class="font-mono">@{slug}</span>
        </Badge>
        {#if state === 'disabled'}
          <span class="text-amber-800">disabled: jobs will not call it</span>
        {:else if state === 'unknown'}
          <span class="text-red-800">no server with this slug</span>
        {/if}
      </li>
    {/each}
  </ul>
{/if}
