<script lang="ts">
  import { page } from '$app/stores';
  import { getMembers, getTribes } from '$lib/api';
  import { matchText } from '$lib/filter';
  import { failureText } from '$lib/forms';
  import { setParams } from '$lib/params';
  import { canManageTribe, shell, showShell } from '$lib/shell.svelte';
  import type { TribeRecord } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import Table from '$lib/ui/Table.svelte';

  let tribes = $state<TribeRecord[]>([]);
  let counts = $state<Record<string, number>>({});
  let loaded = $state(false);
  let loading = $state('Loading tribes');
  let loadError = $state('');

  showShell(() => ({ title: 'Tribes' }));

  const q = $derived($page.url.searchParams.get('q') ?? '');
  const shown = $derived(tribes.filter((tribe) => matchText([tribe.name], q)));

  function load() {
    loading = 'Loading tribes';
    loadError = '';
    void getTribes()
      .then((body) => {
        tribes = body.tribes;
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

  $effect(() => {
    for (const tribe of tribes) {
      if (!canManageTribe(tribe.id)) continue;
      void getMembers(tribe.id)
        .then((body) => (counts[tribe.id] = body.members.length))
        .catch(() => {});
    }
  });
</script>

<Page title="Tribes" description="A tribe groups people with the projects they can see." {loading} error={loadError} onretry={load}>
  {#snippet actions()}
    {#if shell.admin}<Button variant="primary" href="/tribes/new" icon="plus">New tribe</Button>{/if}
  {/snippet}
  {#if loaded && tribes.length === 0}
    <EmptyState title="No tribes yet" />
  {:else if tribes.length > 0}
    <div class="flex flex-col gap-4">
      <SearchBox label="Search tribes" value={q} placeholder="Search by name" />
      {#if shown.length === 0}
        <EmptyState title="No tribes match" description="Change the search to see more tribes.">
          <Button type="button" variant="secondary" icon="close" onclick={() => setParams({ q: '' })}>Clear search</Button>
        </EmptyState>
      {:else}
        <Table label="Tribes">
          <thead>
            <tr class="border-b border-line bg-ground/50">
              <th class={th} scope="col">Name</th>
              <th class={th} scope="col">Members</th>
              <th class={th} scope="col"><span class="sr-only">Manage</span></th>
            </tr>
          </thead>
          <tbody>
            {#each shown as tribe (tribe.id)}
              <tr class={tr}>
                <td class="{td} font-medium text-ink">{tribe.name}</td>
                <td class="{td} text-muted">{counts[tribe.id] ?? '—'}</td>
                <td class="{td} text-right">
                  {#if canManageTribe(tribe.id)}
                    <a
                      class="inline-flex min-h-11 items-center px-2 text-sm font-medium text-accent hover:underline"
                      href={`/tribes/${tribe.id}`}
                      aria-label={`Manage ${tribe.name}`}>Manage</a
                    >
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </Table>
      {/if}
    </div>
  {/if}
</Page>
