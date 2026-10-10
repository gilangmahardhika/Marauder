<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getConventions, unreachable } from '$lib/api';
  import { categoryLabel, markersSummary, ruleGroups } from '$lib/conventions';
  import { showShell } from '$lib/shell.svelte';
  import type { ConventionCatalog } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Table from '$lib/ui/Table.svelte';

  let catalog = $state<ConventionCatalog>({ rules: [], stacks: [], skills: [] });
  let loaded = $state(false);
  let message = $state('Loading conventions');

  showShell(() => ({ title: 'Conventions', message }));

  const tab = $derived($page.url.searchParams.get('tab') === 'stacks' ? 'stacks' : 'rules');
  const groups = $derived(ruleGroups(catalog.rules, catalog.stacks));
  const tabs = [
    { key: 'rules', label: 'Rules', href: '/admin/conventions' },
    { key: 'stacks', label: 'Stacks', href: '/admin/conventions?tab=stacks' }
  ];

  $effect(() => {
    void getConventions()
      .then((body) => {
        catalog = body;
        loaded = true;
        message = 'Conventions';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  const link = 'inline-flex min-h-11 items-center px-2 text-sm font-medium text-accent hover:underline';
</script>

<Page
  title="Conventions"
  description="Company coding rules. Global rules reach every project. Stack rules reach projects whose repository uses that stack, detected on Sync or added by a Lead Developer."
>
  {#snippet actions()}
    {#if tab === 'stacks'}
      <Button variant="primary" href="/admin/conventions/new?kind=stack">Add stack</Button>
    {:else}
      <Button variant="primary" href="/admin/conventions/new">Add rule</Button>
    {/if}
  {/snippet}

  <nav class="mb-6 flex gap-1 border-b border-line" aria-label="Conventions">
    {#each tabs as item (item.key)}
      <a
        href={item.href}
        aria-current={tab === item.key ? 'page' : undefined}
        class="-mb-px flex min-h-11 items-center border-b-2 px-3 text-sm {tab === item.key
          ? 'border-accent font-medium text-ink'
          : 'border-transparent text-muted hover:text-ink'}"
      >
        {item.label}
        <span class="ml-2 text-xs text-muted">{item.key === 'rules' ? catalog.rules.length : catalog.stacks.length}</span>
      </a>
    {/each}
  </nav>

  {#if tab === 'rules'}
    {#if loaded && catalog.rules.length === 0}
      <EmptyState title="No rules yet" description="Add a global rule for naming or database columns, or a rule for one stack.">
        <Button variant="primary" href="/admin/conventions/new">Add rule</Button>
      </EmptyState>
    {:else}
      <div class="flex flex-col gap-8">
        {#each groups as group (group.key)}
          {#if group.rules.length > 0}
            <section>
              <div class="mb-3 flex items-baseline justify-between gap-4">
                <h2 class="text-[15px] font-semibold text-ink">{group.label}</h2>
                <a
                  class="text-sm text-accent hover:underline"
                  href={group.stackId ? `/admin/conventions/new?stack=${encodeURIComponent(group.stackId)}` : '/admin/conventions/new'}
                  >Add {group.stackId ? `${group.label} rule` : 'global rule'}</a
                >
              </div>
              <Table label={`${group.label} rules`}>
                <thead>
                  <tr class="border-b border-line bg-ground/50">
                    <th class={th} scope="col">Rule</th>
                    <th class={th} scope="col">Category</th>
                    <th class={th} scope="col">Version</th>
                    <th class={th} scope="col"><span class="sr-only">Edit</span></th>
                  </tr>
                </thead>
                <tbody>
                  {#each group.rules as rule (rule.id)}
                    <tr class={tr}>
                      <td class="{td} py-2.5">
                        <span class="block font-medium text-ink">{rule.title}</span>
                        <span class="mt-0.5 line-clamp-1 text-xs text-muted">{rule.body}</span>
                      </td>
                      <td class={td}><Badge>{categoryLabel(rule.category)}</Badge></td>
                      <td class="{td} text-sm text-muted">
                        v{rule.version}
                        {#if rule.disabled}<span class="ml-2"><Badge tone="amber">Disabled</Badge></span>{/if}
                      </td>
                      <td class="{td} text-right">
                        <a class={link} href={`/admin/conventions/${encodeURIComponent(rule.id)}`} aria-label={`Edit ${rule.title}`}
                          >Edit</a
                        >
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </Table>
            </section>
          {/if}
        {/each}
      </div>
    {/if}
  {:else if loaded && catalog.stacks.length === 0}
    <EmptyState title="No stacks yet" description="A stack is a language or framework, with the files and dependencies that identify it.">
      <Button variant="primary" href="/admin/conventions/new?kind=stack">Add stack</Button>
    </EmptyState>
  {:else}
    <Table label="Stacks">
      <thead>
        <tr class="border-b border-line bg-ground/50">
          <th class={th} scope="col">Stack</th>
          <th class={th} scope="col">Detected by</th>
          <th class={th} scope="col">Rules</th>
          <th class={th} scope="col"><span class="sr-only">Edit</span></th>
        </tr>
      </thead>
      <tbody>
        {#each catalog.stacks as stack (stack.id)}
          <tr class={tr}>
            <td class="{td} py-2.5">
              <span class="block font-medium text-ink">{stack.name}</span>
              {#if stack.disabled}<span class="mt-1 block"><Badge tone="amber">Disabled</Badge></span>{/if}
            </td>
            <td class="{td} font-mono text-xs text-muted">{markersSummary(stack)}</td>
            <td class="{td} text-sm text-muted">
              {catalog.rules.filter((rule) => rule.stackId === stack.id).length}
            </td>
            <td class="{td} text-right">
              <a class={link} href={`/admin/conventions/${encodeURIComponent(stack.id)}`} aria-label={`Edit ${stack.name}`}>Edit</a>
            </td>
          </tr>
        {/each}
      </tbody>
    </Table>
  {/if}
</Page>
