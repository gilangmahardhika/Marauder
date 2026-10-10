<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getCard, unreachable } from '$lib/api';
  import CardActions from '$lib/components/CardActions.svelte';
  import { shell } from '$lib/shell.svelte';
  import type { Card } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let card = $state<Card | null>(null);
  let missing = $state(false);

  const controls = ['approve', 'deploy', 'request_changes', 'reject', 'unblock'];
  const hasControls = $derived(
    card !== null &&
      ((card.actions ?? []).some((action) => controls.includes(action)) ||
        (card.status === 'blocked' && card.unblockTo === null))
  );

  async function load(id: string) {
    shell.message = 'Loading card';
    missing = false;
    try {
      card = await getCard(id);
      shell.message = card.title;
    } catch (error) {
      card = null;
      if (error instanceof ApiError && error.status === 404) missing = true;
      shell.message = error instanceof ApiError ? error.message : unreachable();
    }
  }

  $effect(() => {
    const id = $page.params.id;
    if (!id || shell.projectId === '') return;
    void load(id);
  });

  $effect(() => {
    shell.title = missing ? 'Card' : (card?.title ?? 'Card');
  });

  function linkable(url: string) {
    return url.startsWith('https://github.com/') || url.startsWith('http://127.0.0.1');
  }

  function tone(value: Card) {
    if (value.status === 'blocked' || value.group === 'stopped') return 'red';
    if (value.group === 'finished') return 'green';
    if (value.group === 'review') return 'amber';
    if (value.group === 'running' || value.group === 'shipping') return 'sky';
    return 'default';
  }
</script>

{#snippet item(label: string, value: string, mono = false, danger = false)}
  <div class="min-w-0">
    <dt class="text-xs text-muted">{label}</dt>
    <dd class="mt-1 break-words text-sm {mono ? 'font-mono text-[13px]' : ''} {danger ? 'danger' : 'text-ink'}">{value}</dd>
  </div>
{/snippet}

{#if missing}
  <Page title="Card not found">
    <EmptyState title="This card is not on the board for this project." href="/" link="Back to the board" />
  </Page>
{:else if card}
  <Page title={card.title}>
    {#snippet meta()}
      <Badge tone={tone(card!)}>{card!.status}</Badge>
    {/snippet}
    {#snippet actions()}
      <Button href={`/cards/${card!.id}/trace`}>View trace</Button>
    {/snippet}
    <div class="flex flex-col gap-6">
      {#if hasControls}
        <Panel title="Next step">
          <CardActions bind:card />
        </Panel>
      {/if}
      <Panel title="Details">
        <dl class="grid gap-x-10 gap-y-5 sm:grid-cols-2">
          {@render item('Requirements', card.requirementIds.join(', ') || '—', true)}
          {@render item('Decisions', card.decisionIds.join(', ') || '—', true)}
          {@render item('Risk', card.risk)}
          {@render item('Attempts', `${card.attemptsUsed} of ${card.maxAttempts}`)}
          {#if card.branch}{@render item('Branch', card.branch, true)}{/if}
          {#if card.prUrl}
            <div class="min-w-0">
              <dt class="text-xs text-muted">Pull request</dt>
              <dd class="mt-1 break-words text-sm">
                {#if linkable(card.prUrl)}
                  <a class="text-accent underline" href={card.prUrl}>{card.prUrl}</a>
                {:else}{card.prUrl}{/if}
              </dd>
            </div>
          {/if}
          {#if card.localCheck}{@render item('Local check', card.localCheck, true)}{/if}
          {#if card.devCheck}{@render item('Dev check', card.devCheck, true)}{/if}
          {#if card.headSha}{@render item('Head SHA', card.headSha, true)}{/if}
          {#if card.reviewedSha}{@render item('Reviewed SHA', card.reviewedSha, true)}{/if}
          {#if card.reviewIndependent !== null}{@render item('Review independent', card.reviewIndependent ? 'yes' : 'no')}{/if}
          {#if card.approvedSha}{@render item('Approved SHA', card.approvedSha, true)}{/if}
          {#if card.deployedSha}{@render item('Deployed SHA', card.deployedSha, true)}{/if}
          {#if card.failureReason}{@render item('Failure reason', card.failureReason)}{/if}
          {#if card.blockedReason}{@render item('Blocked reason', card.blockedReason, false, true)}{/if}
        </dl>
      </Panel>
    </div>
  </Page>
{/if}
