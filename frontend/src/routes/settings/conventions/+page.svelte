<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getConventions, getProjectConventions, putProjectConventions, unreachable } from '$lib/api';
  import { categories, categoryLabel, categoryOptions, filterRules } from '$lib/conventions';
  import { pickParam } from '$lib/filter';
  import { setParams } from '$lib/params';
  import { shell, showShell } from '$lib/shell.svelte';
  import type { ConventionCatalog, ProjectConventions } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import FilterSelect from '$lib/ui/FilterSelect.svelte';
  import MultiSearchSelect from '$lib/ui/MultiSearchSelect.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import Table from '$lib/ui/Table.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  let view = $state<ProjectConventions>({ stacks: [], skillIds: [], rules: [] });
  let catalog = $state<ConventionCatalog>({ rules: [], stacks: [], skills: [] });
  let manual = $state<string[]>([]);
  let skillIds = $state<string[]>([]);
  let loaded = $state(false);
  let fields = $state<Record<string, string>>({});
  let busy = $state(false);
  let message = $state('Loading conventions');

  showShell(() => ({ title: 'Conventions', message }));

  const detected = $derived(view.stacks.filter((stack) => stack.source === 'detected'));
  const stackOptions = $derived(
    catalog.stacks
      .filter((stack) => !stack.disabled && !detected.some((item) => item.id === stack.id))
      .map((stack) => ({ value: stack.id, label: stack.name }))
  );
  const skillOptions = $derived(
    catalog.skills
      .filter((skill) => !skill.disabled)
      .map((skill) => ({ value: skill.id, label: skill.title, detail: skill.slug }))
  );

  const params = $derived($page.url.searchParams);
  const ruleFilter = $derived({
    q: params.get('q') ?? '',
    category: pickParam(params.get('category'), ['', ...categories.map((item) => item.value)], ''),
    applies: ''
  });
  const rules = $derived(filterRules(view.rules, ruleFilter));

  function apply(next: ProjectConventions) {
    view = next;
    manual = next.stacks.filter((stack) => stack.source === 'manual').map((stack) => stack.id);
    skillIds = [...next.skillIds];
  }

  $effect(() => {
    if (shell.projectId === '') return;
    void Promise.all([getProjectConventions(), getConventions()])
      .then(([project, body]) => {
        catalog = body;
        apply(project);
        loaded = true;
        message = 'Conventions';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    fields = {};
    message = 'Saving conventions';
    try {
      apply(await putProjectConventions({ manualStackIds: manual, skillIds }));
      message = 'Conventions saved';
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title="Conventions"
  description="The company rules this project's cards follow, and the integration skills the planner may attach to them."
>
  <form onsubmit={save}>
    <div class="flex flex-col gap-6">
      <Panel title="Stacks" hint="Sync detects stacks from the repository. Add a stack by hand when the repository has no files for it yet.">
        <div class="flex flex-col gap-5">
          <div class="flex flex-col gap-1.5">
            <p class="text-[13px] font-medium text-ink">Detected on Sync</p>
            {#if detected.length > 0}
              <div class="flex flex-wrap gap-2">
                {#each detected as stack (stack.id)}
                  <Badge tone="sky">{stack.name}</Badge>
                {/each}
              </div>
            {:else}
              <p class="text-sm text-muted">{loaded ? 'None yet. Sync the repository to detect its stacks.' : ''}</p>
            {/if}
          </div>
          <MultiSearchSelect
            label="Added by hand"
            id="manual-stacks"
            width="lg"
            max={30}
            placeholder={stackOptions.length === 0 ? 'No other stacks in the catalog' : 'Search stacks'}
            options={stackOptions}
            bind:values={manual}
            error={fields.manualStackIds ?? ''}
          />
        </div>
      </Panel>
      <Panel title="Skills" hint="The planner may attach only these skills to this project's cards.">
        <MultiSearchSelect
          label="Enabled skills"
          id="enabled-skills"
          width="lg"
          max={30}
          placeholder={skillOptions.length === 0 ? 'No skills in the catalog' : 'Search skills'}
          options={skillOptions}
          bind:values={skillIds}
          error={fields.skillIds ?? ''}
        />
      </Panel>
      <Panel title="Rules that apply" hint="Global rules and the rules of this project's stacks, as saved. An admin edits them.">
        {#if view.rules.length === 0}
          <p class="text-sm text-muted">{loaded ? 'No rules apply yet.' : ''}</p>
        {:else}
          <div class="flex flex-col gap-4">
            <Toolbar>
              <SearchBox label="Search rules" value={ruleFilter.q} placeholder="Search rules" />
              <FilterSelect
                label="Category"
                options={categoryOptions}
                value={ruleFilter.category}
                onchange={(category) => setParams({ category })}
              />
            </Toolbar>
            {#if rules.length === 0}
              <div class="flex flex-wrap items-center gap-3">
                <p class="text-sm text-muted">No rules match.</p>
                <Button type="button" variant="secondary" onclick={() => setParams({ q: '', category: '' })}>Clear filters</Button>
              </div>
            {:else}
              <Table label="Rules that apply">
                <thead>
                  <tr class="border-b border-line bg-ground/50">
                    <th class={th} scope="col">Rule</th>
                    <th class={th} scope="col">From</th>
                    <th class={th} scope="col">Category</th>
                    <th class={th} scope="col">Version</th>
                  </tr>
                </thead>
                <tbody>
                  {#each rules as rule (rule.id)}
                    <tr class={tr}>
                      <td class="{td} font-medium text-ink">{rule.title}</td>
                      <td class="{td} text-sm text-muted">{rule.scope === 'global' ? 'Global' : rule.stackName}</td>
                      <td class={td}><Badge>{categoryLabel(rule.category)}</Badge></td>
                      <td class="{td} text-sm text-muted">v{rule.version}</td>
                    </tr>
                  {/each}
                </tbody>
              </Table>
            {/if}
          </div>
        {/if}
      </Panel>
    </div>
    <SaveBar {busy} note="Only a Lead Developer can save. Saved rules apply from the next attempt." />
  </form>
</Page>
