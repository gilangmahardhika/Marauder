<script lang="ts">
  import { ApiError, getLLM, unreachable } from '$lib/api';
  import { protocolLabel } from '$lib/llm';
  import { shell, showShell } from '$lib/shell.svelte';
  import type { Family } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Table from '$lib/ui/Table.svelte';

  let families = $state<Family[]>([]);
  let loaded = $state(false);
  let message = $state('Loading LLM settings');

  showShell(() => ({ title: 'LLM APIs', message }));

  $effect(() => {
    if (shell.projectId === '') return;
    void getLLM()
      .then((body) => {
        families = body.families;
        loaded = true;
        message = 'LLM APIs';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  function models(list: string[]) {
    if (list.length === 0) return '—';
    const shown = list.slice(0, 2).join(', ');
    return list.length > 2 ? `${shown} +${list.length - 2} more` : shown;
  }
</script>

<Page
  title="LLM APIs"
  description="Model families this project can route work to. Each family has one endpoint, one key, and any number of models."
  width="wide"
>
  {#snippet actions()}
    <Button variant="primary" href="/settings/llm/new">Add family</Button>
  {/snippet}
  {#if loaded && families.length === 0}
    <EmptyState title="No LLM families yet" description="Add a family for each provider endpoint, then list the models it serves.">
      <Button variant="primary" href="/settings/llm/new">Add family</Button>
    </EmptyState>
  {:else if families.length > 0}
    <Table label="LLM families">
      <thead>
        <tr class="border-b border-line bg-ground/50">
          <th class={th} scope="col">Family</th>
          <th class={th} scope="col">Models</th>
          <th class={th} scope="col">Key</th>
          <th class={th} scope="col"><span class="sr-only">Edit</span></th>
        </tr>
      </thead>
      <tbody>
        {#each families as family (family.id)}
          <tr class={tr}>
            <td class="{td} py-2.5">
              <span class="block break-all font-mono text-[13px] font-medium text-ink">{family.id}</span>
              <span class="mt-0.5 block break-all text-xs text-muted">
                {protocolLabel(family.protocol)} · <span class="font-mono">{family.baseUrl || 'no base URL'}</span>
              </span>
            </td>
            <td class="{td} font-mono text-xs text-muted">{models(family.models)}</td>
            <td class={td}>
              {#if family.keySet}<Badge tone="green">Saved</Badge>{:else}<Badge>Not set</Badge>{/if}
            </td>
            <td class="{td} text-right">
              <a
                class="inline-flex min-h-11 items-center px-2 text-sm font-medium text-accent hover:underline"
                href={`/settings/llm/${encodeURIComponent(family.id)}`}
                aria-label={`Edit ${family.id}`}>Edit</a
              >
            </td>
          </tr>
        {/each}
      </tbody>
    </Table>
  {/if}
</Page>
