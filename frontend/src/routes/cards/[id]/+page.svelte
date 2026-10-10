<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getCard, unreachable } from '$lib/api';
  import CardActions from '$lib/components/CardActions.svelte';
  import Live from '$lib/components/Live.svelte';
  import { live } from '$lib/live';
  import type { Card } from '$lib/types';

  let card = $state<Card | null>(null);
  let missing = $state(false);

  async function load(id: string) {
    live.set('Loading card');
    missing = false;
    try {
      card = await getCard(id);
      live.set(card.title);
    } catch (error) {
      card = null;
      if (error instanceof ApiError && error.status === 404) missing = true;
      live.set(error instanceof ApiError ? error.message : unreachable());
    }
  }

  $effect(() => {
    const id = $page.params.id;
    if (id) void load(id);
  });

  function linkable(url: string) {
    return url.startsWith('https://github.com/') || url.startsWith('http://127.0.0.1');
  }
</script>

<Live message={$live} />
{#if missing}
  <p>Card not found</p>
  <a href="/">Board</a>
{:else if card}
  <h1>{card.title}</h1>
  <p class:danger={card.status === 'blocked'}>{card.status}</p>
  <dl>
    <dt>Requirements</dt>
    <dd>{card.requirementIds.join(', ')}</dd>
    <dt>Decisions</dt>
    <dd>{card.decisionIds.join(', ')}</dd>
    <dt>Risk</dt>
    <dd>{card.risk}</dd>
    <dt>Attempts</dt>
    <dd>{card.attemptsUsed} / {card.maxAttempts}</dd>
    {#if card.localCheck}<dt>Local check</dt><dd>{card.localCheck}</dd>{/if}
    {#if card.devCheck}<dt>Dev check</dt><dd>{card.devCheck}</dd>{/if}
    {#if card.branch}<dt>Branch</dt><dd>{card.branch}</dd>{/if}
    {#if card.prUrl}
      <dt>Pull request</dt>
      <dd>
        {#if linkable(card.prUrl)}<a href={card.prUrl}>{card.prUrl}</a>{:else}{card.prUrl}{/if}
      </dd>
    {/if}
    {#if card.headSha}<dt>Head SHA</dt><dd>{card.headSha}</dd>{/if}
    {#if card.reviewedSha}<dt>Reviewed SHA</dt><dd>{card.reviewedSha}</dd>{/if}
    {#if card.reviewIndependent !== null}<dt>Review independent</dt><dd>{card.reviewIndependent ? 'yes' : 'no'}</dd>{/if}
    {#if card.approvedSha}<dt>Approved SHA</dt><dd>{card.approvedSha}</dd>{/if}
    {#if card.deployedSha}<dt>Deployed SHA</dt><dd>{card.deployedSha}</dd>{/if}
    {#if card.failureReason}<dt>Failure reason</dt><dd>{card.failureReason}</dd>{/if}
    {#if card.blockedReason}<dt>Blocked reason</dt><dd>{card.blockedReason}</dd>{/if}
  </dl>
  <CardActions bind:card />
  <a href={`/cards/${card.id}/trace`}>Trace</a>
{/if}
