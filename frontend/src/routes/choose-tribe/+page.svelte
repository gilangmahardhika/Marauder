<script lang="ts">
  import { goto } from '$app/navigation';
  import { closeSession, getMe, getProjects, getTribes } from '$lib/api';
  import TribeIllustration from '$lib/components/TribeIllustration.svelte';
  import { failureText } from '$lib/forms';
  import { session } from '$lib/session';
  import { showShell } from '$lib/shell.svelte';
  import type { ProjectRecord, TribeRecord } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Spinner from '$lib/ui/Spinner.svelte';

  let tribes = $state<TribeRecord[]>([]);
  let projects = $state<ProjectRecord[]>([]);
  let admin = $state(false);
  let loaded = $state(false);
  let loading = $state(true);
  let loadError = $state('');

  const current = session.tribe();

  showShell(() => ({ title: 'Choose a tribe' }));

  function projectsOf(tribeId: string) {
    return projects.filter((project) => project.tribeId === tribeId);
  }

  function pick(tribe: TribeRecord) {
    session.setTribe(tribe.id);
    session.setProject(projectsOf(tribe.id)[0]?.id ?? '');
    void goto('/');
  }

  async function signOut() {
    try {
      await closeSession();
    } catch {
      /* The local session still ends. */
    }
    session.clear();
    window.location.assign('/login');
  }

  function load() {
    loading = true;
    loadError = '';
    void Promise.all([getTribes(), getProjects()])
      .then(([tribeBody, projectBody]) => {
        tribes = tribeBody.tribes;
        projects = projectBody.projects;
        loaded = true;
      })
      .catch((error: unknown) => {
        loadError = failureText(error);
      })
      .finally(() => {
        loading = false;
      });
  }

  $effect(() => {
    load();
    void getMe()
      .then((me) => (admin = me.admin))
      .catch(() => (admin = false));
  });
</script>

<div class="flex min-h-screen items-start justify-center bg-ground px-6 py-12 sm:items-center">
  <div class="w-full max-w-md">
    <div class="mb-8 flex items-center gap-2">
      <span class="flex size-7 items-center justify-center rounded-control bg-accent text-xs font-semibold text-surface">M</span>
      <span class="font-semibold">Marauder</span>
    </div>
    <TribeIllustration class="mb-6 h-auto max-h-52 w-full" />
    <h1 class="text-xl font-semibold">Choose a tribe</h1>
    <p class="mt-1 text-sm text-muted">The board and the project switcher show this tribe's projects.</p>

    {#if loadError}
      <div class="mt-6 flex items-start gap-3 rounded-panel border border-red-200 bg-red-50 p-4" role="alert">
        <Icon name="alert" class="mt-0.5 size-4 shrink-0 text-danger" />
        <div class="min-w-0 flex-1">
          <p class="wrap-break-word text-sm text-red-900">{loadError}</p>
          <div class="mt-3">
            <Button icon="refresh" onclick={load}>Try again</Button>
          </div>
        </div>
      </div>
    {:else if loading}
      <p class="mt-6 flex items-center gap-2 text-sm text-muted" role="status">
        <Spinner />
        Loading tribes
      </p>
    {:else if loaded && tribes.length === 0}
      <div class="mt-6 rounded-panel border border-dashed border-line-strong bg-surface px-5 py-6">
        <p class="text-sm text-ink">You are not in a tribe yet. Ask an admin to add you.</p>
        {#if admin}
          <div class="mt-3">
            <Button href="/tribes/new" icon="plus">Create a tribe</Button>
          </div>
        {/if}
      </div>
    {:else if tribes.length > 0}
      <ul class="mt-6 flex flex-col gap-2" aria-label="Tribes">
        {#each tribes as tribe (tribe.id)}
          {@const count = projectsOf(tribe.id).length}
          <li>
            <button
              type="button"
              class="flex min-h-14 w-full items-center gap-3 rounded-panel border border-line bg-surface px-4 py-3 text-left hover:border-line-strong hover:bg-ground/60"
              aria-current={tribe.id === current ? 'true' : undefined}
              onclick={() => pick(tribe)}
            >
              <span class="min-w-0 flex-1">
                <span class="block truncate text-sm font-medium text-ink">{tribe.name}</span>
                <span class="block text-xs text-muted">{count} {count === 1 ? 'project' : 'projects'}</span>
              </span>
              {#if tribe.id === current}<Badge tone="green">Current</Badge>{/if}
              <Icon name="chevron" class="size-4 shrink-0 -rotate-90 text-muted" />
            </button>
          </li>
        {/each}
      </ul>
    {/if}

    <div class="mt-6 border-t border-line pt-4">
      <Button variant="ghost" icon="log-out" onclick={() => void signOut()}>Sign out</Button>
    </div>
  </div>
</div>
