<script lang="ts">
  import { session } from '$lib/session';
  import { getCompanyRepos, getRepository, putRepository, syncRepository } from '$lib/api';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { shell, showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { CompanyRepo, Repository } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';
  import SearchSelect from '$lib/ui/SearchSelect.svelte';

  const shown = ['repo', 'baseBranch', 'prdPath', 'rfcPath'];

  let view = $state<Repository | null>(null);
  let repos = $state<CompanyRepo[] | null>(null);
  let reposError = $state('');
  let repo = $state('');
  let baseBranch = $state('main');
  let prdPath = $state('docs/prd');
  let rfcPath = $state('docs/rfc');
  let loading = $state('Loading repository');
  let loadError = $state('');
  let failure = $state(noFailure);
  let busy = $state(false);
  let syncing = $state(false);
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: 'Repository' }));

  const locked = $derived(view !== null && !view.canEdit);
  const canSync = $derived(
    view !== null &&
      (view.canEdit || ['lead_developer', 'senior_developer', 'developer'].includes(shell.role))
  );

  const repoOptions = $derived(
    (repos ?? []).map((item) => ({
      value: item.name,
      label: item.name,
      detail: item.archived ? `${item.defaultBranch} · archived` : item.defaultBranch,
      disabled: item.archived
    }))
  );
  const repoError = $derived(
    failure.fields.repo ||
      (locked
        ? ''
        : reposError
          ? `The repository list did not load: ${reposError}`
          : repos?.length === 0
            ? `The company token sees no repositories in ${view?.org || 'the organization'}.`
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

  function show(next: Repository) {
    view = next;
    repo = next.repo;
    baseBranch = next.baseBranch || 'main';
    prdPath = next.prdPath;
    rfcPath = next.rfcPath;
  }

  function load() {
    loading = 'Loading repository';
    loadError = '';
    void getRepository()
      .then(show)
      .catch((error: unknown) => {
        loadError = failureText(error);
      })
      .finally(() => {
        loading = '';
      });
  }

  $effect(() => {
    if (shell.projectId === '') {
      if (session.project() === '') {
        loading = '';
        loadError = 'Choose a project in the sidebar first.';
      }
      return;
    }
    load();
    void getCompanyRepos()
      .then((body) => {
        repos = body.repos;
      })
      .catch((error: unknown) => {
        repos = null;
        reposError = failureText(error);
      });
  });

  function pickRepo(value: string) {
    repo = value;
    const found = repos?.find((item) => item.name === value);
    if (found) baseBranch = found.defaultBranch;
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failure = noFailure;
    try {
      show(await putRepository({ repo, baseBranch, prdPath, rfcPath }));
      toast.success('Repository saved');
    } catch (error) {
      failure = formFailure(error, shown);
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }

  async function sync() {
    syncing = true;
    try {
      show(await syncRepository());
      toast.success('Synced');
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      syncing = false;
    }
  }
</script>

<Page
  title="Repository"
  description="The one repository this project works in. The service clones it with the company GitHub account."
  {loading}
  error={loadError}
  onretry={load}
>
  {#if view && view.org === ''}
    <div class="mb-6">
      <Notice>
        The company GitHub organization is not set.
        {#if shell.admin}<a class="font-medium text-accent hover:underline" href="/admin/github">Set it up</a>{:else}Ask an admin to set it up.{/if}
      </Notice>
    </div>
  {/if}
  <form onsubmit={save} bind:this={form} novalidate>
    <div class="flex flex-col gap-6">
      <FormAlert message={failure.message} extra={failure.extra} />
      <Panel title="Source" hint={view ? `${view.host} / ${view.org || 'no organization'}` : ''}>
        <fieldset class="flex min-w-0 flex-col gap-5" disabled={locked}>
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
          <Field
            label="Base branch"
            id="base"
            width="sm"
            mono
            error={failure.fields.baseBranch}
            hint="Card branches start here, and Sync resets the clone to it. It must exist on GitHub."
            bind:value={baseBranch}
          />
        </fieldset>
      </Panel>
      <Panel title="Docs" hint="Folders in the repository, relative to its root.">
        <fieldset class="grid min-w-0 gap-5 sm:grid-cols-2" disabled={locked}>
          <Field
            label="PRD path"
            id="prd"
            width="full"
            mono
            hint="Ingest reads requirements from this folder of the repository."
            error={failure.fields.prdPath}
            bind:value={prdPath}
          />
          <Field
            label="RFC path"
            id="rfc"
            width="full"
            mono
            hint="Ingest reads decisions from this folder of the repository."
            error={failure.fields.rfcPath}
            bind:value={rfcPath}
          />
        </fieldset>
      </Panel>
      <Panel title="Clone">
        {#snippet actions()}
          {#if view}
            <Badge tone={view.cloned ? 'green' : 'amber'}>{view.cloned ? 'Cloned' : 'Not cloned'}</Badge>
          {/if}
        {/snippet}
        <dl class="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-[8rem_minmax(0,1fr)]">
          <dt class="text-muted">Head</dt>
          <dd class="truncate font-mono text-ink">{view?.headSha ?? 'none'}</dd>
        </dl>
        {#snippet footer()}
          <Button
            type="button"
            variant="secondary"
            icon="refresh"
            loading={syncing}
            disabled={busy || !canSync || view?.repo === ''}
            onclick={() => void sync()}
          >
            Sync
          </Button>
        {/snippet}
      </Panel>
    </div>
    <SaveBar
      {busy}
      disabled={locked || syncing}
      note={locked ? 'Only a Lead Developer, a tribe admin, or an admin can change these settings.' : ''}
    />
  </form>
</Page>
