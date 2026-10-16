<script lang="ts">
  import type { Snippet } from 'svelte';
  import { page } from '$app/stores';
  import { pickParam } from '$lib/filter';
  import { count } from '$lib/format';
  import { failureText } from '$lib/forms';
  import { setParams } from '$lib/params';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type {
    ProjectSummary,
    SummaryAttention,
    SummaryGroups,
    SummaryRange,
    TribeProjectSummary,
    TribeSummary
  } from '$lib/types';
  import { rangeOptions, rangeValues, rangeWindow } from '$lib/usage';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';
  import Table from '$lib/ui/Table.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  type Report = ProjectSummary | TribeSummary;

  let {
    title,
    description,
    load,
    actions,
    usageHref = '',
    showProjects = false,
    showMcp = false
  }: {
    title: string;
    description: string;
    load: (from: string, to: string) => Promise<Report>;
    actions?: Snippet;
    usageHref?: string;
    showProjects?: boolean;
    showMcp?: boolean;
  } = $props();

  let report = $state<Report | null>(null);
  let loading = $state('Loading overview');
  let loadError = $state('');
  let attempt = $state(0);

  showShell(() => ({ title }));

  const params = $derived($page.url.searchParams);
  const range = $derived(pickParam(params.get('range'), rangeValues, '30'));

  const groups = $derived(report?.groups);
  const attention = $derived(report?.attention);
  const rangeBlock = $derived(report?.range);
  const aging = $derived(report && 'aging' in report ? report.aging : []);
  const projects = $derived(report && 'projects' in report ? report.projects : []);
  const mcp = $derived(report && 'mcp' in report ? report.mcp : null);

  const groupRows: { key: keyof SummaryGroups; label: string }[] = [
    { key: 'waiting', label: 'Waiting' },
    { key: 'question', label: 'Needs answer' },
    { key: 'running', label: 'Running' },
    { key: 'review', label: 'Review' },
    { key: 'shipping', label: 'Shipping' },
    { key: 'finished', label: 'Finished' },
    { key: 'stopped', label: 'Blocked' },
    { key: 'closed', label: "Won't fix" }
  ];

  function groupTotal(g: SummaryGroups | undefined) {
    if (!g) return 0;
    return groupRows.reduce((sum, row) => sum + g[row.key], 0);
  }

  function peakGroup(g: SummaryGroups | undefined) {
    return Math.max(1, ...groupRows.map((row) => g?.[row.key] ?? 0));
  }

  function retry() {
    loading = 'Loading overview';
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

{#snippet totals(att: SummaryAttention | undefined, rng: SummaryRange | undefined)}
  <dl class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
    <div class="rounded-panel border border-line bg-panel px-3 py-2">
      <dt class="text-xs text-muted">Ready to run</dt>
      <dd class="mt-0.5 text-lg font-semibold tabular-nums text-ink">{count(att?.readyCount ?? 0)}</dd>
    </div>
    <div class="rounded-panel border border-line bg-panel px-3 py-2">
      <dt class="text-xs text-muted">Needs answer</dt>
      <dd class="mt-0.5 text-lg font-semibold tabular-nums text-ink">{count(att?.waitingAnswer ?? 0)}</dd>
    </div>
    <div class="rounded-panel border border-line bg-panel px-3 py-2">
      <dt class="text-xs text-muted">Human review</dt>
      <dd class="mt-0.5 text-lg font-semibold tabular-nums text-ink">{count(att?.humanReview ?? 0)}</dd>
    </div>
    <div class="rounded-panel border border-line bg-panel px-3 py-2">
      <dt class="text-xs text-muted">Queue / cap</dt>
      <dd class="mt-0.5 text-lg font-semibold tabular-nums text-ink"
        >{count(att?.openRuns ?? 0)} / {count(att?.implementationCap ?? 0)}</dd
      >
    </div>
    <div class="rounded-panel border border-line bg-panel px-3 py-2">
      <dt class="text-xs text-muted">Done in range</dt>
      <dd class="mt-0.5 text-lg font-semibold tabular-nums text-ink">{count(rng?.done ?? 0)}</dd>
    </div>
    <div class="rounded-panel border border-line bg-panel px-3 py-2">
      <dt class="text-xs text-muted">Rework in range</dt>
      <dd class="mt-0.5 text-lg font-semibold tabular-nums text-ink">{count(rng?.rework ?? 0)}</dd>
    </div>
  </dl>
{/snippet}

{#snippet groupBars(g: SummaryGroups | undefined, label: string)}
  {@const peak = peakGroup(g)}
  {@const total = groupTotal(g)}
  <Panel title={label}>
    {#if total === 0}
      <p class="text-sm text-muted">No cards in this scope.</p>
    {:else}
      <ul class="flex flex-col gap-2" aria-label={label}>
        {#each groupRows as row (row.key)}
          {@const n = g?.[row.key] ?? 0}
          {#if n > 0}
            <li class="flex items-center gap-3 text-sm">
              <span class="w-28 shrink-0 text-muted">{row.label}</span>
              <div class="h-2 min-w-0 flex-1 rounded-full bg-line" role="presentation">
                <div
                  class="h-2 rounded-full bg-accent"
                  style={`width: ${Math.max(4, (n / peak) * 100)}%`}
                ></div>
              </div>
              <span class="w-8 shrink-0 text-right tabular-nums text-ink">{n}</span>
            </li>
          {/if}
        {/each}
      </ul>
      <p class="sr-only">Total cards: {total}</p>
    {/if}
  </Panel>
{/snippet}

<Page {title} {description} {actions} {loading} error={loadError} onretry={retry}>
  <div class="flex flex-col gap-6">
    <Toolbar>
      <Segmented
        label="Range"
        options={rangeOptions}
        value={range}
        onchange={(next) => setParams({ range: next === '30' ? '' : next })}
      />
      {#if usageHref}
        <a class="text-sm font-medium text-accent underline-offset-2 hover:underline" href={usageHref}
          >Token usage</a
        >
      {/if}
    </Toolbar>

    {#if report}
      {@render totals(attention, rangeBlock)}

      <div class="grid gap-6 lg:grid-cols-2">
        {@render groupBars(groups, 'Pipeline')}

        <Panel title="Attention">
          <dl class="grid grid-cols-2 gap-3 text-sm">
            <div>
              <dt class="text-muted">Open questions</dt>
              <dd class="font-semibold tabular-nums">{count(attention?.openQuestions ?? 0)}</dd>
            </div>
            <div>
              <dt class="text-muted">Locked cards</dt>
              <dd class="font-semibold tabular-nums">{count(attention?.locked ?? 0)}</dd>
            </div>
          </dl>
          {#if (attention?.blocked.length ?? 0) > 0}
            <h3 class="mt-4 text-xs font-semibold uppercase tracking-wide text-muted">Blocked by</h3>
            <ul class="mt-2 flex flex-col gap-1 text-sm">
              {#each attention?.blocked ?? [] as item (item.reason)}
                <li class="flex justify-between gap-2">
                  <span class="font-mono text-ink">{item.reason}</span>
                  <span class="tabular-nums text-muted">{item.count}</span>
                </li>
              {/each}
            </ul>
          {:else}
            <p class="mt-3 text-sm text-muted">No blocked cards.</p>
          {/if}
        </Panel>
      </div>

      <div class="grid gap-6 lg:grid-cols-2">
        <Panel title="In this range">
          <p class="mb-3 text-xs text-muted">
            {report.from} → {report.to} (UTC)
          </p>
          <dl class="grid grid-cols-2 gap-3 text-sm">
            <div>
              <dt class="text-muted">Done</dt>
              <dd class="font-semibold tabular-nums">{count(rangeBlock?.done ?? 0)}</dd>
            </div>
            <div>
              <dt class="text-muted">Won't fix</dt>
              <dd class="font-semibold tabular-nums">{count(rangeBlock?.wontFix ?? 0)}</dd>
            </div>
            <div>
              <dt class="text-muted">Rework</dt>
              <dd class="font-semibold tabular-nums">{count(rangeBlock?.rework ?? 0)}</dd>
            </div>
            <div>
              <dt class="text-muted">Median answer</dt>
              <dd class="font-semibold tabular-nums">
                {rangeBlock?.medianAnswerHours == null
                  ? '—'
                  : `${rangeBlock.medianAnswerHours} h`}
              </dd>
            </div>
          </dl>
          <h3 class="mt-4 text-xs font-semibold uppercase tracking-wide text-muted">Answers</h3>
          <ul class="mt-2 flex flex-col gap-1 text-sm">
            <li class="flex justify-between gap-2">
              <span>Person</span><span class="tabular-nums">{rangeBlock?.answerSources.person ?? 0}</span>
            </li>
            <li class="flex justify-between gap-2">
              <span>Suggestion</span
              ><span class="tabular-nums">{rangeBlock?.answerSources.suggestion ?? 0}</span>
            </li>
            <li class="flex justify-between gap-2">
              <span>Auto</span><span class="tabular-nums">{rangeBlock?.answerSources.auto ?? 0}</span>
            </li>
          </ul>
          {#if (rangeBlock?.budgetStops.length ?? 0) > 0}
            <h3 class="mt-4 text-xs font-semibold uppercase tracking-wide text-muted">Budget stops</h3>
            <ul class="mt-2 flex flex-col gap-1 text-sm">
              {#each rangeBlock?.budgetStops ?? [] as stop (stop.cap)}
                <li class="flex justify-between gap-2">
                  <span class="font-mono">{stop.cap}</span>
                  <span class="tabular-nums">{stop.count}</span>
                </li>
              {/each}
            </ul>
          {/if}
        </Panel>

        {#if showMcp && mcp}
          <Panel title="MCP health">
            <dl class="grid grid-cols-1 gap-3 text-sm sm:grid-cols-3">
              <div>
                <dt class="text-muted">Schema changed</dt>
                <dd class="font-semibold tabular-nums">{count(mcp.changed)}</dd>
              </div>
              <div>
                <dt class="text-muted">Tool errors</dt>
                <dd class="font-semibold tabular-nums">{count(mcp.toolErrors)}</dd>
              </div>
              <div>
                <dt class="text-muted">Missing mentions</dt>
                <dd class="font-semibold tabular-nums">{count(mcp.missingMentions)}</dd>
              </div>
            </dl>
          </Panel>
        {:else if aging.length > 0}
          <Panel title="Oldest open cards">
            <Table label="Oldest open cards">
              <thead>
                <tr class={tr}>
                  <th class={th}>Card</th>
                  <th class={th}>Status</th>
                  <th class={th}>Updated</th>
                </tr>
              </thead>
              <tbody>
                {#each aging as card (card.cardId)}
                  <tr class={tr}>
                    <td class={td}>
                      <a class="font-medium text-accent hover:underline" href={`/cards/${card.cardId}`}
                        >{card.title}</a
                      >
                    </td>
                    <td class="{td} font-mono text-xs">{card.status}</td>
                    <td class="{td} text-muted text-xs tabular-nums">{card.updatedAt.slice(0, 10)}</td>
                  </tr>
                {/each}
              </tbody>
            </Table>
          </Panel>
        {:else}
          <Panel title="Oldest open cards">
            <EmptyState title="No open cards" description="Every card is done or won't fix." />
          </Panel>
        {/if}
      </div>

      {#if showProjects}
        <Panel title="Projects">
          {#if projects.length === 0}
            <EmptyState title="No projects" description="Create a project in this tribe to see WIP here." />
          {:else}
            <Table label="Projects">
              <thead>
                <tr class={tr}>
                  <th class={th}>Project</th>
                  <th class={th}>Waiting</th>
                  <th class={th}>Needs answer</th>
                  <th class={th}>Running</th>
                  <th class={th}>Review</th>
                  <th class={th}>Blocked</th>
                  <th class={th}>Ready</th>
                </tr>
              </thead>
              <tbody>
                {#each projects as project (project.projectId)}
                  {@const p = project as TribeProjectSummary}
                  <tr class={tr}>
                    <td class="{td} font-medium">{p.projectName}</td>
                    <td class="{td} tabular-nums">{p.groups.waiting}</td>
                    <td class="{td} tabular-nums">{p.groups.question}</td>
                    <td class="{td} tabular-nums">{p.groups.running}</td>
                    <td class="{td} tabular-nums">{p.groups.review}</td>
                    <td class="{td} tabular-nums">{p.groups.stopped}</td>
                    <td class="{td} tabular-nums">{p.attention.readyCount}</td>
                  </tr>
                {/each}
              </tbody>
            </Table>
          {/if}
        </Panel>
      {/if}
    {/if}
  </div>
</Page>
