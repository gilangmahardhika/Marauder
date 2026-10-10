<script lang="ts">
  import { goto } from '$app/navigation';
  import { ApiError, createProject, getTribes, unreachable } from '$lib/api';
  import { session } from '$lib/session';
  import { showShell } from '$lib/shell.svelte';
  import type { TribeRecord } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Select from '$lib/ui/Select.svelte';

  let tribes = $state<TribeRecord[]>([]);
  let tribeId = $state('');
  let name = $state('');
  let repoPath = $state('');
  let message = $state('New project');
  let busy = $state(false);

  showShell(() => ({ title: 'New project', message }));

  $effect(() => {
    void getTribes()
      .then((body) => {
        tribes = body.tribes;
        if (tribeId === '' && tribes[0]) tribeId = tribes[0].id;
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    message = 'Creating project';
    try {
      const project = await createProject({ tribeId, name, repoPath });
      session.setProject(project.id);
      message = 'Project created';
      await goto('/projects');
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page title="New project" description="A project is one repository. Members of its tribe can see its board." width="form">
  <form onsubmit={submit}>
    <Panel title="Project">
      <Select label="Tribe" id="tribe" bind:value={tribeId}>
        {#each tribes as tribe (tribe.id)}
          <option value={tribe.id}>{tribe.name}</option>
        {/each}
      </Select>
      <Field label="Name" id="name" bind:value={name} />
      <Field
        label="Repo path"
        id="repo"
        width="lg"
        mono
        hint="Absolute directory outside the data root."
        placeholder="/srv/repos/app"
        bind:value={repoPath}
      />
      {#snippet footer()}
        <Button variant="ghost" href="/projects">Cancel</Button>
        <Button variant="primary" type="submit" disabled={busy}>Create project</Button>
      {/snippet}
    </Panel>
  </form>
</Page>
