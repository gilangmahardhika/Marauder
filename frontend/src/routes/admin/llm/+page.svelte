<script lang="ts">
  import { page } from '$app/stores';
  import { getLLM } from '$lib/api';
  import { pickParam } from '$lib/filter';
  import { failureText } from '$lib/forms';
  import { filterFamilies, keyFilters, keyOptions, protocolFilters, protocolLabel, protocolOptions } from '$lib/llm';
  import { setParams } from '$lib/params';
  import { showShell } from '$lib/shell.svelte';
  import type { Family } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';
  import Table from '$lib/ui/Table.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  let families = $state<Family[]>([]);
  let loaded = $state(false);
  let loading = $state('Loading LLM APIs');
  let loadError = $state('');

  showShell(() => ({ title: 'LLM APIs' }));

  const params = $derived($page.url.searchParams);
  const filter = $derived({
    q: params.get('q') ?? '',
    protocol: pickParam(params.get('protocol'), protocolFilters, 'all'),
    key: pickParam(params.get('key'), keyFilters, 'all')
  });
  const shown = $derived(filterFamilies(families, filter));

  function load() {
    loading = 'Loading LLM APIs';
    loadError = '';
    void getLLM()
      .then((body) => {
        families = body.families;
        loaded = true;
      })
      .catch((error: unknown) => {
        loadError = failureText(error);
      })
      .finally(() => {
        loading = '';
      });
  }

  $effect(() => {
    load();
  });

  function models(list: string[]) {
    if (list.length === 0) return '—';
    const shown = list.slice(0, 2).join(', ');
    return list.length > 2 ? `${shown} +${list.length - 2} more` : shown;
  }
</script>

<Page
  title="LLM APIs"
  description="Model families every project can route work to. Each family has one endpoint, one key, and any number of models."
  {loading}
  error={loadError}
  onretry={load}
>
  {#snippet actions()}
    <Button variant="primary" href="/admin/llm/new" icon="plus">Add family</Button>
  {/snippet}
  {#if loaded && families.length === 0}
    <EmptyState title="No LLM families yet" description="Add a family for each provider endpoint, then list the models it serves.">
      <Button variant="primary" href="/admin/llm/new" icon="plus">Add family</Button>
    </EmptyState>
  {:else if families.length > 0}
    <div class="flex flex-col gap-4">
      <Toolbar>
        <SearchBox label="Search LLM families" value={filter.q} placeholder="Search family, base URL, or model" />
        <Segmented
          label="Protocol"
          options={protocolOptions(families)}
          value={filter.protocol}
          onchange={(protocol) => setParams({ protocol: protocol === 'all' ? '' : protocol })}
        />
        <Segmented label="Key" options={keyOptions(families)} value={filter.key} onchange={(key) => setParams({ key: key === 'all' ? '' : key })} />
      </Toolbar>
      {#if shown.length === 0}
        <EmptyState title="No families match" description="Change the search or filters to see more families.">
          <Button type="button" variant="secondary" icon="close" onclick={() => setParams({ q: '', protocol: '', key: '' })}>Clear filters</Button>
        </EmptyState>
      {:else}
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
            {#each shown as family (family.id)}
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
                    href={`/admin/llm/${encodeURIComponent(family.id)}`}
                    aria-label={`Edit ${family.id}`}>Edit</a
                  >
                </td>
              </tr>
            {/each}
          </tbody>
        </Table>
      {/if}
    </div>
  {/if}
</Page>
