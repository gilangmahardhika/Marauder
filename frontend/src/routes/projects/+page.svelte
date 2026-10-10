<script lang="ts">
  import { ApiError, getProjects, getTribes, unreachable } from '$lib/api';
  import { session } from '$lib/session';
  import { shell, showShell } from '$lib/shell.svelte';
  import type { ProjectRecord, TribeRecord } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Table from '$lib/ui/Table.svelte';

  let projects = $state<ProjectRecord[]>([]);
  let tribes = $state<TribeRecord[]>([]);
  let loaded = $state(false);
  let message = $state('Loading projects');

  showShell(() => ({ title: 'Projects', message }));

  function tribeName(id: string) {
    return tribes.find((tribe) => tribe.id === id)?.name ?? id;
  }

  function open(id: string) {
    session.setProject(id);
    window.location.assign('/');
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

<Page title="Projects" description="Each project is one repository with its own board and settings.">
  {#snippet actions()}
    {#if shell.admin}<Button variant="primary" href="/projects/new">New project</Button>{/if}
  {/snippet}
  {#if loaded && projects.length === 0}
    <EmptyState title="No projects yet" description="Projects you can see belong to your tribes." />
  {:else if projects.length > 0}
    <Table label="Projects">
      <thead>
        <tr class="border-b border-line bg-ground/50">
          <th class={th} scope="col">Name</th>
          <th class={th} scope="col">Tribe</th>
          <th class={th} scope="col">Repository</th>
          <th class={th} scope="col"><span class="sr-only">Open</span></th>
        </tr>
      </thead>
      <tbody>
        {#each projects as project (project.id)}
          <tr class={tr}>
            <td class="{td} font-medium text-ink">
              <span class="flex items-center gap-2">
                {project.name}
                {#if project.id === shell.projectId}<Badge tone="green">Current</Badge>{/if}
              </span>
            </td>
            <td class="{td} text-muted">{tribeName(project.tribeId)}</td>
            <td class="{td} font-mono text-xs text-muted">
              {#if project.repo}{project.repo}<span class="text-muted/70"> · {project.baseBranch}</span>{:else}Not set{/if}
            </td>
            <td class="{td} text-right">
              <button
                type="button"
                class="inline-flex min-h-11 items-center px-2 text-sm font-medium text-accent hover:underline"
                onclick={() => open(project.id)}
              >
                Open board
              </button>
            </td>
          </tr>
        {/each}
      </tbody>
    </Table>
  {/if}
</Page>
