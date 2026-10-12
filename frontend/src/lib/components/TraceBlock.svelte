<script lang="ts">
  import { untrack } from 'svelte';
  import { pageSize } from '$lib/filter';
  import { between, count, duration, tokens } from '$lib/format';
  import { outcomeLabel, outcomeTone, verdictLabel, verdictTone, type Block } from '$lib/trace';
  import { sentence } from '$lib/status';
  import Badge from '$lib/ui/Badge.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import ShowMore from '$lib/ui/ShowMore.svelte';
  import TraceRow from './TraceRow.svelte';

  let { block, startOpen = false }: { block: Block; startOpen?: boolean } = $props();

  let open = $state(untrack(() => startOpen));
  let limit = $state(pageSize);

  const bodyId = $derived(`trace-block-${block.key}`);
  const attempt = $derived(block.attempt);
  const number = $derived(attempt ? String(attempt.n) : block.attemptId);
  const title = $derived(
    `${block.review ? `Review of attempt ${number}` : `Attempt ${number}`}${block.continued ? ' (continued)' : ''}`
  );
  const model = $derived(block.review ? attempt?.reviewModelId : attempt?.modelId);
  const took = $derived.by(() => {
    const ms =
      !block.review && !block.continued && attempt?.startedAt && attempt.endedAt
        ? between(attempt.startedAt, attempt.endedAt)
        : between(block.start, block.end);
    return ms === null ? '' : duration(ms);
  });
  const stats = $derived(
    [
      block.models > 0 ? `${count(block.models)} ${block.models === 1 ? 'model call' : 'model calls'}` : '',
      block.tools > 0 ? `${count(block.tools)} ${block.tools === 1 ? 'tool' : 'tools'}` : '',
      block.models > 0 ? `${tokens(block.inputTokens)} in / ${tokens(block.outputTokens)} out` : '',
      took
    ].filter(Boolean)
  );
</script>

<li class="overflow-hidden rounded-panel border {block.problems > 0 ? 'border-red-200' : 'border-line'} bg-surface">
  <button
    type="button"
    class="flex min-h-12 w-full flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-left hover:bg-ground/70"
    aria-expanded={open}
    aria-controls={bodyId}
    onclick={() => (open = !open)}
  >
    <Icon name="chevron" class="size-4 shrink-0 text-muted transition-transform {open ? 'rotate-180' : ''}" />
    <span class="text-sm font-semibold text-ink">{title}</span>
    {#if attempt && !block.review}<span class="text-sm text-muted">{sentence(attempt.taskClass)}</span>{/if}
    {#if model}<span class="font-mono text-xs text-muted">{model}</span>{/if}
    <span class="flex flex-wrap items-center gap-2 sm:ml-auto">
      {#each stats as stat (stat)}<span class="text-xs whitespace-nowrap text-muted">{stat}</span>{/each}
      {#if block.problems > 0}<Badge tone="red">{block.problems} {block.problems === 1 ? 'problem' : 'problems'}</Badge>{/if}
      {#if attempt && block.review}
        {#if attempt.reviewVerdict}
          <Badge tone={verdictTone(attempt.reviewVerdict)}>{verdictLabel(attempt.reviewVerdict)}</Badge>
        {/if}
        {#if attempt.reviewIndependent === false && attempt.reviewVerdict !== 'failed'}<Badge>Not independent</Badge>{/if}
      {:else if attempt && !block.continued}
        <Badge tone={outcomeTone(attempt)}>{outcomeLabel(attempt)}</Badge>
        {#if attempt.commitSha}<span class="font-mono text-xs text-muted" title={attempt.commitSha}>{attempt.commitSha.slice(0, 7)}</span>{/if}
      {/if}
    </span>
  </button>
  {#if open}
    <div id={bodyId} class="border-t border-line">
      <ul class="flex flex-col divide-y divide-line/70">
        {#each block.events.slice(0, limit) as event (event.id)}
          <TraceRow {event} />
        {/each}
      </ul>
      {#if block.events.length > limit}
        <div class="border-t border-line px-3 py-2">
          <ShowMore total={block.events.length} {limit} noun={['event', 'events']} onmore={() => (limit += pageSize)} />
        </div>
      {/if}
    </div>
  {/if}
</li>
