<script lang="ts">
  import { goto } from '$app/navigation';
  import { ApiError, closeSession, getMe, getProjects, getTribes, unreachable } from '$lib/api';
  import Live from '$lib/components/Live.svelte';
  import { session } from '$lib/session';
  import { showShell } from '$lib/shell.svelte';
  import type { ProjectRecord, TribeRecord } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Icon from '$lib/ui/Icon.svelte';

  let tribes = $state<TribeRecord[]>([]);
  let projects = $state<ProjectRecord[]>([]);
  let admin = $state(false);
  let loaded = $state(false);
  let message = $state('Loading tribes');

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

  $effect(() => {
    void Promise.all([getTribes(), getProjects()])
      .then(([tribeBody, projectBody]) => {
        tribes = tribeBody.tribes;
        projects = projectBody.projects;
        loaded = true;
        message = '';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
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
    <h1 class="text-xl font-semibold">Choose a tribe</h1>
    <p class="mt-1 text-sm text-muted">The board and the project switcher show this tribe's projects.</p>

    {#if loaded && tribes.length === 0}
      <div class="mt-6 rounded-panel border border-dashed border-line-strong bg-surface px-5 py-6">
        <p class="text-sm text-ink">You are not in a tribe yet. Ask an admin to add you.</p>
        {#if admin}
          <a class="mt-3 inline-flex min-h-11 items-center text-sm font-medium text-accent underline" href="/tribes/new">Create a tribe</a>
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

    <div class="mt-4">
      <Live {message} />
    </div>
    <div class="mt-6 border-t border-line pt-4">
      <Button variant="ghost" onclick={() => void signOut()}>Sign out</Button>
    </div>
  </div>
</div>
