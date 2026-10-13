<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getProjects, getTribes, unreachable } from '$lib/api';
  import { matchText } from '$lib/filter';
  import { setParams } from '$lib/params';
  import { session } from '$lib/session';
  import { managesAnyTribe, shell, showShell } from '$lib/shell.svelte';
  import type { ProjectRecord, TribeRecord } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import Table from '$lib/ui/Table.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  let projects = $state<ProjectRecord[]>([]);
  let tribes = $state<TribeRecord[]>([]);
  let loaded = $state(false);
  let message = $state('Loading projects');

  const tribeId = session.tribe();

  showShell(() => ({ title: 'Projects', message }));

  const params = $derived($page.url.searchParams);
  const q = $derived(params.get('q') ?? '');
  const tribeName = $derived(tribes.find((tribe) => tribe.id === tribeId)?.name ?? 'this tribe');
  const own = $derived(projects.filter((project) => project.tribeId === tribeId));
  const shown = $derived(own.filter((project) => matchText([project.name, project.repo, project.baseBranch], q)));

  function open(id: string, path: string) {
    session.setProject(id);
    window.location.assign(path);
  }

  $effect(() => {
    void Promise.all([getProjects(), getTribes()])
      .then(([projectBody, tribeBody]) => {
        projects = projectBody.projects;
        tribes = tribeBody.tribes;
        loaded = true;
        message = 'Projects';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });
</script>

<Page
  title="Projects"
  description={tribeId === ''
    ? 'Each project is one repository with its own board and settings.'
    : `Projects in ${tribeName}. Switch tribe in the sidebar to see another tribe's projects.`}
>
  {#snippet actions()}
    {#if managesAnyTribe()}<Button variant="primary" href="/projects/new">New project</Button>{/if}
  {/snippet}
  {#if tribeId === ''}
    <EmptyState title="Choose a tribe to see its projects." href="/choose-tribe" link="Choose a tribe" />
  {:else if loaded && own.length === 0}
    <EmptyState title="No projects in this tribe yet." description="An admin or a tribe admin adds projects." />
  {:else if own.length > 0}
    <div class="flex flex-col gap-4">
      <Toolbar>
        <SearchBox label="Search projects" value={q} placeholder="Search name or repository" />
      </Toolbar>
      {#if shown.length === 0}
        <EmptyState title="No projects match" description="Change the search to see more projects.">
          <Button type="button" variant="secondary" onclick={() => setParams({ q: '' })}>Clear filters</Button>
        </EmptyState>
      {:else}
        <Table label="Projects">
          <thead>
            <tr class="border-b border-line bg-ground/50">
              <th class={th} scope="col">Name</th>
              <th class={th} scope="col">Repository</th>
              <th class={th} scope="col"><span class="sr-only">Open</span></th>
            </tr>
          </thead>
          <tbody>
            {#each shown as project (project.id)}
              <tr class={tr}>
                <td class="{td} font-medium text-ink">
                  <span class="flex items-center gap-2">
                    {project.name}
                    {#if project.id === shell.projectId}<Badge tone="green">Current</Badge>{/if}
                  </span>
                </td>
                <td class="{td} font-mono text-xs text-muted">
                  {#if project.repo}{project.repo}<span class="text-muted/70"> · {project.baseBranch}</span>{:else}Not set{/if}
                </td>
                <td class="{td} text-right">
                  <span class="inline-flex items-center gap-1">
                    <button
                      type="button"
                      class="inline-flex min-h-11 items-center px-2 text-sm font-medium text-accent hover:underline"
                      aria-label="Settings for {project.name}"
                      onclick={() => open(project.id, '/settings/repository')}
                    >
                      Settings
                    </button>
                    <button
                      type="button"
                      class="inline-flex min-h-11 items-center px-2 text-sm font-medium text-accent hover:underline"
                      onclick={() => open(project.id, '/')}
                    >
                      Open board
                    </button>
                  </span>
                </td>
              </tr>
            {/each}
          </tbody>
        </Table>
      {/if}
    </div>
  {/if}
</Page>
