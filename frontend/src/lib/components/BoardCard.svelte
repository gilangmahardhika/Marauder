<script lang="ts">
  import { personName } from '$lib/board';
  import { statusLabel } from '$lib/status';
  import type { Card } from '$lib/types';
  import Avatar from '$lib/ui/Avatar.svelte';
  import Button from '$lib/ui/Button.svelte';

  let { card, busy = false, onrun }: { card: Card; busy?: boolean; onrun: (card: Card) => void } = $props();

  const canRun = $derived((card.actions ?? []).includes('run'));
  const waiting = $derived(card.status === 'ready' && (card.waitingOn ?? []).length > 0);

  function tone() {
    if (card.status === 'blocked' || card.group === 'stopped') return 'bg-red-600';
    if (card.group === 'running' || card.group === 'shipping') return 'bg-sky-600';
    if (card.group === 'review') return 'bg-amber-500';
    if (card.group === 'finished') return 'bg-emerald-600';
    return 'bg-ink/30';
  }
</script>

<article class="rounded-control border border-line bg-surface text-ink hover:border-line-strong hover:shadow-sm">
  <a class="block p-3" href={`/cards/${card.id}`}>
    <span class="block text-sm font-medium leading-snug">{card.title}</span>
    <span class="mt-2 flex items-center gap-1.5 text-xs {card.status === 'blocked' ? 'danger' : 'text-muted'}">
      <span class="size-1.5 shrink-0 rounded-full {tone()}"></span>
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
  {#if canRun || card.queued || waiting}
    <div class="flex items-center justify-between gap-2 border-t border-line px-3 py-2 text-xs text-muted">
      {#if canRun}
        <span>Ready to start</span>
        <Button type="button" disabled={busy} onclick={() => onrun(card)}>Run</Button>
      {:else if card.queued}
        <span class="rounded-full bg-sky-50 px-2 py-0.5 font-medium text-sky-800">Queued</span>
      {:else}
        {@const count = card.waitingOn.length}
        <span title={`Waiting on ${card.waitingOn.join(', ')}`}>Waiting on {count} {count === 1 ? 'card' : 'cards'}</span>
      {/if}
    </div>
  {/if}
</article>
