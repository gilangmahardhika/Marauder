<script lang="ts">
  import { ApiError, getCards, startRun, unreachable } from '$lib/api';
  import Live from '$lib/components/Live.svelte';
  import type { Card } from '$lib/types';

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
  let message = $state('Loading board');
  let busy = $state(false);

  async function load() {
    try {
      const board = await getCards();
      cards = board.cards;
      canRun = board.canRun;
      running = board.running;
      if (message === 'Loading board') message = 'Board';
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    }
  }

  $effect(() => {
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
</script>

<h1>Board</h1>
<Live {message} />
<button class="primary" type="button" disabled={!canRun || running || busy} onclick={() => void run()}>Run</button>
{#if cards.length === 0}
  <p>No cards yet</p>
  <a href="/intake">Ingest requirements</a>
{:else}
  <div class="groups">
    {#each labels as [group, label]}
      <section>
        <h2>{label}</h2>
        {#each cards.filter((card) => card.group === group) as card}
          <a class="row" href={`/cards/${card.id}`}>
            {card.title}
            <span class:danger={card.status === 'blocked'}>{card.status}</span>
          </a>
        {/each}
      </section>
    {/each}
  </div>
{/if}
