<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getCard, getCardDiff, unreachable } from '$lib/api';
  import CardActions from '$lib/components/CardActions.svelte';
  import CardChanges from '$lib/components/CardChanges.svelte';
  import CardContext from '$lib/components/CardContext.svelte';
  import CardInstructions from '$lib/components/CardInstructions.svelte';
  import CardNotes from '$lib/components/CardNotes.svelte';
  import CardPullRequest from '$lib/components/CardPullRequest.svelte';
  import { plainText } from '$lib/markdown';
  import { displayName } from '$lib/people';
  import { shell } from '$lib/shell.svelte';
  import { cardTone, reasonLabel, statusLabel } from '$lib/status';
  import { verdictLabel } from '$lib/trace';
  import type { Card, CardDiff, CardOwner, CardRequirement } from '$lib/types';
  import Avatar from '$lib/ui/Avatar.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let card = $state<Card | null>(null);
  let missing = $state(false);

  const controls = ['run', 'retry_review', 'approve', 'deploy', 'request_changes', 'reject', 'unblock'];
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

  let diff = $state<CardDiff | null>(null);
  let diffError = $state('');
  const diffKey = $derived(card ? `${card.id}@${card.headSha ?? ''}` : '');

  $effect(() => {
    const key = diffKey;
    if (!key) return;
    const id = key.slice(0, key.indexOf('@'));
    diff = null;
    diffError = '';
    getCardDiff(id)
      .then((body) => {
        if (diffKey === key) diff = body;
      })
      .catch((error: unknown) => {
        if (diffKey === key) diffError = error instanceof ApiError ? error.message : unreachable();
      });
  });

  function linkable(url: string) {
    return url.startsWith('https://github.com/') || url.startsWith('http://127.0.0.1');
  }

  /** The heading without the requirement id it often starts with. */
  function headingText(requirement: CardRequirement) {
    const heading = requirement.heading.trim();
    return heading.startsWith(requirement.id) ? heading.slice(requirement.id.length).trim() : heading;
  }
</script>

{#snippet item(label: string, value: string, mono = false, danger = false)}
  <div class="flex min-w-0 items-baseline justify-between gap-4">
    <dt class="shrink-0 text-xs text-muted">{label}</dt>
    <dd class="min-w-0 break-words text-right text-sm {mono ? 'font-mono text-[13px]' : ''} {danger ? 'danger' : 'text-ink'}">{value}</dd>
  </div>
{/snippet}

{#snippet sha(label: string, value: string)}
  <div class="flex min-w-0 items-baseline justify-between gap-4">
    <dt class="shrink-0 text-xs text-muted">{label}</dt>
    <dd class="font-mono text-[13px] text-ink" title={value}>{value.slice(0, 7)}</dd>
  </div>
{/snippet}

{#snippet person(who: CardOwner, role: string)}
  <li class="flex items-center gap-3">
    <Avatar person={who} />
    <span class="min-w-0 flex-1">
      <span class="block truncate text-sm font-medium text-ink">{displayName(who)}</span>
      <span class="block truncate text-xs text-muted" title={who.email}>{who.email}</span>
      <span class="block truncate text-xs text-muted">
        {who.githubLogin ? `GitHub @${who.githubLogin}` : 'No verified GitHub login'}
      </span>
    </span>
    <span class="shrink-0 text-xs text-muted">{role}</span>
  </li>
{/snippet}

{#if missing}
  <Page title="Card not found">
    <EmptyState title="This card is not on the board for this project." href="/" link="Back to the board" />
  </Page>
{:else if card}
  <Page title={card.title}>
    {#snippet meta()}
      <span title={card!.status}><Badge tone={cardTone(card!)}>{statusLabel(card!.status)}</Badge></span>
    {/snippet}
    {#snippet summary()}
      <p class="flex flex-wrap items-center gap-x-4 gap-y-1 text-[13px] text-muted">
        <span class="inline-flex items-center gap-1.5">
          <Avatar person={card!.owner} size="sm" />
          {card!.owner ? displayName(card!.owner) : 'No owner'}
        </span>
        <span>Attempt {card!.attemptsUsed} of {card!.maxAttempts}</span>
        {#if card!.branch}
          <span class="inline-flex min-w-0 items-center gap-1.5">
            <Icon name="git" class="size-4 shrink-0" /><span class="truncate font-mono">{card!.branch}</span>
          </span>
        {/if}
      </p>
    {/snippet}
    {#snippet actions()}
      {#if card!.prUrl && linkable(card!.prUrl)}
        <Button href={card!.prUrl}>Open pull request</Button>
      {/if}
      <Button href={`/cards/${card!.id}/trace`}><Icon name="terminal" />View trace</Button>
    {/snippet}
    <div class="flex flex-col gap-6">
      {#if card.blockedReason === 'owner_github'}
        <Notice tone="danger">
          The owner's GitHub account cannot push this card. The owner saves a token on the Account page and runs Test
          connection. Then unblock the card.
        </Notice>
      {/if}
      <div class="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_340px]">
        <div class="flex min-w-0 flex-col gap-6" data-testid="card-main">
          <Panel title="Next step">
            {#if hasControls}
              <CardActions bind:card />
            {:else if card.queued}
              <p class="text-sm text-muted">Queued. The card starts when the worker picks it up.</p>
            {:else}
              <p class="text-sm text-muted">No action is open for you right now.</p>
            {/if}
          </Panel>
          <CardPullRequest pullRequest={card.pullRequest ?? null} prUrl={card.prUrl} branch={card.branch} base={diff?.base ?? ''} />
          <CardChanges {diff} error={diffError} />
          <CardNotes notes={card.notes ?? []} />
          {#if (card.actions ?? []).includes('instructions')}
            <CardInstructions bind:card />
          {/if}
          {#if card.context}
            <CardContext bind:card />
          {/if}
        </div>
        <aside class="flex min-w-0 flex-col gap-6" aria-label="Card details">
          <Panel title="Details">
            <dl class="flex flex-col gap-3">
              {#if card.blockedReason}{@render item('Blocked reason', reasonLabel(card.blockedReason), false, true)}{/if}
              {#if card.failureReason}{@render item('Failure reason', reasonLabel(card.failureReason))}{/if}
              {#if card.reviewVerdict}{@render item('AI review', verdictLabel(card.reviewVerdict), false, card.reviewVerdict === 'failed')}{/if}
              {#if card.reviewIndependent !== null}{@render item('Review independent', card.reviewIndependent ? 'yes' : 'no')}{/if}
              {#if card.selfReview}{@render item('Approval', 'Self review: the owner approves alone')}{/if}
              {#if card.aiReviewMode === 'manual'}{@render item('After AI review', 'A person reviews')}{/if}
              {#if card.headSha}{@render sha('Head SHA', card.headSha)}{/if}
              {#if card.reviewedSha}{@render sha('Reviewed SHA', card.reviewedSha)}{/if}
              {#if card.approvedSha}{@render sha('Approved SHA', card.approvedSha)}{/if}
              {#if card.deployedSha}{@render sha('Deployed SHA', card.deployedSha)}{/if}
              {@render item('Attempts', `${card.attemptsUsed} of ${card.maxAttempts}`)}
              {#if card.localCheck}{@render item('Local check', card.localCheck, true)}{/if}
              {#if card.devCheck}{@render item('Dev check', card.devCheck, true)}{/if}
              {@render item('Risk', card.risk)}
              {@render item('Decisions', card.decisionIds.join(', ') || '—', true)}
            </dl>
          </Panel>
          <Panel title="People">
            <ul class="flex flex-col gap-4">
              {#if card.owner}
                {@render person(card.owner, 'Owner')}
              {:else}
                <li class="flex items-center gap-3 text-sm text-muted"><Avatar person={null} />No owner</li>
              {/if}
              {#each card.reviewers ?? [] as reviewer (reviewer.id)}
                {@render person(reviewer, 'Reviewer')}
              {/each}
            </ul>
            {#if (card.reviewers ?? []).length === 0}
              <p class="text-sm text-muted">
                No reviewers. Reviewers come from the card's requirements.
                {#if card.requirementIds.length > 0}
                  <a class="text-accent underline" href={`/intake/${card.requirementIds[0]}`}>Edit reviewers</a>
                {/if}
              </p>
            {/if}
          </Panel>
          <Panel title="Intake" hint="The requirements this card builds.">
            {#if (card.requirements ?? []).length === 0}
              <p class="text-sm text-muted">No requirements.</p>
            {:else}
              <ul class="flex flex-col divide-y divide-line">
                {#each card.requirements ?? [] as requirement (requirement.id)}
                  <li class="py-3 first:pt-0 last:pb-0">
                    <a class="group block" href={`/intake/${requirement.id}`}>
                      <span class="flex flex-wrap items-center gap-2">
                        <span class="font-mono text-[13px] text-muted">{requirement.id}</span>
                        {#if headingText(requirement)}
                          <span class="text-sm font-medium text-ink group-hover:underline">{headingText(requirement)}</span>
                        {/if}
                        {#if !requirement.accepted}<Badge tone="amber">Draft</Badge>{/if}
                      </span>
                      <span class="mt-1 line-clamp-3 text-sm text-muted">{plainText(requirement.statement)}</span>
                    </a>
                  </li>
                {/each}
              </ul>
            {/if}
          </Panel>
        </aside>
      </div>
    </div>
  </Page>
{/if}
