<script lang="ts">
  import { sourceLabel } from '$lib/intake';
  import type { Requirement } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import { td } from '$lib/ui/control';
  import Icon from '$lib/ui/Icon.svelte';

  let {
    requirement,
    selectable = false,
    selected = false,
    columns,
    onselect
  }: {
    requirement: Requirement;
    selectable?: boolean;
    selected?: boolean;
    columns: number;
    onselect?: (checked: boolean) => void;
  } = $props();

  let open = $state(false);

  const detailId = $derived(`requirement-${requirement.id}`);
  const narrowHidden = 'max-[720px]:hidden';
  const reviewersHidden = 'max-[1280px]:hidden';
  const link = 'inline-flex min-h-11 items-center text-sm font-medium text-accent hover:underline';
</script>

<tr class="border-b border-line align-top hover:bg-ground/50 {open ? 'bg-ground/40' : ''}">
  {#if selectable}
    <td class="w-11 px-2 py-1">
      {#if !requirement.accepted}
        <label class="flex size-11 items-center justify-center">
          <input
            type="checkbox"
            class="size-4 accent-accent"
            checked={selected}
            aria-label={`Select ${requirement.id}`}
            onchange={(event) => onselect?.(event.currentTarget.checked)}
          />
        </label>
      {/if}
    </td>
  {/if}
  <td class="{td} w-24 py-3.5 font-mono text-xs whitespace-nowrap text-ink">{requirement.id}</td>
  <td class="{td} min-w-52 py-2.5">
    <button
      type="button"
      tabindex="-1"
      class="block w-full cursor-pointer text-left"
      onclick={() => (open = !open)}
    >
      <span class="line-clamp-2 text-sm text-ink">{requirement.statement}</span>
      {#if requirement.sourceKind === 'manual'}
        <span class="mt-0.5 line-clamp-1 text-xs break-all text-muted">Added by {requirement.createdBy ?? 'a former user'}</span>
      {:else}
        <span class="mt-0.5 line-clamp-1 font-mono text-xs break-all text-muted">{requirement.sourcePath}</span>
      {/if}
    </button>
  </td>
  <td class="{td} py-3 {narrowHidden}">
    <Badge tone={requirement.sourceKind === 'draft' ? 'amber' : 'default'}>{sourceLabel(requirement.sourceKind)}</Badge>
  </td>
  <td class="{td} py-3.5 text-sm text-muted {reviewersHidden}">
    {requirement.reviewers.length > 0 ? requirement.reviewers.length : '—'}
  </td>
  <td class="{td} py-2 {narrowHidden}">
    {#if requirement.linkedCards.length === 0}
      <span class="inline-flex min-h-11 items-center text-sm text-muted">—</span>
    {:else}
      <span class="flex flex-wrap items-center gap-x-2">
        <a class="inline-flex min-h-11 items-center font-mono text-xs text-accent hover:underline" href={`/cards/${requirement.linkedCards[0]}`}
          >{requirement.linkedCards[0]}</a
        >
        {#if requirement.linkedCards.length > 1}
          <span class="text-xs text-muted" title={requirement.linkedCards.slice(1).join(', ')}>+{requirement.linkedCards.length - 1}</span>
        {/if}
      </span>
    {/if}
  </td>
  <td class="{td} py-3 whitespace-nowrap">
    <Badge tone={requirement.accepted ? 'green' : 'default'}>{requirement.accepted ? 'Accepted' : 'Not accepted'}</Badge>
  </td>
  <td class="w-12 px-1 py-1 text-right">
    <button
      type="button"
      class="inline-flex size-11 items-center justify-center rounded-control text-muted hover:bg-ground hover:text-ink"
      aria-expanded={open}
      aria-controls={detailId}
      aria-label={`Details for ${requirement.id}`}
      onclick={() => (open = !open)}
    >
      <Icon name="chevron" class="size-4 transition-transform {open ? 'rotate-180' : ''}" />
    </button>
  </td>
</tr>
{#if open}
  <tr id={detailId} class="border-b border-line bg-ground/40">
    <td colspan={columns} class="px-4 pt-1 pb-5">
      <div class="flex max-w-3xl flex-col gap-4 {selectable ? 'sm:pl-11' : ''}">
        <p class="text-sm leading-relaxed text-ink">{requirement.statement}</p>
        {#if requirement.acceptance.length > 0}
          <ul class="flex flex-col gap-1.5" aria-label={`Acceptance for ${requirement.id}`}>
            {#each requirement.acceptance as line, index (index)}
              <li class="flex items-start gap-2 text-sm text-ink">
                <Icon name="check" class="mt-0.5 size-4 shrink-0 text-muted" />
                {line}
              </li>
            {/each}
          </ul>
        {/if}
        <dl class="grid grid-cols-[auto_1fr] gap-x-6 gap-y-1.5 text-sm">
          {#if requirement.sourceKind === 'manual'}
            <dt class="text-muted">Added by</dt>
            <dd class="text-ink">{requirement.createdBy ?? 'a former user'}</dd>
          {:else}
            <dt class="text-muted">Source</dt>
            <dd class="font-mono text-xs leading-5 break-all text-ink">{requirement.sourcePath}</dd>
            {#if requirement.createdBy}
              <dt class="text-muted">Ingested by</dt>
              <dd class="text-ink">{requirement.createdBy}</dd>
            {/if}
          {/if}
          <dt class="text-muted">Reviewers</dt>
          <dd>
            {#if requirement.reviewers.length === 0}
              <span class="text-muted">None</span>
            {:else}
              <ul class="flex flex-wrap gap-1.5" aria-label={`Reviewers of ${requirement.id}`}>
                {#each requirement.reviewers as reviewer (reviewer.id)}
                  <li><Badge>{reviewer.githubLogin || reviewer.email}</Badge></li>
                {/each}
              </ul>
            {/if}
          </dd>
          <dt class="text-muted">Cards</dt>
          <dd>
            {#if requirement.linkedCards.length === 0}
              <span class="text-muted">None yet</span>
            {:else}
              <span class="flex flex-wrap gap-x-3">
                {#each requirement.linkedCards as id (id)}
                  <a class="font-mono text-xs leading-5 text-accent hover:underline" href={`/cards/${id}`}>{id}</a>
                {/each}
              </span>
            {/if}
          </dd>
        </dl>
        {#if requirement.sourceKind === 'manual'}
          <a class={link} href={`/intake/${encodeURIComponent(requirement.id)}`} aria-label={`Edit ${requirement.id}`}>Edit requirement</a>
        {:else if requirement.canEditReviewers}
          <a class={link} href={`/intake/${encodeURIComponent(requirement.id)}`} aria-label={`Reviewers for ${requirement.id}`}>Edit reviewers</a>
        {/if}
      </div>
    </td>
  </tr>
{/if}
