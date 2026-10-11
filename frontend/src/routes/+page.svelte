<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getCards, startRun, unreachable } from '$lib/api';
  import { filterCards, ownerChoices, parseOwners, personName } from '$lib/board';
  import OwnerFilter from '$lib/components/OwnerFilter.svelte';
  import { setParams } from '$lib/params';
  import { shell, showShell } from '$lib/shell.svelte';
  import { statusLabel } from '$lib/status';
  import type { Card } from '$lib/types';
  import Avatar from '$lib/ui/Avatar.svelte';
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
    message = 'Running the worker';
    try {
      await startRun();
      await load();
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  function tone(card: Card) {
    if (card.status === 'blocked' || card.group === 'stopped') return 'bg-red-600';
    if (card.group === 'running' || card.group === 'shipping') return 'bg-sky-600';
    if (card.group === 'review') return 'bg-amber-500';
    if (card.group === 'finished') return 'bg-emerald-600';
    return 'bg-ink/30';
  }

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
    <Button variant="primary" type="button" disabled={!canRun || running || busy} onclick={() => void run()}>Run</Button>
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
                <a
                  class="block rounded-control border border-line bg-surface p-3 text-ink hover:border-line-strong hover:shadow-sm"
                  href={`/cards/${card.id}`}
                >
                  <span class="block text-sm font-medium leading-snug">{card.title}</span>
                  <span class="mt-2 flex items-center gap-1.5 text-xs {card.status === 'blocked' ? 'danger' : 'text-muted'}">
                    <span class="size-1.5 shrink-0 rounded-full {tone(card)}"></span>
                    <span title={card.status}>{statusLabel(card.status)}</span>
                  </span>
                  <span class="mt-2 flex items-center gap-2 text-xs text-muted">
                    <Avatar person={card.owner} size="sm" />
                    <span class="min-w-0 flex-1 truncate">{card.owner ? personName(card.owner) : 'No owner'}</span>
                    {#if card.reviewers?.length}
                      {@const names = card.reviewers.map(personName).join(', ')}
                      <span class="flex shrink-0 items-center" role="img" aria-label={`Reviewers: ${names}`} title={`Reviewers: ${names}`}>
                        {#each card.reviewers.slice(0, 3) as reviewer (reviewer.id)}
                          <span class="-ml-1.5 rounded-full ring-2 ring-surface first:ml-0"><Avatar person={reviewer} size="sm" /></span>
                        {/each}
                        {#if card.reviewers.length > 3}
                          <span class="ml-1 text-[11px]">+{card.reviewers.length - 3}</span>
                        {/if}
                      </span>
                    {/if}
                  </span>
                  <span class="mt-3 flex items-center justify-between border-t border-line pt-2 text-[11px] text-muted">
                    <span>Attempt {card.attemptsUsed} of {card.maxAttempts}</span>
                    {#if card.headSha}<span class="font-mono">{card.headSha.slice(0, 7)}</span>{/if}
                  </span>
                </a>
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
