<script lang="ts">
  import { page } from '$app/stores';
  import { getConventions, getProjectConventions, putProjectConventions } from '$lib/api';
  import {
    categories,
    categoryLabel,
    categoryOptions,
    copyChoices,
    emptyChoices,
    layerRows,
    setLayerChoice,
    type LayerChoice,
    type LayerRow
  } from '$lib/conventions';
  import { matchText, pickParam } from '$lib/filter';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { setParams } from '$lib/params';
  import { session } from '$lib/session';
  import { shell, showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { ConventionCatalog, ConventionChoices, ProjectConventions } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import FilterSelect from '$lib/ui/FilterSelect.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import SearchSelect from '$lib/ui/SearchSelect.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';
  import Table from '$lib/ui/Table.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  type Kind = keyof ConventionChoices;

  const shown = ['stacks.on', 'stacks.off', 'skills.on', 'skills.off', 'rules.on', 'rules.off'];

  let view = $state<ProjectConventions>({
    tribe: { stackIds: [], skillIds: [], rulesOff: [] },
    choices: emptyChoices(),
    detectedStackIds: [],
    stacks: [],
    skills: [],
    rules: [],
    canEdit: true
  });
  let catalog = $state<ConventionCatalog>({ rules: [], stacks: [], skills: [] });
  let draft = $state<ConventionChoices>(emptyChoices());
  let adding = $state<Record<Kind, string>>({ stacks: '', skills: '', rules: '' });
  let loaded = $state(false);
  let failure = $state(noFailure);
  let busy = $state(false);
  let loading = $state('Loading conventions');
  let loadError = $state('');
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: 'Conventions' }));

  const locked = $derived(!view.canEdit);
  const tribeId = $derived(shell.projectId ? session.tribe() : '');
  const rows = $derived(layerRows(catalog, view, draft));

  const params = $derived($page.url.searchParams);
  const ruleQuery = $derived(params.get('q') ?? '');
  const ruleCategory = $derived(pickParam(params.get('category'), ['', ...categories.map((item) => item.value)], ''));
  const shownRules = $derived(
    rows.rules.filter(
      (rule) => (ruleCategory === '' || rule.category === ruleCategory) && matchText([rule.label, rule.detail], ruleQuery)
    )
  );

  const addOptions = $derived({
    stacks: catalog.stacks
      .filter((stack) => !stack.disabled && !rows.stacks.some((item) => item.id === stack.id))
      .map((stack) => ({ value: stack.id, label: stack.name })),
    skills: catalog.skills
      .filter((skill) => !skill.disabled && !rows.skills.some((item) => item.id === skill.id))
      .map((skill) => ({ value: skill.id, label: skill.title, detail: skill.slug })),
    rules: catalog.rules
      .filter((rule) => !rule.disabled && !rows.rules.some((item) => item.id === rule.id))
      .map((rule) => ({ value: rule.id, label: rule.title, detail: categoryLabel(rule.category) }))
  });

  function apply(next: ProjectConventions) {
    view = next;
    draft = copyChoices(next.choices);
  }

  function choose(kind: Kind, id: string, choice: LayerChoice) {
    draft = { ...draft, [kind]: setLayerChoice(draft[kind], id, choice) };
  }

  function add(kind: Kind, id: string) {
    adding = { ...adding, [kind]: '' };
    if (id) choose(kind, id, 'on');
  }

  function choiceOptions(item: LayerRow) {
    return [
      { value: 'inherit', label: item.inheritedOn ? 'Inherit (on)' : 'Inherit (off)' },
      { value: 'on', label: 'On' },
      { value: 'off', label: 'Off' }
    ];
  }

  function fieldError(kind: Kind) {
    return [failure.fields[`${kind}.on`], failure.fields[`${kind}.off`]].filter(Boolean).join(' ');
  }

  function load() {
    loading = 'Loading conventions';
    loadError = '';
    void Promise.all([getProjectConventions(), getConventions()])
      .then(([project, body]) => {
        catalog = body;
        apply(project);
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

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failure = noFailure;
    try {
      apply(await putProjectConventions(draft));
      toast.success('Conventions saved');
    } catch (error) {
      failure = formFailure(error, shown);
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title="Conventions"
  description="This project starts from its tribe's conventions. Switch a stack, skill, or rule on or off here, or inherit it from the tribe."
  {loading}
  error={loadError}
  onretry={load}
>
  {#snippet actions()}
    {#if tribeId}
      <Button href={`/tribes/${tribeId}/conventions`}>Tribe conventions</Button>
    {/if}
  {/snippet}
  <form onsubmit={save} bind:this={form} novalidate>
    <div class="flex flex-col gap-6">
      <FormAlert message={failure.message} extra={failure.extra} />
      <Panel title="Stacks" hint="Sync detects stacks from the repository. The tribe adds stacks for all its projects. A stack's rules apply while it is on.">
        {@render layer('stacks', 'Stacks', rows.stacks, 'No stacks yet. Sync the repository, or add one.')}
      </Panel>
      <Panel title="Skills" hint="The planner may attach only skills that are on to this project's cards.">
        {@render layer('skills', 'Skills', rows.skills, 'No skills are on. The tribe has none, or add one.')}
      </Panel>
      <Panel title="Rules" hint="Global rules and the rules of the stacks that are on apply unless the tribe or the project switches them off. An admin edits the text.">
        <div class="flex flex-col gap-4">
          {#if rows.rules.length > 0}
            <Toolbar>
              <SearchBox label="Search rules" value={ruleQuery} placeholder="Search rules" />
              <FilterSelect
                label="Category"
                options={categoryOptions}
                value={ruleCategory}
                onchange={(category) => setParams({ category })}
              />
            </Toolbar>
          {/if}
          {#if rows.rules.length > 0 && shownRules.length === 0}
            <div class="flex flex-wrap items-center gap-3">
              <p class="text-sm text-muted">No rules match.</p>
              <Button type="button" variant="secondary" icon="close" onclick={() => setParams({ q: '', category: '' })}>Clear filters</Button>
            </div>
          {:else}
            {@render layer('rules', 'Rules', shownRules, 'No rules apply yet.')}
          {/if}
        </div>
      </Panel>
    </div>
    <SaveBar
      {busy}
      disabled={locked}
      note={locked
        ? 'Only a Lead Developer, a tribe admin, or an admin can change these settings.'
        : "Saved conventions apply from each card's next attempt."}
    />
  </form>
</Page>

{#snippet layer(kind: Kind, label: string, items: LayerRow[], empty: string)}
  <div class="flex flex-col gap-4">
    {#if items.length === 0}
      <p class="text-sm text-muted">{loaded ? empty : ''}</p>
    {:else}
      <Table {label}>
        <thead>
          <tr class="border-b border-line bg-ground/50">
            <th class={th} scope="col">{kind === 'rules' ? 'Rule' : kind === 'skills' ? 'Skill' : 'Stack'}</th>
            <th class={th} scope="col">From</th>
            <th class={th} scope="col">This project</th>
          </tr>
        </thead>
        <tbody>
          {#each items as item (item.id)}
            <tr class={tr}>
              <td class={td}>
                <span class="flex flex-wrap items-center gap-2">
                  <span class="font-medium {item.applies ? 'text-ink' : 'text-muted line-through'}">{item.label}</span>
                  {#if item.category}<Badge>{categoryLabel(item.category)}</Badge>{/if}
                </span>
                {#if item.detail}<span class="block text-xs text-muted">{item.detail}</span>{/if}
              </td>
              <td class={td}>
                {#if item.choice !== 'inherit'}
                  <Badge tone="sky">Project</Badge>
                {:else if item.inherited === 'Off at tribe'}
                  <Badge tone="amber">Off at tribe</Badge>
                {:else if item.inherited}
                  <Badge>{item.inherited}</Badge>
                {/if}
              </td>
              <td class={td}>
                <Segmented
                  label={`${item.label} for this project`}
                  options={choiceOptions(item)}
                  value={item.choice}
                  disabled={locked}
                  onchange={(choice) => choose(kind, item.id, choice as LayerChoice)}
                />
              </td>
            </tr>
          {/each}
        </tbody>
      </Table>
    {/if}
    {#if !locked}
      <SearchSelect
        label={`Add ${kind === 'rules' ? 'rule' : kind === 'skills' ? 'skill' : 'stack'}`}
        id={`add-${kind}`}
        width="lg"
        placeholder={addOptions[kind].length === 0 ? 'Nothing else in the catalog' : `Add ${kind === 'rules' ? 'a rule' : kind === 'skills' ? 'a skill' : 'a stack'}`}
        options={addOptions[kind]}
        error={fieldError(kind)}
        bind:value={adding[kind]}
        onchange={(id) => add(kind, id)}
      />
    {/if}
  </div>
{/snippet}
