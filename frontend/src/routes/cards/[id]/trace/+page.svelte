<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { page } from '$app/stores';
  import { ApiError, getCard, getTrace, unreachable } from '$lib/api';
  import TerminalView from '$lib/components/TerminalView.svelte';
  import TraceBlock from '$lib/components/TraceBlock.svelte';
  import TraceRow from '$lib/components/TraceRow.svelte';
  import { count, day, duration, stamp, tokens } from '$lib/format';
  import { pager } from '$lib/pager.svelte';
  import { setParams } from '$lib/params';
  import { shell, showShell } from '$lib/shell.svelte';
  import { cardTone, statusLabel } from '$lib/status';
  import {
    attemptOptions,
    buildTimeline,
    describe,
    filterEvents,
    kindOptions,
    mergeEvents,
    outcomeLabel,
    summarize,
    type Block
  } from '$lib/trace';
  import type { Card, TraceAttempt, TraceEvent } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import FilterSelect from '$lib/ui/FilterSelect.svelte';
  import Page from '$lib/ui/Page.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';
  import ShowMore from '$lib/ui/ShowMore.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  const views = [
    { value: 'structured', label: 'Structured' },
    { value: 'terminal', label: 'Terminal' }
  ];

  let events = $state.raw<TraceEvent[]>([]);
  let attempts = $state.raw<TraceAttempt[]>([]);
  let card = $state<Card | null>(null);
  let loaded = $state(false);
  let live = $state(false);
  let follow = $state(true);
  let now = $state(Date.now());
  let message = $state('Loading trace');
  let list = $state<HTMLOListElement | null>(null);
  let current = '';
  let inFlight = '';

  const cardId = $derived($page.params.id ?? '');

  showShell(() => ({ title: 'Trace', message }));

  const params = $derived($page.url.searchParams);
  const view = $derived(params.get('view') === 'terminal' ? 'terminal' : 'structured');
  /** How often the page reads the trace again while the card works. */
  const pollMs = $derived(view === 'terminal' ? 1000 : 3000);
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
    const total = timeline.length;
    untrack(() => paging.follow(total));
  });

  /** The attempt with no end, which the "Now running" strip follows. */
  const running = $derived(attempts.findLast((attempt) => !attempt.endedAt) ?? null);
  const phase = $derived.by(() => {
    const moved = events.findLast((event) => event.kind === 'transition');
    const to = (moved?.payload as { to?: unknown } | null)?.to;
    return typeof to === 'string' ? to : (card?.status ?? '');
  });
  const turn = $derived(
    running
      ? events.filter((event) => event.attemptId === running.id && event.kind === 'model_call' && (event.payload as { role?: unknown } | null)?.role === 'implementer').length
      : 0
  );
  const latest = $derived(events.at(-1) ?? null);
  const latestWords = $derived(latest ? describe(latest) : null);

  function age(ts: string) {
    const at = Date.parse(ts);
    return Number.isNaN(at) ? '' : duration(now - at);
  }

  function ended(before: TraceAttempt[], after: TraceAttempt[]) {
    const open = new Set(before.filter((attempt) => !attempt.endedAt).map((attempt) => attempt.id));
    return after.filter((attempt) => open.has(attempt.id) && attempt.endedAt);
  }

  async function showLatest() {
    await tick();
    list?.lastElementChild?.scrollIntoView({ block: 'end' });
  }

  /** Reads the trace; with `since`, merges the new events into the ones shown. A failed poll keeps the last good data. */
  async function load(id: string, since = '') {
    if (inFlight === id) return;
    inFlight = id;
    try {
      const body = await getTrace(id, since || undefined);
      if (id !== current) return;
      const before = events;
      const wasLive = live;
      const next = since ? mergeEvents(before, body.events) : body.events;
      const done = ended(attempts, body.attempts ?? []);
      events = next;
      attempts = body.attempts ?? [];
      live = body.live === true;
      loaded = true;
      if (done.length > 0) message = done.map((attempt) => `Attempt ${attempt.n} ended: ${outcomeLabel(attempt)}`).join('. ');
      else if (!since) message = 'Trace';
      if (since && follow && view === 'structured' && next.length > before.length) void showLatest();
      if (since && (done.length > 0 || wasLive !== live)) {
        void getCard(id)
          .then((body) => id === current && (card = body))
          .catch(() => {});
      }
    } catch (error: unknown) {
      if (id === current && !since) message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      if (inFlight === id) inFlight = '';
    }
  }

  $effect(() => {
    const id = $page.params.id;
    if (!id || shell.projectId === '') return;
    current = id;
    events = [];
    attempts = [];
    live = false;
    loaded = false;
    void load(id);
    void getCard(id)
      .then((body) => id === current && (card = body))
      .catch(() => id === current && (card = null));
  });

  $effect(() => {
    if (!live) return;
    const id = cardId;
    const every = pollMs;
    now = Date.now();
    const timer = setInterval(() => void load(id, events.at(-1)?.ts ?? ''), every);
    const clock = setInterval(() => (now = Date.now()), 1000);
    return () => {
      clearInterval(timer);
      clearInterval(clock);
    };
  });
</script>

<Page title="Trace" description={card ? card.title : `Events for ${cardId}, oldest first.`}>
  {#snippet meta()}
    {#if card}
      <span title={card.status}><Badge tone={cardTone(card)}>{statusLabel(card.status)}</Badge></span>
    {/if}
    {#if live}
      <span title="This page reads new events every few seconds"><Badge tone="sky">Live</Badge></span>
    {/if}
  {/snippet}
  {#snippet actions()}
    <Button href={`/cards/${cardId}`}>Back to card</Button>
  {/snippet}
  {#if live}
    <section aria-label="Now running" class="mb-4 flex flex-wrap items-baseline gap-x-6 gap-y-2 rounded-panel border border-sky-200 bg-sky-50/60 px-4 py-3 text-sm">
      <p class="font-semibold text-sky-900">{running ? `Attempt ${running.n}` : 'Waiting for the worker'}</p>
      <dl class="contents">
        {#if phase}
          <div class="flex items-baseline gap-1.5">
            <dt class="text-muted">Phase</dt>
            <dd class="text-ink">{statusLabel(phase)}</dd>
          </div>
        {/if}
        {#if running}
          <div class="flex items-baseline gap-1.5">
            <dt class="text-muted">Elapsed</dt>
            <dd class="font-mono text-ink">{age(running.startedAt)}</dd>
          </div>
          <div class="flex items-baseline gap-1.5">
            <dt class="text-muted">Turn</dt>
            <dd class="text-ink">{count(turn)}{running.maxTurns ? ` of ${count(running.maxTurns)}` : ''}</dd>
          </div>
          <div class="flex items-baseline gap-1.5">
            <dt class="text-muted">Tokens</dt>
            <dd class="text-ink">{tokens(running.inputTokens + running.outputTokens)}</dd>
          </div>
        {/if}
        {#if latest && latestWords}
          <div class="flex min-w-0 basis-full items-baseline gap-1.5">
            <dt class="shrink-0 text-muted">Last</dt>
            <dd class="min-w-0 truncate text-ink">
              {latestWords.label}{latestWords.detail ? ` · ${latestWords.detail}` : ''}
              <span class="text-muted">· {age(latest.ts)} ago</span>
            </dd>
          </div>
        {/if}
      </dl>
    </section>
  {/if}
  {#if loaded && events.length === 0}
    <EmptyState title="No events yet" description={live ? 'Events show here as the worker writes them.' : ''} />
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
        <Segmented label="View" options={views} value={view} onchange={(next) => setParams({ view: next === 'terminal' ? 'terminal' : '' })} />
        <SearchBox label="Search events" value={filter.q} placeholder="Search events, tools, files, or errors" />
        <FilterSelect label="Kind" options={kinds} value={filter.kind} onchange={(kind) => setParams({ kind })} />
        {#if attemptChoices.length > 2}
          <FilterSelect label="Attempt" options={attemptChoices} value={filter.attempt} onchange={(attempt) => setParams({ attempt })} />
        {/if}
        {#if live}
          <label class="flex min-h-11 items-center gap-2 text-sm text-ink">
            <input type="checkbox" class="size-4 accent-accent" bind:checked={follow} />
            Follow latest
          </label>
        {/if}
      </Toolbar>
      {#if shown.length === 0}
        <EmptyState title="No events match" description="Change the search or filters to see more events.">
          <Button type="button" variant="secondary" onclick={() => setParams({ q: '', kind: '', attempt: '' })}>Clear filters</Button>
        </EmptyState>
      {:else if view === 'terminal'}
        <TerminalView events={shown} {attempts} bind:follow />
      {:else}
        <ol bind:this={list} class="flex flex-col gap-2" aria-label="Events, oldest first">
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
