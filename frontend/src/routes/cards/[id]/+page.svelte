<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getCard, getCardDiff, unreachable } from '$lib/api';
  import CardActions from '$lib/components/CardActions.svelte';
  import CardChanges from '$lib/components/CardChanges.svelte';
  import { categoryLabel } from '$lib/conventions';
  import { displayName } from '$lib/people';
  import { shell } from '$lib/shell.svelte';
  import { cardTone, reasonLabel, statusLabel } from '$lib/status';
  import { verdictLabel } from '$lib/trace';
  import type { Card, CardDiff, CardOwner, CardRequirement, CardRule } from '$lib/types';
  import Avatar from '$lib/ui/Avatar.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Notice from '$lib/ui/Notice.svelte';
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

  /** Rules in API order, grouped by label: global first, then each stack. */
  function groupRules(rules: CardRule[]) {
    const groups: { label: string; rules: CardRule[] }[] = [];
    for (const rule of rules) {
      const last = groups[groups.length - 1];
      if (last && last.label === rule.label) last.rules.push(rule);
      else groups.push({ label: rule.label, rules: [rule] });
    }
    return groups;
  }

  const ruleGroups = $derived(groupRules(card?.rules ?? []));

  /** The heading without the requirement id it often starts with. */
  function headingText(requirement: CardRequirement) {
    const heading = requirement.heading.trim();
    return heading.startsWith(requirement.id) ? heading.slice(requirement.id.length).trim() : heading;
  }
</script>

{#snippet item(label: string, value: string, mono = false, danger = false)}
  <div class="min-w-0">
    <dt class="text-xs text-muted">{label}</dt>
    <dd class="mt-1 break-words text-sm {mono ? 'font-mono text-[13px]' : ''} {danger ? 'danger' : 'text-ink'}">{value}</dd>
  </div>
{/snippet}

{#snippet person(who: CardOwner, role: string)}
  <li class="flex items-center gap-3">
    <Avatar person={who} />
    <span class="min-w-0 flex-1">
      <span class="block truncate text-sm font-medium text-ink">{displayName(who)}</span>
      <span class="block truncate text-xs text-muted">
        {who.email} · {who.githubLogin ? `GitHub @${who.githubLogin}` : 'No verified GitHub login'}
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
    {#snippet actions()}
      <Button href={`/cards/${card!.id}/trace`}>View trace</Button>
    {/snippet}
    <div class="flex flex-col gap-6">
      {#if card.blockedReason === 'owner_github'}
        <Notice tone="danger">
          The owner's GitHub account cannot push this card. The owner saves a token on the Account page and runs Test
          connection. Then unblock the card.
        </Notice>
      {/if}
      {#if hasControls}
        <Panel title="Next step">
          <CardActions bind:card />
        </Panel>
      {/if}
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
                  <span class="mt-1 line-clamp-3 text-sm text-muted">{requirement.statement}</span>
                </a>
              </li>
            {/each}
          </ul>
        {/if}
      </Panel>
      <Panel title="Rules" hint="Coding rules the implementer and reviewer follow for this project.">
        {#if ruleGroups.length === 0}
          <p class="text-sm text-muted">No rules apply to this project yet.</p>
        {:else}
          {#each ruleGroups as group (group.label)}
            <div>
              <h3 class="text-xs font-semibold uppercase tracking-wider text-muted">
                {group.label === 'global' ? 'Global' : group.label}
              </h3>
              <ul class="mt-2 flex flex-col gap-2">
                {#each group.rules as rule (rule.id)}
                  <li class="flex items-center justify-between gap-3 text-sm text-ink">
                    <span class="min-w-0">{rule.title}</span>
                    <Badge>{categoryLabel(rule.category)}</Badge>
                  </li>
                {/each}
              </ul>
            </div>
          {/each}
        {/if}
        {#snippet footer()}
          <Button href="/settings/conventions">Project conventions</Button>
        {/snippet}
      </Panel>
      <Panel title="Skills" hint="Integration procedures attached to this card.">
        {#if card.skills.length === 0}
          <p class="text-sm text-muted">No skills attached.</p>
        {:else}
          <ul class="flex flex-col gap-3">
            {#each card.skills as skill (skill.id)}
              <li>
                <span class="block text-sm font-medium text-ink">{skill.title}</span>
                {#if skill.useWhen}<span class="mt-0.5 line-clamp-2 text-sm text-muted">{skill.useWhen}</span>{/if}
              </li>
            {/each}
          </ul>
        {/if}
      </Panel>
      <Panel title="Details">
        <dl class="grid gap-x-10 gap-y-5 sm:grid-cols-2">
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
          {#if card.reviewVerdict}{@render item('AI review', verdictLabel(card.reviewVerdict), false, card.reviewVerdict === 'failed')}{/if}
          {#if card.reviewedSha}{@render item('Reviewed SHA', card.reviewedSha, true)}{/if}
          {#if card.reviewIndependent !== null}{@render item('Review independent', card.reviewIndependent ? 'yes' : 'no')}{/if}
          {#if card.approvedSha}{@render item('Approved SHA', card.approvedSha, true)}{/if}
          {#if card.deployedSha}{@render item('Deployed SHA', card.deployedSha, true)}{/if}
          {#if card.failureReason}{@render item('Failure reason', reasonLabel(card.failureReason))}{/if}
          {#if card.blockedReason}{@render item('Blocked reason', reasonLabel(card.blockedReason), false, true)}{/if}
        </dl>
      </Panel>
      <CardChanges {diff} error={diffError} />
    </div>
  </Page>
{/if}
