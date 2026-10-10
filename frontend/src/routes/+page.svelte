<script lang="ts">
  import { ApiError, getCards, startRun, unreachable } from '$lib/api';
  import { shell, showShell } from '$lib/shell.svelte';
  import type { Card } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';

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
    <div
      class="grid grid-cols-1 gap-3 min-[720px]:auto-cols-[minmax(15rem,1fr)] min-[720px]:grid-flow-col min-[720px]:grid-cols-none min-[720px]:overflow-x-auto min-[720px]:pb-3"
    >
      {#each labels as [group, label] (group)}
        {@const column = cards.filter((card) => card.group === group)}
        <section class="flex min-w-0 flex-col rounded-panel bg-sidebar/70 p-2" aria-label={label}>
          <h2 class="flex items-center justify-between px-2 pb-2.5 pt-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted">
            {label}
            <span class="rounded-full bg-surface px-2 py-0.5 text-[11px] font-medium tracking-normal text-ink">{column.length}</span>
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
                  {card.status}
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
</Page>
