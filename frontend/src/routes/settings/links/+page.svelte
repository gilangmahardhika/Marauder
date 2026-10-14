<script lang="ts">
  import { session } from '$lib/session';
  import { getLinks, getProjects, getTribes, putLinks } from '$lib/api';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { shell, showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { ProjectLink, ProjectRecord, TribeRecord } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import { labelClass } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Row from '$lib/ui/Row.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';
  import SearchSelect from '$lib/ui/SearchSelect.svelte';

  let links = $state<ProjectLink[]>([]);
  let projects = $state<ProjectRecord[]>([]);
  let tribes = $state<TribeRecord[]>([]);
  let loaded = $state(false);
  let loading = $state('Loading linked projects');
  let loadError = $state('');
  let adding = $state('');
  let failure = $state(noFailure);
  let busy = $state(false);
  let canEdit = $state(true);
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: 'Linked projects' }));

  const self = $derived(projects.find((project) => project.id === shell.projectId));
  const tribeName = $derived(tribes.find((tribe) => tribe.id === self?.tribeId)?.name ?? 'this tribe');
  const unlinked = $derived(
    projects.filter((project) => project.id !== shell.projectId && !links.some((link) => link.projectId === project.id))
  );
  const sameTribe = $derived(unlinked.filter((project) => project.tribeId === self?.tribeId));
  const candidates = $derived([
    ...sameTribe.map((project) => ({ value: project.id, label: project.name, detail: project.repo })),
    ...unlinked
      .filter((project) => project.tribeId !== self?.tribeId)
      .map((project) => ({
        value: project.id,
        label: project.name,
        detail: `Other tribe: ${tribes.find((tribe) => tribe.id === project.tribeId)?.name ?? project.tribeId}`,
        disabled: true
      }))
  ]);

  function load() {
    loading = 'Loading linked projects';
    loadError = '';
    void Promise.all([getLinks(), getProjects(), getTribes().catch(() => ({ tribes: [] }))])
      .then(([linkBody, projectBody, tribeBody]) => {
        links = linkBody.links;
        canEdit = linkBody.canEdit;
        projects = projectBody.projects;
        tribes = tribeBody.tribes;
        loaded = true;
      })
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
  });

  function addLink(id: string) {
    const project = projects.find((item) => item.id === id);
    if (!project) return;
    links = [...links, { projectId: project.id, name: project.name, repo: project.repo, sharedPaths: [] }];
    adding = '';
  }

  function removeLink(index: number) {
    links = links.filter((_, i) => i !== index);
  }

  function addPath(index: number) {
    links[index].sharedPaths = [...links[index].sharedPaths, ''];
  }

  function removePath(index: number, pathIndex: number) {
    links[index].sharedPaths = links[index].sharedPaths.filter((_, i) => i !== pathIndex);
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failure = noFailure;
    try {
      const saved = await putLinks(
        links.map((link) => ({
          projectId: link.projectId,
          sharedPaths: link.sharedPaths.map((path) => path.trim()).filter(Boolean)
        }))
      );
      links = saved.links;
      canEdit = saved.canEdit;
      toast.success('Linked projects saved');
    } catch (error) {
      failure = formFailure(error);
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title="Linked projects"
  description="Projects in the same tribe whose context this project reads. The implementer sees their accepted requirements, decisions, and shared paths, read only."
  {loading}
  error={loadError}
  onretry={load}
>
  <form onsubmit={save} bind:this={form} novalidate>
    <fieldset class="flex min-w-0 flex-col gap-6" disabled={!canEdit}>
      <FormAlert message={failure.message} extra={failure.extra} />
      {#if loaded && links.length === 0}
        <EmptyState title="No linked projects" description="Link a project to share its requirements and files with this one." />
      {/if}
      {#each links as link, index (link.projectId)}
        <Panel title={link.name} hint={link.repo}>
          {#snippet actions()}
            <Button type="button" variant="ghost" icon="trash" disabled={busy} onclick={() => removeLink(index)}>Remove link</Button>
          {/snippet}
          <div class="flex flex-col gap-1.5">
            <p class={labelClass}>Shared paths</p>
            <p class="text-xs text-muted">Files or folders in {link.name}, relative to its repository root. At most 20.</p>
            {#if link.sharedPaths.length > 0}
              <div class="mt-1 rounded-control border border-line bg-surface">
                {#each link.sharedPaths as _, pathIndex (pathIndex)}
                  <Row
                    id="path-{link.projectId}-{pathIndex}"
                    label="Shared path {pathIndex + 1}"
                    placeholder="docs/api"
                    bind:value={links[index].sharedPaths[pathIndex]}
                    onremove={() => removePath(index, pathIndex)}
                  />
                {/each}
              </div>
            {/if}
            <div>
              <button
                type="button"
                class="mt-1 inline-flex min-h-11 items-center gap-2 px-1 text-sm font-medium text-accent hover:underline disabled:text-muted"
                disabled={link.sharedPaths.length >= 20}
                onclick={() => addPath(index)}
              >
                <Icon name="plus" />
                Add path
              </button>
            </div>
          </div>
        </Panel>
      {/each}
      <Panel title="Add link" hint={`Only projects in ${tribeName} can be linked. Projects in other tribes are listed but cannot be picked.`}>
        <SearchSelect
          label="Project"
          id="add-link"
          width="lg"
          placeholder={sameTribe.length === 0 ? `No other projects in ${tribeName}` : 'Search projects'}
          options={candidates}
          bind:value={adding}
          onchange={addLink}
        />
      </Panel>
    </fieldset>
    <SaveBar
      {busy}
      disabled={!canEdit}
      note={canEdit
        ? 'Links are one way.'
        : 'Only a Lead Developer, a tribe admin, or an admin can change these settings.'}
    />
  </form>
</Page>
