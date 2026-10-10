<script lang="ts">
  import { ApiError, getLinks, getProjects, putLinks, unreachable } from '$lib/api';
  import { shell, showShell } from '$lib/shell.svelte';
  import type { ProjectLink, ProjectRecord } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import { labelClass } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Row from '$lib/ui/Row.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';
  import SearchSelect from '$lib/ui/SearchSelect.svelte';

  let links = $state<ProjectLink[]>([]);
  let projects = $state<ProjectRecord[]>([]);
  let loaded = $state(false);
  let adding = $state('');
  let fields = $state<Record<string, string>>({});
  let message = $state('Loading linked projects');
  let busy = $state(false);

  showShell(() => ({ title: 'Linked projects', message }));

  const self = $derived(projects.find((project) => project.id === shell.projectId));
  const candidates = $derived(
    projects
      .filter(
        (project) =>
          project.tribeId === self?.tribeId &&
          project.id !== shell.projectId &&
          !links.some((link) => link.projectId === project.id)
      )
      .map((project) => ({ value: project.id, label: project.name, detail: project.repo }))
  );

  $effect(() => {
    if (shell.projectId === '') return;
    void Promise.all([getLinks(), getProjects()])
      .then(([linkBody, projectBody]) => {
        links = linkBody.links;
        projects = projectBody.projects;
        loaded = true;
        message = 'Linked projects';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
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
    fields = {};
    message = 'Saving linked projects';
    try {
      const saved = await putLinks(
        links.map((link) => ({
          projectId: link.projectId,
          sharedPaths: link.sharedPaths.map((path) => path.trim()).filter(Boolean)
        }))
      );
      links = saved.links;
      message = 'Linked projects saved';
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title="Linked projects"
  description="Projects in the same tribe whose context this project reads. The implementer sees their accepted requirements, decisions, and shared paths, read only."
>
  <form onsubmit={save}>
    <div class="flex flex-col gap-6">
      {#if fields.links || fields.sharedPaths}
        <Notice tone="danger">{fields.links ?? fields.sharedPaths}</Notice>
      {/if}
      {#if loaded && links.length === 0}
        <EmptyState title="No linked projects" description="Link a project to share its requirements and files with this one." />
      {/if}
      {#each links as link, index (link.projectId)}
        <Panel title={link.name} hint={link.repo}>
          {#snippet actions()}
            <Button type="button" variant="ghost" disabled={busy} onclick={() => removeLink(index)}>Remove link</Button>
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
      <Panel title="Add link" hint="Only projects in this project's tribe can be linked.">
        <SearchSelect
          label="Project"
          id="add-link"
          width="lg"
          placeholder={candidates.length === 0 ? 'No other projects in this tribe' : 'Search projects'}
          options={candidates}
          value={adding}
          onchange={addLink}
        />
      </Panel>
    </div>
    <SaveBar {busy} note="Only a Lead Developer can save. Links are one way." />
  </form>
</Page>
