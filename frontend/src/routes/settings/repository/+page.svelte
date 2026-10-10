<script lang="ts">
  import { ApiError, getCompanyRepos, getRepository, putRepository, syncRepository, unreachable } from '$lib/api';
  import { shell, showShell } from '$lib/shell.svelte';
  import type { CompanyRepo, Repository } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';
  import SearchSelect from '$lib/ui/SearchSelect.svelte';

  let view = $state<Repository | null>(null);
  let repos = $state<CompanyRepo[] | null>(null);
  let repo = $state('');
  let baseBranch = $state('main');
  let fields = $state<Record<string, string>>({});
  let message = $state('Loading repository');
  let busy = $state(false);

  showShell(() => ({ title: 'Repository', message }));

  const repoOptions = $derived(
    (repos ?? []).map((item) => ({ value: item.name, label: item.name, detail: item.defaultBranch }))
  );

  function show(next: Repository) {
    view = next;
    repo = next.repo;
    baseBranch = next.baseBranch || 'main';
  }

  $effect(() => {
    if (shell.projectId === '') return;
    void getRepository()
      .then((body) => {
        show(body);
        message = 'Repository';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
    void getCompanyRepos()
      .then((body) => {
        repos = body.repos;
      })
      .catch(() => {
        repos = null;
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
    fields = {};
    message = 'Saving repository';
    try {
      show(await putRepository({ repo, baseBranch }));
      message = 'Repository saved';
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  async function sync() {
    busy = true;
    fields = {};
    message = 'Syncing';
    try {
      show(await syncRepository());
      message = 'Synced';
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title="Repository"
  description="The one repository this project works in. The service clones it with the company GitHub account."
  width="form"
>
  {#if view && view.org === ''}
    <div class="mb-6">
      <Notice>
        The company GitHub organization is not set.
        {#if shell.admin}<a class="font-medium text-accent hover:underline" href="/admin/github">Set it up</a>{:else}Ask an admin to set it up.{/if}
      </Notice>
    </div>
  {/if}
  <form onsubmit={save}>
    <div class="flex flex-col gap-6">
      <Panel title="Source" hint={view ? `${view.host} / ${view.org || 'no organization'}` : ''}>
        {#if repos !== null}
          <SearchSelect
            label="Repository"
            id="repo"
            width="lg"
            placeholder="Search repositories"
            options={repoOptions}
            value={repo}
            error={fields.repo}
            onchange={pickRepo}
          />
        {:else}
          <Field label="Repository" id="repo" width="lg" mono error={fields.repo} bind:value={repo} />
        {/if}
        <Field
          label="Base branch"
          id="base"
          width="sm"
          mono
          error={fields.baseBranch}
          hint="Card branches start here, and Sync resets the clone to it."
          bind:value={baseBranch}
        />
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
        {#if fields.token}<p class="error text-xs">{fields.token}</p>{/if}
        {#snippet footer()}
          <Button type="button" variant="secondary" disabled={busy || !view || view.repo === ''} onclick={() => void sync()}>
            Sync
          </Button>
        {/snippet}
      </Panel>
    </div>
    <SaveBar {busy} note="Only a Lead Developer can save." />
  </form>
</Page>
