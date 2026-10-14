<script lang="ts">
  import { page } from '$app/stores';
  import { getRunners, putJobs, runnerAction } from '$lib/api';
  import { pickParam } from '$lib/filter';
  import { failureText } from '$lib/forms';
  import { setParams } from '$lib/params';
  import {
    actionLabel,
    agentLabel,
    filterRunners,
    since,
    statusLabel,
    statusOptions,
    statusTone,
    statusValues,
    waitingForClaude,
    waitingForRunners
  } from '$lib/runners';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { AgentKind, Executor, Runner } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import FilterSelect from '$lib/ui/FilterSelect.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';
  import Table from '$lib/ui/Table.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  let runners = $state<Runner[]>([]);
  let executor = $state<Executor>('local');
  let agent = $state<AgentKind>('builtin');
  let loaded = $state(false);
  let busy = $state(false);
  let loading = $state('Loading runners');
  let loadError = $state('');
  let confirming = $state('');
  let now = $state(Date.now());

  showShell(() => ({ title: 'Runners' }));

  const params = $derived($page.url.searchParams);
  const filter = $derived({
    q: params.get('q') ?? '',
    status: pickParam(params.get('status'), [...statusValues], '')
  });
  const shown = $derived(filterRunners(runners, filter));
  const waiting = $derived(waitingForRunners(executor, runners));
  const noClaude = $derived(!waiting && waitingForClaude(executor, agent, runners));
  const removing = $derived(runners.find((runner) => runner.id === confirming));

  const executorOptions = [
    { value: 'local', label: 'This server' },
    { value: 'runners', label: 'Runners' }
  ];

  async function load() {
    try {
      const body = await getRunners();
      runners = body.runners;
      executor = body.executor;
      agent = body.agent ?? 'builtin';
      loaded = true;
      loadError = '';
      now = Date.now();
    } catch (error) {
      if (loaded) toast.error(failureText(error));
      else loadError = failureText(error);
    } finally {
      loading = '';
    }
  }

  function retry() {
    loading = 'Loading runners';
    loadError = '';
    void load();
  }

  $effect(() => {
    void load();
    const timer = setInterval(() => void load(), 5000);
    return () => clearInterval(timer);
  });

  async function saveExecutor(next: string) {
    if (next === executor || busy) return;
    busy = true;
    try {
      const body = await putJobs(next as Executor);
      executor = body.executor;
      toast.success(body.executor === 'runners' ? 'Implement jobs now go to runners.' : 'Implement jobs now run on this server.');
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      busy = false;
    }
  }

  async function act(runner: Runner, action: string) {
    if (action === 'remove' && confirming !== runner.id) {
      confirming = runner.id;
      return;
    }
    busy = true;
    try {
      await runnerAction(runner.id, action);
      confirming = '';
      toast.success(action === 'remove' ? `${runner.name} removed` : `${runner.name}: ${actionLabel(action).toLowerCase()} done`);
      await load();
    } catch (error) {
      toast.error(failureText(error));
      await load();
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title="Runners"
  description="Machines that run implement jobs. A runner calls this server over HTTPS and never holds the database. It gets a model key only for a Claude Code job."
  {loading}
  error={loadError}
  onretry={retry}
>
  {#snippet actions()}
    <Button variant="ghost" href="/admin/runners/guide">Runner guide</Button>
    <Button variant="primary" href="/admin/runners/new" icon="plus">Add runner</Button>
  {/snippet}
  <div class="flex flex-col gap-6">
    <Panel title="Implement jobs run on" hint="Review, deploy, and verify always run on this server.">
      <div class="flex flex-col gap-3">
        <Segmented label="Implement jobs run on" options={executorOptions} value={executor} disabled={busy || !loaded} onchange={(value) => void saveExecutor(value)} />
        {#if waiting}
          <Notice tone="warning">No runner is online. Queued cards wait until a runner connects.</Notice>
        {:else if noClaude}
          <Notice tone="warning">No runner has Claude Code. Queued cards wait.</Notice>
        {/if}
      </div>
    </Panel>

    {#if removing}
      <Notice tone="danger">
        <div class="flex flex-wrap items-center gap-2">
          <span>Remove {removing.name}? Its open jobs go back to the queue, and its secret stops working.</span>
          <Button type="button" variant="ghost" disabled={busy} onclick={() => (confirming = '')}>Cancel</Button>
          <Button type="button" variant="danger" icon="trash" loading={busy} onclick={() => void act(removing, 'remove')}>Remove</Button>
        </div>
      </Notice>
    {/if}

    {#if loaded && runners.length === 0}
      <EmptyState title="No runners yet" description="Add a runner to get an install command for a Linux machine with Podman." />
    {:else if runners.length > 0}
      <Toolbar>
        <SearchBox label="Search runners" value={filter.q} placeholder="Search name or label" />
        <FilterSelect label="Status" options={statusOptions(runners)} value={filter.status} onchange={(status) => setParams({ status })} />
      </Toolbar>
      {#if shown.length === 0}
        <EmptyState title="No runners match" description="Change the search or filters to see more runners.">
          <Button type="button" variant="secondary" icon="close" onclick={() => setParams({ q: '', status: '' })}>Clear filters</Button>
        </EmptyState>
      {:else}
        <Table label="Runners">
          <thead>
            <tr class="border-b border-line bg-ground/50">
              <th class={th} scope="col">Runner</th>
              <th class={th} scope="col">Status</th>
              <th class={th} scope="col">Slots</th>
              <th class={th} scope="col">Version</th>
              <th class={th} scope="col">Agents</th>
              <th class={th} scope="col">Heartbeat</th>
              <th class={th} scope="col">Cards</th>
              <th class={th} scope="col"><span class="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {#each shown as runner (runner.id)}
              <tr class={tr}>
                <td class={td}>
                  <span class="block font-medium text-ink">{runner.name}</span>
                  {#if runner.labels.length > 0}
                    <span class="mt-1 flex flex-wrap gap-1">
                      {#each runner.labels as label (label)}<Badge>{label}</Badge>{/each}
                    </span>
                  {/if}
                </td>
                <td class={td}><Badge tone={statusTone(runner.status)}>{statusLabel(runner.status)}</Badge></td>
                <td class="{td} tabular-nums text-muted">{runner.busy}/{runner.slots}</td>
                <td class="{td} text-muted">
                  {runner.version || 'unknown'}
                  {#if runner.updateRequired}<span class="ml-1"><Badge tone="amber">Update required</Badge></span>{/if}
                </td>
                <td class="{td} text-muted">{(runner.agents ?? []).map(agentLabel).join(', ') || 'Built-in'}</td>
                <td class="{td} text-muted" title={runner.heartbeatAt ?? ''}>{since(runner.heartbeatAt, now)}</td>
                <td class={td}>
                  {#if runner.leases.length === 0}
                    <span class="text-muted">None</span>
                  {:else}
                    <span class="flex flex-col gap-0.5">
                      {#each runner.leases as lease (lease.id)}
                        <a class="font-mono text-xs text-accent hover:underline" href={`/cards/${lease.cardId}`}>{lease.cardId}</a>
                      {/each}
                    </span>
                  {/if}
                </td>
                <td class="{td} text-right">
                  <span class="inline-flex flex-wrap justify-end gap-1">
                    {#each runner.actions as action (action)}
                      <button
                        type="button"
                        class="inline-flex min-h-11 items-center rounded-control px-2 text-sm font-medium hover:underline disabled:opacity-50 {action === 'remove'
                          ? 'text-red-800'
                          : 'text-accent'}"
                        aria-label={`${actionLabel(action)} ${runner.name}`}
                        disabled={busy}
                        onclick={() => void act(runner, action)}>{actionLabel(action)}</button
                      >
                    {/each}
                  </span>
                </td>
              </tr>
            {/each}
          </tbody>
        </Table>
      {/if}
    {/if}
  </div>
</Page>
