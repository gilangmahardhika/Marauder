<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getCard, getTrace, unreachable } from '$lib/api';
  import TraceBlock from '$lib/components/TraceBlock.svelte';
  import TraceRow from '$lib/components/TraceRow.svelte';
  import { count, day, stamp, tokens } from '$lib/format';
  import { pager } from '$lib/pager.svelte';
  import { setParams } from '$lib/params';
  import { shell, showShell } from '$lib/shell.svelte';
  import { cardTone, statusLabel } from '$lib/status';
  import { attemptOptions, buildTimeline, filterEvents, kindOptions, summarize, type Block } from '$lib/trace';
  import type { Card, TraceAttempt, TraceEvent } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import FilterSelect from '$lib/ui/FilterSelect.svelte';
  import Page from '$lib/ui/Page.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import ShowMore from '$lib/ui/ShowMore.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  let events = $state<TraceEvent[]>([]);
  let attempts = $state<TraceAttempt[]>([]);
  let card = $state<Card | null>(null);
  let loaded = $state(false);
  let message = $state('Loading trace');

  const cardId = $derived($page.params.id ?? '');

  showShell(() => ({ title: 'Trace', message }));

  const params = $derived($page.url.searchParams);
  const kinds = $derived(kindOptions(events));
  const attemptChoices = $derived(attemptOptions(events, attempts));
  const filter = $derived({
    q: params.get('q') ?? '',
    kind: kinds.some((option) => option.value === params.get('kind')) ? (params.get('kind') ?? '') : '',
    attempt: attemptChoices.some((option) => option.value === params.get('attempt')) ? (params.get('attempt') ?? '') : ''
  });
  const filtering = $derived(filter.q.trim() !== '' || filter.kind !== '' || filter.attempt !== '');
  const shown = $derived(filterEvents(events, filter));
  const timeline = $derived(buildTimeline(shown, attempts));
  const lastBlock = $derived(timeline.findLastIndex((item) => item.type === 'block'));
  const summary = $derived(summarize(events, attempts));
  const paging = pager(() => `${filter.q}|${filter.kind}|${filter.attempt}`);

  type Section =
    | { type: 'day'; key: string; label: string }
    | { type: 'block'; key: string; block: Block; open: boolean }
    | { type: 'events'; key: string; events: TraceEvent[] };

  /** Day headers where the day changes, attempt blocks, and runs of milestones in one list. */
  const sections = $derived.by(() => {
    const out: Section[] = [];
    let current = '';
    timeline.slice(0, paging.limit).forEach((item, index) => {
      const ts = item.type === 'block' ? item.start : item.event.ts;
      const label = day(ts);
      if (label !== current) {
        out.push({ type: 'day', key: `day-${ts}`, label });
        current = label;
      }
      const last = out.at(-1);
      if (item.type === 'block') {
        out.push({ type: 'block', key: `${item.key}|${filtering}`, block: item, open: filtering || item.problems > 0 || index === lastBlock });
      } else if (last?.type === 'events') {
        last.events.push(item.event);
      } else {
        out.push({ type: 'events', key: `events-${item.key}`, events: [item.event] });
      }
    });
    return out;
  });

  $effect(() => {
    const id = $page.params.id;
    if (!id || shell.projectId === '') return;
    void getTrace(id)
      .then((body) => {
        events = body.events;
        attempts = body.attempts ?? [];
        loaded = true;
        message = 'Trace';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
    void getCard(id)
      .then((body) => (card = body))
      .catch(() => (card = null));
  });
</script>

<Page title="Trace" description={card ? card.title : `Events for ${cardId}, oldest first.`}>
  {#snippet meta()}
    {#if card}
      <span title={card.status}><Badge tone={cardTone(card)}>{statusLabel(card.status)}</Badge></span>
    {/if}
  {/snippet}
  {#snippet actions()}
    <Button href={`/cards/${cardId}`}>Back to card</Button>
  {/snippet}
  {#if loaded && events.length === 0}
    <EmptyState title="No events yet" />
  {:else if events.length > 0}
    <div class="flex flex-col gap-4">
      <dl class="flex flex-wrap gap-x-6 gap-y-2 rounded-panel border border-line bg-surface px-4 py-3 text-sm">
        <div class="flex items-baseline gap-1.5">
          <dt class="text-muted">Attempts</dt>
          <dd class="font-semibold text-ink">{count(summary.attempts)}</dd>
        </div>
        <div class="flex items-baseline gap-1.5">
          <dt class="text-muted">Events</dt>
          <dd class="font-semibold text-ink">{count(summary.events)}</dd>
        </div>
        <div class="flex items-baseline gap-1.5">
          <dt class="text-muted">Tokens</dt>
          <dd class="font-semibold text-ink" title={count(summary.tokens)}>{tokens(summary.tokens)}</dd>
        </div>
        <div class="flex items-baseline gap-1.5">
          <dt class="text-muted">Errors</dt>
          <dd class="font-semibold {summary.errors > 0 ? 'text-red-800' : 'text-ink'}">
            {#if summary.errors > 0}
              <a class="underline-offset-2 hover:underline" href={`?kind=job_error`}>{count(summary.errors)}</a>
            {:else}0{/if}
          </dd>
        </div>
        <div class="flex items-baseline gap-1.5">
          <dt class="text-muted">From</dt>
          <dd class="text-ink"><time datetime={summary.first} title={summary.first}>{stamp(summary.first)}</time></dd>
          <dt class="text-muted">to</dt>
          <dd class="text-ink"><time datetime={summary.last} title={summary.last}>{stamp(summary.last)}</time></dd>
        </div>
      </dl>
      <Toolbar>
        <SearchBox label="Search events" value={filter.q} placeholder="Search events, tools, files, or errors" />
        <FilterSelect label="Kind" options={kinds} value={filter.kind} onchange={(kind) => setParams({ kind })} />
        {#if attemptChoices.length > 2}
          <FilterSelect label="Attempt" options={attemptChoices} value={filter.attempt} onchange={(attempt) => setParams({ attempt })} />
        {/if}
      </Toolbar>
      {#if shown.length === 0}
        <EmptyState title="No events match" description="Change the search or filters to see more events.">
          <Button type="button" variant="secondary" onclick={() => setParams({ q: '', kind: '', attempt: '' })}>Clear filters</Button>
        </EmptyState>
      {:else}
        <ol class="flex flex-col gap-2" aria-label="Events, oldest first">
          {#each sections as section (section.key)}
            {#if section.type === 'day'}
              <li class="pt-3 text-[11px] font-semibold tracking-wider text-muted uppercase first:pt-0">{section.label}</li>
            {:else if section.type === 'block'}
              <TraceBlock block={section.block} startOpen={section.open} />
            {:else}
              <li class="overflow-hidden rounded-panel border border-line bg-surface">
                <ul class="flex flex-col divide-y divide-line/70">
                  {#each section.events as event (event.id)}<TraceRow {event} />{/each}
                </ul>
              </li>
            {/if}
          {/each}
        </ol>
        <ShowMore total={timeline.length} limit={paging.limit} noun={['entry', 'entries']} onmore={paging.more} />
      {/if}
    </div>
  {/if}
</Page>
