<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getTrace, unreachable } from '$lib/api';
  import { pager } from '$lib/pager.svelte';
  import { setParams } from '$lib/params';
  import { shell, showShell } from '$lib/shell.svelte';
  import { attemptOptions, filterEvents, kindOptions } from '$lib/trace';
  import type { TraceEvent } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import FilterSelect from '$lib/ui/FilterSelect.svelte';
  import Page from '$lib/ui/Page.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import ShowMore from '$lib/ui/ShowMore.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  let events = $state<TraceEvent[]>([]);
  let loaded = $state(false);
  let message = $state('Loading trace');

  const cardId = $derived($page.params.id ?? '');

  showShell(() => ({ title: 'Trace', message }));

  const params = $derived($page.url.searchParams);
  const kinds = $derived(kindOptions(events));
  const attempts = $derived(attemptOptions(events));
  const filter = $derived({
    q: params.get('q') ?? '',
    kind: kinds.some((option) => option.value === params.get('kind')) ? (params.get('kind') ?? '') : '',
    attempt: attempts.some((option) => option.value === params.get('attempt')) ? (params.get('attempt') ?? '') : ''
  });
  const shown = $derived(filterEvents(events, filter));
  const paging = pager(() => `${filter.q}|${filter.kind}|${filter.attempt}`);

  $effect(() => {
    const id = $page.params.id;
    if (!id || shell.projectId === '') return;
    void getTrace(id)
      .then((body) => {
        events = body.events;
        loaded = true;
        message = 'Trace';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });
</script>

<Page title="Trace" description={`Events for ${cardId}, oldest first.`}>
  {#snippet actions()}
    <Button href={`/cards/${cardId}`}>Back to card</Button>
  {/snippet}
  {#if loaded && events.length === 0}
    <EmptyState title="No events yet" />
  {:else if events.length > 0}
    <div class="flex flex-col gap-4">
      <Toolbar>
        <SearchBox label="Search events" value={filter.q} placeholder="Search kind, attempt, or payload" />
        <FilterSelect label="Kind" options={kinds} value={filter.kind} onchange={(kind) => setParams({ kind })} />
        {#if attempts.length > 2}
          <FilterSelect label="Attempt" options={attempts} value={filter.attempt} onchange={(attempt) => setParams({ attempt })} />
        {/if}
      </Toolbar>
      {#if shown.length === 0}
        <EmptyState title="No events match" description="Change the search or filters to see more events.">
          <Button type="button" variant="secondary" onclick={() => setParams({ q: '', kind: '', attempt: '' })}>Clear filters</Button>
        </EmptyState>
      {:else}
        <ol class="flex flex-col gap-4 border-l border-line pl-6">
          {#each shown.slice(0, paging.limit) as event (event.id)}
            <li class="relative">
              <span class="absolute -left-[29px] top-4 size-2.5 rounded-full border-2 border-ground bg-accent" aria-hidden="true"
              ></span>
              <div class="rounded-panel border border-line bg-surface">
                <div class="flex flex-wrap items-center gap-2 border-b border-line px-4 py-2.5">
                  <Badge>{event.kind}</Badge>
                  {#if event.attemptId}<span class="font-mono text-xs text-muted">{event.attemptId}</span>{/if}
                  <span class="ml-auto font-mono text-xs text-muted">{event.ts}</span>
                </div>
                <pre class="overflow-x-auto px-4 py-3 font-mono text-xs leading-relaxed text-ink">{JSON.stringify(
                    event.payload,
                    null,
                    2
                  )}</pre>
              </div>
            </li>
          {/each}
        </ol>
        <ShowMore total={shown.length} limit={paging.limit} noun={['event', 'events']} onmore={paging.more} />
      {/if}
    </div>
  {/if}
</Page>
