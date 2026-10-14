<script lang="ts">
  import type { Snippet } from 'svelte';
  import { page } from '$app/stores';
  import { pickParam } from '$lib/filter';
  import { count, day, tokens } from '$lib/format';
  import { failureText } from '$lib/forms';
  import { pager } from '$lib/pager.svelte';
  import { setParams } from '$lib/params';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { UsageResponse } from '$lib/types';
  import {
    dailyTotals,
    dimensionOptionsFor,
    dimensionValues,
    filterUsage,
    groupUsage,
    none,
    rangeOptions,
    rangeValues,
    rangeWindow,
    sortGroups,
    sortValues,
    usageOptions,
    usageSummary,
    type Dimension,
    type SortDir,
    type SortKey,
    type UsageFilter
  } from '$lib/usage';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import FilterSelect from '$lib/ui/FilterSelect.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';
  import ShowMore from '$lib/ui/ShowMore.svelte';
  import Table from '$lib/ui/Table.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  let {
    title,
    description,
    load,
    dimensions = dimensionValues,
    actions
  }: {
    title: string;
    description: string;
    load: (from: string, to: string) => Promise<UsageResponse>;
    dimensions?: readonly Dimension[];
    actions?: Snippet;
  } = $props();

  let report = $state<UsageResponse | null>(null);
  let loading = $state('Loading token usage');
  let loadError = $state('');
  let attempt = $state(0);

  showShell(() => ({ title }));

  const params = $derived($page.url.searchParams);
  const range = $derived(pickParam(params.get('range'), rangeValues, '30'));
  const by = $derived(pickParam(params.get('by'), dimensions, 'model'));
  const sort = $derived(pickParam(params.get('sort'), sortValues, 'total'));
  const dir = $derived(pickParam<SortDir>(params.get('dir'), ['asc', 'desc'], sort === 'name' ? 'asc' : 'desc'));
  const filter = $derived(
    Object.fromEntries(dimensionValues.map((key) => [key, dimensions.includes(key) ? (params.get(key) ?? '') : ''])) as UsageFilter
  );
  const filtered = $derived(dimensionValues.some((key) => filter[key] !== ''));

  const rows = $derived(report?.rows ?? []);
  const shown = $derived(filterUsage(rows, filter));
  const summary = $derived(usageSummary(shown));
  const groups = $derived(sortGroups(groupUsage(shown, by), sort, dir));
  const days = $derived(report ? dailyTotals(shown, report.from, report.to) : []);
  const peak = $derived(Math.max(1, ...days.map((entry) => entry.total)));
  const paging = pager(() => `${range}|${by}|${sort}|${dir}|${dimensionValues.map((key) => filter[key]).join('|')}`);

  const groupOptions = $derived(dimensionOptionsFor(dimensions));
  const columns: { key: SortKey; label: string }[] = [
    { key: 'calls', label: 'Calls' },
    { key: 'input', label: 'Input' },
    { key: 'output', label: 'Output' },
    { key: 'cached', label: 'Cached' },
    { key: 'total', label: 'Total' }
  ];
  const groupLabel = $derived(groupOptions.find((option) => option.value === by)?.label ?? 'Model');
  const sortTh = th.replace('h-10 ', '');

  function sortBy(key: SortKey) {
    const first: SortDir = key === 'name' ? 'asc' : 'desc';
    const next: SortDir = key === sort ? (dir === 'asc' ? 'desc' : 'asc') : first;
    setParams({ sort: key === 'total' ? '' : key, dir: next === first ? '' : next });
  }

  function ariaSort(key: SortKey) {
    if (key !== sort) return 'none';
    return dir === 'asc' ? 'ascending' : 'descending';
  }

  function share(total: number) {
    return summary.total === 0 ? 0 : total / summary.total;
  }

  function percent(part: number) {
    return `${Math.round(part * 100)}%`;
  }

  function retry() {
    loading = 'Loading token usage';
    loadError = '';
    attempt += 1;
  }

  $effect(() => {
    void attempt;
    const { from, to } = rangeWindow(Number(range), new Date());
    let current = true;
    void load(from, to)
      .then((body) => {
        if (!current) return;
        report = body;
      })
      .catch((error: unknown) => {
        if (!current) return;
        if (report) toast.error(failureText(error));
        else loadError = failureText(error);
      })
      .finally(() => {
        if (current) loading = '';
      });
    return () => {
      current = false;
    };
  });
</script>

<Page {title} {description} {actions} {loading} error={loadError} onretry={retry}>
  <div class="flex flex-col gap-4">
    <Toolbar>
      <Segmented label="Range" options={rangeOptions} value={range} onchange={(next) => setParams({ range: next === '30' ? '' : next })} />
      <Segmented label="Group by" options={groupOptions} value={by} onchange={(next) => setParams({ by: next === 'model' ? '' : next })} />
    </Toolbar>
    {#if report && rows.length === 0}
      <EmptyState title="No model calls in this range" description="Calls show here after a job, review, or refine talks to a model." />
    {:else if report}
      <Toolbar>
        {#each groupOptions as item (item.value)}
          <FilterSelect
            label={item.label}
            options={usageOptions(rows, filter, item.value)}
            value={filter[item.value]}
            onchange={(value) => setParams({ [item.value]: value })}
          />
        {/each}
        {#if filtered}
          <Button
            type="button"
            variant="secondary"
            icon="close"
            onclick={() => setParams({ llm: '', model: '', project: '', user: '', task: '', source: '', card: '' })}>Clear filters</Button
          >
        {/if}
      </Toolbar>
      {#if shown.length === 0}
        <EmptyState title="No calls match" description="Change the filters to see more calls.">
          <Button
            type="button"
            variant="secondary"
            icon="close"
            onclick={() => setParams({ llm: '', model: '', project: '', user: '', task: '', source: '', card: '' })}>Clear filters</Button
          >
        </EmptyState>
      {:else}
        <dl class="flex flex-wrap gap-x-6 gap-y-2 rounded-panel border border-line bg-surface px-4 py-3 text-sm" aria-label="Totals">
          <div class="flex items-baseline gap-1.5">
            <dt class="text-muted">Total tokens</dt>
            <dd class="font-semibold text-ink" title={count(summary.total)}>{tokens(summary.total)}</dd>
          </div>
          <div class="flex items-baseline gap-1.5">
            <dt class="text-muted">Input</dt>
            <dd class="font-semibold text-ink" title={count(summary.inputTokens)}>{tokens(summary.inputTokens)}</dd>
          </div>
          <div class="flex items-baseline gap-1.5">
            <dt class="text-muted">Output</dt>
            <dd class="font-semibold text-ink" title={count(summary.outputTokens)}>{tokens(summary.outputTokens)}</dd>
          </div>
          <div class="flex items-baseline gap-1.5">
            <dt class="text-muted">Cached input</dt>
            <dd class="font-semibold text-ink" title={count(summary.cachedInputTokens)}>{tokens(summary.cachedInputTokens)}</dd>
          </div>
          <div class="flex items-baseline gap-1.5">
            <dt class="text-muted">Calls</dt>
            <dd class="font-semibold text-ink">{count(summary.calls)}</dd>
          </div>
          <div class="flex items-baseline gap-1.5">
            <dt class="text-muted">Cache share</dt>
            <dd class="font-semibold text-ink">{percent(summary.cacheShare)}</dd>
          </div>
        </dl>

        <section class="rounded-panel border border-line bg-surface px-4 py-3" aria-labelledby="usage-days">
          <h2 id="usage-days" class="text-sm font-medium text-ink">Tokens per day</h2>
          <div class="mt-3 flex h-28 items-end gap-px" aria-hidden="true">
            {#each days as entry (entry.day)}
              <div
                class="min-w-0 flex-1 rounded-t-sm {entry.total > 0 ? 'bg-accent/70 hover:bg-accent' : 'bg-line'}"
                style:height={entry.total > 0 ? `${Math.max(3, (entry.total / peak) * 100)}%` : '1px'}
                title={`${day(`${entry.day}T12:00:00Z`)}: ${count(entry.total)} tokens, ${count(entry.calls)} calls`}
              ></div>
            {/each}
          </div>
          {#if days.length > 0}
            <p class="mt-2 flex justify-between text-xs text-muted" aria-hidden="true">
              <span>{day(`${days[0].day}T12:00:00Z`)}</span>
              <span>{day(`${days[days.length - 1].day}T12:00:00Z`)}</span>
            </p>
          {/if}
          <table class="sr-only">
            <caption>Tokens per day</caption>
            <thead><tr><th scope="col">Day</th><th scope="col">Total tokens</th><th scope="col">Calls</th></tr></thead>
            <tbody>
              {#each days as entry (entry.day)}
                <tr><th scope="row">{entry.day}</th><td>{entry.total}</td><td>{entry.calls}</td></tr>
              {/each}
            </tbody>
          </table>
        </section>

        <Table label={`Token usage by ${groupLabel.toLowerCase()}`}>
          <thead>
            <tr class="border-b border-line bg-ground/50">
              <th class={sortTh} scope="col" aria-sort={ariaSort('name')}>
                <button type="button" class="inline-flex min-h-11 items-center gap-1 px-2 uppercase hover:text-ink" onclick={() => sortBy('name')}>
                  {groupLabel}{#if sort === 'name'}<span aria-hidden="true">{dir === 'asc' ? '↑' : '↓'}</span>{/if}
                </button>
              </th>
              {#each columns as column (column.key)}
                <th class="{sortTh} text-right" scope="col" aria-sort={ariaSort(column.key)}>
                  <button
                    type="button"
                    class="inline-flex min-h-11 items-center gap-1 px-2 uppercase hover:text-ink"
                    onclick={() => sortBy(column.key)}
                  >
                    {column.label}{#if sort === column.key}<span aria-hidden="true">{dir === 'asc' ? '↑' : '↓'}</span>{/if}
                  </button>
                </th>
              {/each}
              <th class="{th} w-40" scope="col">Share</th>
            </tr>
          </thead>
          <tbody>
            {#each groups.slice(0, paging.limit) as group (group.key)}
              <tr class={tr}>
                <td class="{td} font-medium text-ink" title={by === 'card' && group.key !== none ? group.key : undefined}>
                  <span class="break-all">{group.label}</span>
                </td>
                <td class="{td} text-right tabular-nums">{count(group.calls)}</td>
                <td class="{td} text-right tabular-nums" title={count(group.inputTokens)}>{tokens(group.inputTokens)}</td>
                <td class="{td} text-right tabular-nums" title={count(group.outputTokens)}>{tokens(group.outputTokens)}</td>
                <td class="{td} text-right tabular-nums text-muted" title={count(group.cachedInputTokens)}>{tokens(group.cachedInputTokens)}</td>
                <td class="{td} text-right font-medium tabular-nums text-ink" title={count(group.total)}>{tokens(group.total)}</td>
                <td class={td}>
                  <span class="flex items-center gap-2">
                    <span class="h-1.5 flex-1 overflow-hidden rounded-full bg-ground" aria-hidden="true">
                      <span class="block h-full rounded-full bg-accent" style:width={percent(share(group.total))}></span>
                    </span>
                    <span class="w-10 text-right text-xs tabular-nums text-muted">{percent(share(group.total))}</span>
                  </span>
                </td>
              </tr>
            {/each}
          </tbody>
        </Table>
        <ShowMore total={groups.length} limit={paging.limit} noun={['group', 'groups']} onmore={paging.more} />
      {/if}
    {/if}
  </div>
</Page>
