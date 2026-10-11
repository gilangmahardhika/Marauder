<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getCards, runCard, startRun, unreachable } from '$lib/api';
  import { filterCards, ownerChoices, parseOwners } from '$lib/board';
  import BoardCard from '$lib/components/BoardCard.svelte';
  import OwnerFilter from '$lib/components/OwnerFilter.svelte';
  import { setParams } from '$lib/params';
  import { shell, showShell } from '$lib/shell.svelte';
  import type { Card } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  const labels = [
    ['waiting', 'Waiting'],
    ['running', 'Running'],
    ['review', 'Review'],
    ['shipping', 'Shipping'],
    ['finished', 'Finished'],
    ['stopped', 'Stopped']
  ] as const;

  let cards = $state<Card[]>([]);
  let canRun = $state(false);
  let readyCount = $state(0);
  let running = $state(false);
  let loaded = $state(false);
  let unavailable = $state(false);
  let message = $state('Loading board');
  let busy = $state(false);

  async function load() {
    try {
      const board = await getCards();
      cards = board.cards;
      canRun = board.canRun;
      readyCount = board.readyCount ?? 0;
      running = board.running;
      loaded = true;
      unavailable = false;
      if (message === 'Loading board') message = 'Board';
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) {
        unavailable = true;
        message = 'That project is not available.';
      } else message = error instanceof ApiError ? error.message : unreachable();
    }
  }

  $effect(() => {
    if (shell.projectId === '') return;
    void load();
  });

  const polling = $derived(
    running || cards.some((card) => card.group === 'running' || card.group === 'shipping' || card.status === 'agent_review')
  );

  $effect(() => {
    if (!polling) return;
    const timer = setInterval(() => {
      void load();
    }, 3000);
    return () => clearInterval(timer);
  });

  async function run() {
    busy = true;
    message = 'Queueing ready cards';
    try {
      const result = await startRun();
      await load();
      message = `Queued ${result.queued} ${result.queued === 1 ? 'card' : 'cards'}.`;
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  async function runOne(card: Card) {
    busy = true;
    message = `Queueing ${card.title}`;
    try {
      const next = await runCard(card.id);
      cards = cards.map((item) => (item.id === next.id ? { ...item, ...next } : item));
      readyCount = Math.max(0, readyCount - 1);
      message = `Queued ${next.title}.`;
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        await load();
        message = 'Card changed. Reloaded.';
      } else message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  const runHint = $derived(
    !canRun && readyCount > 0
      ? 'Your role cannot start cards'
      : readyCount > 0
        ? `Queue ${readyCount} ready ${readyCount === 1 ? 'card' : 'cards'}`
        : cards.some((card) => card.queued)
          ? 'All ready cards are queued'
          : 'No ready card can start'
  );

  const params = $derived($page.url.searchParams);
  const choices = $derived(ownerChoices(cards, shell.email));
  const filter = $derived({
    q: params.get('q') ?? '',
    owners: parseOwners(params.get('owner'), choices, shell.email),
    blocked: params.get('blocked') === '1'
  });
  const filtered = $derived(filter.q !== '' || filter.owners.length > 0 || filter.blocked);
  const shown = $derived(filterCards(cards, filter));

  const summary = $derived(
    cards.length === 0
      ? ''
      : `${cards.length} ${cards.length === 1 ? 'card' : 'cards'}${running ? ' · worker running' : ''}`
  );

  showShell(() => ({ title: 'Board', message }));
</script>

<Page title="Board" description={summary}>
  {#snippet actions()}
    <span title={runHint}>
      <Button variant="primary" type="button" disabled={!canRun || readyCount === 0 || busy} onclick={() => void run()}
        >Run ready cards ({readyCount})</Button
      >
    </span>
  {/snippet}
  {#if unavailable}
    <EmptyState
      title="That project is not available."
      description="You are not a member of its tribe. Pick another project in the sidebar, or ask an admin to add you."
    />
  {:else if loaded && cards.length === 0}
    <EmptyState
      title="No cards yet"
      description="Cards appear here once requirements are ingested and accepted."
      href="/intake"
      link="Ingest requirements"
    />
  {:else if cards.length > 0}
    <div class="mb-4">
      <Toolbar>
        <SearchBox label="Search cards" value={filter.q} placeholder="Search title, card id, requirement, owner, or reviewer" />
        <OwnerFilter {choices} selected={filter.owners} onchange={(next) => setParams({ owner: next.join(',') })} />
        <label class="flex min-h-11 items-center gap-2 text-sm text-ink">
          <input
            type="checkbox"
            class="size-4 accent-accent"
            checked={filter.blocked}
            onchange={(event) => setParams({ blocked: event.currentTarget.checked ? '1' : '' })}
          />
          Blocked only
        </label>
      </Toolbar>
    </div>
    {#if shown.length === 0}
      <EmptyState title="No cards match" description="Change the search or filters to see more cards.">
        <Button type="button" variant="secondary" onclick={() => setParams({ q: '', owner: '', blocked: '' })}>Clear filters</Button>
      </EmptyState>
    {:else}
      <div
        class="grid grid-cols-1 gap-3 min-[720px]:auto-cols-[minmax(15rem,1fr)] min-[720px]:grid-flow-col min-[720px]:grid-cols-none min-[720px]:overflow-x-auto min-[720px]:pb-3"
      >
        {#each labels as [group, label] (group)}
          {@const column = shown.filter((card) => card.group === group)}
          {@const total = cards.filter((card) => card.group === group).length}
          <section class="flex min-w-0 flex-col rounded-panel bg-sidebar/70 p-2" aria-label={label}>
            <h2 class="flex items-center justify-between px-2 pb-2.5 pt-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted">
              {label}
              <span class="rounded-full bg-surface px-2 py-0.5 text-[11px] font-medium tracking-normal text-ink"
                >{filtered ? `${column.length} of ${total}` : column.length}</span
              >
            </h2>
            <div class="flex flex-col gap-2">
              {#each column as card (card.id)}
                <BoardCard {card} {busy} onrun={(next) => void runOne(next)} />
              {:else}
                <p class="rounded-control border border-dashed border-line-strong px-3 py-4 text-xs text-muted">No cards</p>
              {/each}
            </div>
          </section>
        {/each}
      </div>
    {/if}
  {/if}
</Page>
