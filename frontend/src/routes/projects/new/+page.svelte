<script lang="ts">
  import { goto } from '$app/navigation';
  import { ApiError, createProject, getCompanyRepos, getProjects, getTribes, unreachable } from '$lib/api';
  import { session } from '$lib/session';
  import { canManageTribe, shell, showShell } from '$lib/shell.svelte';
  import type { CompanyRepo, ProjectRecord, TribeRecord } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import MultiSearchSelect from '$lib/ui/MultiSearchSelect.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SearchSelect from '$lib/ui/SearchSelect.svelte';
  import Select from '$lib/ui/Select.svelte';

  let tribes = $state<TribeRecord[]>([]);
  let projects = $state<ProjectRecord[]>([]);
  let links = $state<string[]>([]);
  let repos = $state<CompanyRepo[] | null>(null);
  let reposError = $state('');
  let tribeId = $state('');
  let name = $state('');
  let repo = $state('');
  let baseBranch = $state('main');
  let fields = $state<Record<string, string>>({});
  let message = $state('New project');
  let busy = $state(false);

  showShell(() => ({ title: 'New project', message }));

  const manageable = $derived(tribes.filter((tribe) => canManageTribe(tribe.id)));
  const linkOptions = $derived(
    projects
      .filter((project) => project.tribeId === tribeId)
      .map((project) => ({ value: project.id, label: project.name, detail: project.repo }))
  );
  const tribeName = $derived(tribes.find((tribe) => tribe.id === tribeId)?.name ?? 'this tribe');

  $effect(() => {
    if (manageable.length > 0 && !manageable.some((tribe) => tribe.id === tribeId)) tribeId = manageable[0].id;
  });

  $effect(() => {
    const allowed = new Set(linkOptions.map((option) => option.value));
    if (links.some((id) => !allowed.has(id))) links = links.filter((id) => allowed.has(id));
  });

  const repoOptions = $derived(
    (repos ?? []).map((item) => ({
      value: item.name,
      label: item.name,
      detail: item.archived ? `${item.defaultBranch} · archived` : item.defaultBranch,
      disabled: item.archived
    }))
  );
  const repoError = $derived(
    fields.repo ||
      (reposError
        ? `The repository list did not load: ${reposError}`
        : repos?.length === 0
          ? 'The company token sees no repositories in the organization.'
          : '')
  );
  const repoPlaceholder = $derived(
    repos === null
      ? reposError
        ? 'Repositories did not load'
        : 'Loading repositories'
      : repos.length === 0
        ? 'No repositories in the organization'
        : 'Search repositories'
  );

  $effect(() => {
    void getTribes()
      .then((body) => {
        tribes = body.tribes;
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
    void getProjects()
      .then((body) => {
        projects = body.projects;
      })
      .catch(() => {
        projects = [];
      });
    void getCompanyRepos()
      .then((body) => {
        repos = body.repos;
      })
      .catch((error: unknown) => {
        reposError = error instanceof ApiError ? error.message : unreachable();
      });
  });

  function pickRepo(value: string) {
    repo = value;
    const found = repos?.find((item) => item.name === value);
    if (found) baseBranch = found.defaultBranch;
    if (name === '' && value !== '') name = value;
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    fields = {};
    message = 'Creating project';
    try {
      const project = await createProject({
        tribeId,
        name,
        repo,
        baseBranch,
        links: links.map((projectId) => ({ projectId, sharedPaths: [] }))
      });
      session.setProject(project.id);
      message = 'Project created';
      await goto('/settings/repository');
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title="New project"
  description="A project is one repository in the company GitHub organization. Members of its tribe can see its board."
>
  <form onsubmit={submit}>
    <Panel title="Project">
      <Select label="Tribe" id="tribe" error={fields.tribeId} bind:value={tribeId}>
        {#each manageable as tribe (tribe.id)}
          <option value={tribe.id}>{tribe.name}</option>
        {/each}
      </Select>
      <div class="flex flex-col gap-1.5">
        <SearchSelect
          label="Repository"
          id="repo"
          width="lg"
          placeholder={repoPlaceholder}
          options={repoOptions}
          value={repo}
          error={repoError}
          onchange={pickRepo}
        />
        {#if repos?.length === 0 && shell.admin}
          <a class="inline-flex min-h-11 items-center self-start text-xs font-medium text-accent hover:underline" href="/admin/github">Check Company GitHub</a>
        {/if}
      </div>
      <Field label="Name" id="name" error={fields.name} bind:value={name} />
      <Field
        label="Base branch"
        id="base"
        width="sm"
        mono
        error={fields.baseBranch}
        hint="Card branches start here. It must exist on GitHub."
        bind:value={baseBranch}
      />
      <div class="flex flex-col gap-1.5">
        <MultiSearchSelect
          label="Linked projects"
          id="links"
          width="lg"
          placeholder={linkOptions.length === 0 ? `No other projects in ${tribeName}` : 'Search projects'}
          options={linkOptions}
          bind:values={links}
          error={fields.links ?? fields.sharedPaths ?? ''}
        />
        <p class="text-xs text-muted">
          The new project reads requirements, decisions, and files from these projects. Add shared paths later in Linked projects.
        </p>
      </div>
      {#snippet footer()}
        <Button variant="ghost" href="/projects">Cancel</Button>
        <Button variant="primary" type="submit" disabled={busy || repo === ''}>Create project</Button>
      {/snippet}
    </Panel>
  </form>
</Page>
