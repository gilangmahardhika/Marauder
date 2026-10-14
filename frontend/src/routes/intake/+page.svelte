<script lang="ts">
  import { untrack } from 'svelte';
  import { page } from '$app/stores';
  import { acceptRequirements, getIntake, ingest } from '$lib/api';
  import IntakeActions from '$lib/components/IntakeActions.svelte';
  import RequirementRow from '$lib/components/RequirementRow.svelte';
  import { failureText } from '$lib/forms';
  import {
    acceptableIds,
    filterDecisions,
    filterRequirements,
    intakeCounts,
    parseSource,
    parseStatus,
    sourceOptions,
    statusOptions
  } from '$lib/intake';
  import { pager } from '$lib/pager.svelte';
  import { setParams } from '$lib/params';
  import { shell, showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { Intake } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import FilterSelect from '$lib/ui/FilterSelect.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Page from '$lib/ui/Page.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';
  import ShowMore from '$lib/ui/ShowMore.svelte';
  import Spinner from '$lib/ui/Spinner.svelte';
  import Table from '$lib/ui/Table.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  let intake = $state<Intake | null>(null);
  let skipped = $state<string[]>([]);
  let loading = $state('Loading intake');
  let loadError = $state('');
  let pending = $state<'' | 'ingest' | 'all' | 'selected'>('');
  let selected = $state<string[]>([]);

  const busy = $derived(pending !== '');

  async function load(first = false) {
    if (first) {
      loading = 'Loading intake';
      loadError = '';
    }
    try {
      intake = await getIntake();
      loadError = '';
    } catch (error) {
      if (intake) toast.error(failureText(error));
      else loadError = failureText(error);
    } finally {
      loading = '';
    }
  }

  $effect(() => {
    if (shell.projectId === '') return;
    void load(true);
  });

  $effect(() => {
    if (!intake?.draftPending) return;
    const timer = setInterval(() => {
      void load();
    }, 3000);
    return () => clearInterval(timer);
  });

  const params = $derived($page.url.searchParams);
  const tab = $derived(params.get('tab') === 'decisions' ? 'decisions' : 'requirements');
  const requirements = $derived(intake?.requirements ?? []);
  const decisions = $derived(intake?.decisions ?? []);
  const filter = $derived({
    q: params.get('q') ?? '',
    status: parseStatus(params.get('status'), requirements),
    source: parseSource(params.get('source'))
  });
  const counts = $derived(intakeCounts(requirements));
  const shown = $derived(filterRequirements(requirements, filter));
  const shownDecisions = $derived(filterDecisions(decisions, filter.q));
  const acceptable = $derived(intake?.canAccept ? acceptableIds(shown) : []);
  const filtered = $derived(filter.q !== '' || filter.source !== '' || params.has('status'));
  const allSelected = $derived(acceptable.length > 0 && acceptable.every((id) => selected.includes(id)));
  const someSelected = $derived(!allSelected && acceptable.some((id) => selected.includes(id)));
  const columns = $derived(acceptable.length > 0 ? 8 : 7);
  const narrowHidden = 'max-[720px]:hidden';

  const filterKey = $derived(`${tab}|${filter.q}|${filter.status}|${filter.source}`);
  const paging = pager(() => filterKey);

  $effect(() => {
    void filterKey;
    untrack(() => {
      selected = selected.filter((id) => acceptable.includes(id));
    });
  });

  function tabHref(key: 'requirements' | 'decisions') {
    const url = new URL($page.url);
    if (key === 'decisions') url.searchParams.set('tab', 'decisions');
    else url.searchParams.delete('tab');
    return `${url.pathname}${url.search}`;
  }

  function clearFilters() {
    setParams({ q: '', source: '', status: 'all' });
  }

  function toggle(id: string, checked: boolean) {
    selected = checked ? [...selected, id] : selected.filter((item) => item !== id);
  }

  function toggleAll(checked: boolean) {
    selected = checked ? [...acceptable] : [];
  }

  async function readDocs() {
    pending = 'ingest';
    try {
      intake = await ingest();
      skipped = intake.skipped ?? [];
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      pending = '';
    }
  }

  async function accept(ids: string[], from: 'all' | 'selected') {
    if (ids.length === 0) return;
    pending = from;
    try {
      intake = await acceptRequirements(ids);
      selected = [];
      toast.success('Accepted. Cards appear on the board shortly.');
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      pending = '';
    }
  }

  showShell(() => ({ title: 'Intake' }));
</script>

<Page
  title="Intake"
  description="Requirements read from the PRD and RFCs, or added by hand, and the decisions from the RFCs. Cards are built only from accepted requirements."
  {loading}
  error={loadError}
  onretry={() => void load(true)}
>
  {#snippet summary()}
    {#if intake?.draftPending}
      <p class="flex items-center gap-2 text-sm text-muted" role="status">
        <Spinner />
        Planner is drafting requirement ids. This page refreshes on its own.
      </p>
    {/if}
  {/snippet}
  {#snippet meta()}
    {#if requirements.length > 0}
      <Badge tone={counts.accepted === counts.all ? 'green' : 'default'}>
        {counts.accepted} of {counts.all} accepted
      </Badge>
    {/if}
  {/snippet}
  {#snippet actions()}
    <IntakeActions {busy} ingesting={pending === 'ingest'} selecting={selected.length > 0} oningest={() => void readDocs()} />
  {/snippet}

  <div class="flex flex-col gap-4">
    {#if intake?.planError}<Notice tone="danger">{intake.planError}</Notice>{/if}
    {#if skipped.length > 0}
      <Notice>
        {skipped.join(', ')}
        {skipped.length === 1 ? 'is' : 'are'} also in the repository. The text added by hand was kept.
      </Notice>
    {/if}
    {#if intake && !intake.canAccept && counts.open > 0}
      <Notice>A Lead Developer, Senior Developer, QA Lead, or Product Owner accepts requirements.</Notice>
    {/if}

    {#if intake && requirements.length === 0 && decisions.length === 0}
      <EmptyState
        title="No requirements yet"
        description="Ingest reads the PRD and RFCs for this project, or add a requirement by hand."
        href="/intake/new"
        link="Add requirement"
      />
    {:else if intake}
      <nav class="flex gap-1 border-b border-line" aria-label="Intake">
        {#each [{ key: 'requirements', label: 'Requirements', count: counts.all }, { key: 'decisions', label: 'Decisions', count: decisions.length }] as item (item.key)}
          <a
            href={tabHref(item.key as 'requirements' | 'decisions')}
            aria-current={tab === item.key ? 'page' : undefined}
            class="-mb-px flex min-h-11 items-center border-b-2 px-3 text-sm {tab === item.key
              ? 'border-accent font-medium text-ink'
              : 'border-transparent text-muted hover:text-ink'}"
          >
            {item.label}
            <span class="ml-2 text-xs text-muted">{item.count}</span>
          </a>
        {/each}
      </nav>

      <Toolbar>
        <SearchBox
          label={`Search ${tab}`}
          value={filter.q}
          placeholder={tab === 'decisions' ? 'Search id, text, or file' : 'Search id, text, acceptance, or file'}
        />
        {#if tab === 'requirements'}
          <Segmented
            label="Status"
            options={statusOptions.map((option) => ({ ...option, count: counts[option.value] }))}
            value={filter.status}
            onchange={(status) => setParams({ status })}
          />
          <FilterSelect label="Source" options={sourceOptions} value={filter.source} onchange={(source) => setParams({ source })} />
          {#if acceptable.length > 0}
            <div class="ml-auto">
              <Button
                type="button"
                variant="secondary"
                loading={pending === 'all'}
                disabled={busy || Boolean(intake.draftPending)}
                onclick={() => void accept(acceptable, 'all')}
              >
                Accept all shown ({acceptable.length})
              </Button>
            </div>
          {/if}
        {/if}
      </Toolbar>

      {#if tab === 'requirements'}
        {#if shown.length === 0}
          <EmptyState title="No requirements match" description="Change the search or filters to see more requirements.">
            {#if filtered}<Button type="button" variant="secondary" onclick={clearFilters}>Clear filters</Button>{/if}
          </EmptyState>
        {:else}
          <Table label="Requirements">
            <thead>
              <tr class="border-b border-line bg-ground/50">
                {#if acceptable.length > 0}
                  <th class="w-11 px-2" scope="col">
                    <label class="flex size-11 items-center justify-center">
                      <input
                        type="checkbox"
                        class="size-4 accent-accent"
                        aria-label="Select all shown"
                        checked={allSelected}
                        indeterminate={someSelected}
                        onchange={(event) => toggleAll(event.currentTarget.checked)}
                      />
                    </label>
                  </th>
                {/if}
                <th class={th} scope="col">ID</th>
                <th class={th} scope="col">Requirement</th>
                <th class="{th} {narrowHidden}" scope="col">Source</th>
                <th class="{th} max-[1280px]:hidden" scope="col">Reviewers</th>
                <th class="{th} {narrowHidden}" scope="col">Cards</th>
                <th class={th} scope="col">Status</th>
                <th class={th} scope="col"><span class="sr-only">Details</span></th>
              </tr>
            </thead>
            <tbody>
              {#each shown.slice(0, paging.limit) as requirement (requirement.id)}
                <RequirementRow
                  {requirement}
                  {columns}
                  selectable={acceptable.length > 0}
                  selected={selected.includes(requirement.id)}
                  onselect={(checked) => toggle(requirement.id, checked)}
                />
              {/each}
            </tbody>
          </Table>
          <ShowMore total={shown.length} limit={paging.limit} noun={['requirement', 'requirements']} onmore={paging.more} />
        {/if}
      {:else if shownDecisions.length === 0}
        <EmptyState
          title={decisions.length === 0 ? 'No decisions yet' : 'No decisions match'}
          description={decisions.length === 0 ? 'Ingest reads decisions from the RFCs.' : 'Change the search to see more decisions.'}
        >
          {#if filter.q !== ''}<Button type="button" variant="secondary" onclick={clearFilters}>Clear search</Button>{/if}
        </EmptyState>
      {:else}
        <Table label="Decisions">
          <thead>
            <tr class="border-b border-line bg-ground/50">
              <th class={th} scope="col">ID</th>
              <th class={th} scope="col">Decision</th>
              <th class="{th} {narrowHidden}" scope="col">Source</th>
            </tr>
          </thead>
          <tbody>
            {#each shownDecisions.slice(0, paging.limit) as decision (decision.id)}
              <tr class="{tr} align-top">
                <td class="{td} w-24 py-3.5 font-mono text-xs whitespace-nowrap text-ink">{decision.id}</td>
                <td class="{td} min-w-56 py-3 text-sm text-ink">{decision.statement}</td>
                <td class="{td} py-3.5 font-mono text-xs break-all text-muted {narrowHidden}">{decision.sourcePath}</td>
              </tr>
            {/each}
          </tbody>
        </Table>
        <ShowMore total={shownDecisions.length} limit={paging.limit} noun={['decision', 'decisions']} onmore={paging.more} />
      {/if}
    {/if}
  </div>

  {#if selected.length > 0}
    <div class="sticky bottom-0 z-10 mt-6 flex items-center justify-between gap-4 border-t border-line bg-ground py-4">
      <p class="text-sm text-ink">
        {selected.length}
        {selected.length === 1 ? 'requirement' : 'requirements'} selected
      </p>
      <div class="flex gap-2">
        <Button type="button" variant="ghost" disabled={busy} onclick={() => (selected = [])}>Clear</Button>
        <Button
          type="button"
          variant="primary"
          loading={pending === 'selected'}
          disabled={busy || Boolean(intake?.draftPending)}
          onclick={() => void accept(selected, 'selected')}
        >
          Accept selected
        </Button>
      </div>
    </div>
  {/if}
</Page>
