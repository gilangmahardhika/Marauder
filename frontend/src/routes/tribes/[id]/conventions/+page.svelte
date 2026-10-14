<script lang="ts">
  import { page } from '$app/stores';
  import { getConventions, getTribeConventions, getTribes, putTribeConventions } from '$lib/api';
  import { categories, categoryLabel, categoryOptions } from '$lib/conventions';
  import { matchText, pickParam } from '$lib/filter';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { setParams } from '$lib/params';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { ConventionCatalog, TribeConventions } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import FilterSelect from '$lib/ui/FilterSelect.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import MultiSearchSelect from '$lib/ui/MultiSearchSelect.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';
  import Table from '$lib/ui/Table.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  const shownFields = ['stackIds', 'skillIds'];

  let catalog = $state<ConventionCatalog>({ rules: [], stacks: [], skills: [] });
  let canEdit = $state(false);
  let stackIds = $state<string[]>([]);
  let skillIds = $state<string[]>([]);
  let rulesOff = $state<string[]>([]);
  let tribeName = $state('');
  let loaded = $state(false);
  let failure = $state(noFailure);
  let busy = $state(false);
  let loading = $state('Loading conventions');
  let loadError = $state('');
  let form = $state<HTMLFormElement>();

  const id = $derived($page.params.id ?? '');
  const title = $derived(tribeName ? `Conventions: ${tribeName}` : 'Conventions');

  showShell(() => ({ title }));

  const stackNames = $derived(new Map(catalog.stacks.map((stack) => [stack.id, stack.name])));
  const stackOptions = $derived(
    catalog.stacks.filter((stack) => !stack.disabled).map((stack) => ({ value: stack.id, label: stack.name }))
  );
  const skillOptions = $derived(
    catalog.skills
      .filter((skill) => !skill.disabled)
      .map((skill) => ({ value: skill.id, label: skill.title, detail: skill.slug }))
  );
  const enabledStacks = $derived(new Set(catalog.stacks.filter((stack) => !stack.disabled).map((stack) => stack.id)));
  const rules = $derived(
    catalog.rules.filter((rule) => !rule.disabled && (rule.scope === 'global' || enabledStacks.has(rule.stackId)))
  );

  const params = $derived($page.url.searchParams);
  const query = $derived(params.get('q') ?? '');
  const category = $derived(pickParam(params.get('category'), ['', ...categories.map((item) => item.value)], ''));
  const shown = $derived(
    rules.filter(
      (rule) =>
        (category === '' || rule.category === category) &&
        matchText([rule.title, rule.scope === 'global' ? 'Global' : stackNames.get(rule.stackId)], query)
    )
  );

  const switchOptions = [
    { value: 'on', label: 'On' },
    { value: 'off', label: 'Off' }
  ];

  function apply(next: TribeConventions) {
    canEdit = next.canEdit;
    stackIds = [...next.stackIds];
    skillIds = [...next.skillIds];
    rulesOff = [...next.rulesOff];
  }

  function setRule(ruleId: string, value: string) {
    rulesOff = value === 'off' ? [...rulesOff.filter((item) => item !== ruleId), ruleId] : rulesOff.filter((item) => item !== ruleId);
  }

  async function load(tribeId: string) {
    loading = 'Loading conventions';
    loadError = '';
    try {
      const [layer, body, tribes] = await Promise.all([getTribeConventions(tribeId), getConventions(), getTribes()]);
      catalog = body;
      apply(layer);
      tribeName = tribes.tribes.find((tribe) => tribe.id === tribeId)?.name ?? '';
      loaded = true;
    } catch (error) {
      loadError = failureText(error);
    } finally {
      loading = '';
    }
  }

  $effect(() => {
    if (id) void load(id);
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failure = noFailure;
    try {
      apply(await putTribeConventions(id, { stackIds, skillIds, rulesOff }));
      toast.success('Conventions saved');
    } catch (error) {
      failure = formFailure(error, shownFields);
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }
</script>

<Page
  {title}
  description="Company stacks and skills every project of this tribe starts with, and the company rules it switches off. A project can change each one."
  {loading}
  error={loadError}
  onretry={() => id && load(id)}
>
  <form onsubmit={save} bind:this={form} novalidate>
    <div class="flex flex-col gap-6">
      <FormAlert message={failure.message} extra={failure.extra} />
      <Panel title="Stacks for every project" hint="Their rules apply to each project, as if Sync had detected them.">
        <MultiSearchSelect
          label="Stacks"
          id="tribe-stacks"
          width="lg"
          max={30}
          placeholder={stackOptions.length === 0 ? 'No stacks in the catalog' : 'Search stacks'}
          options={stackOptions}
          bind:values={stackIds}
          disabled={!canEdit}
          error={failure.fields.stackIds ?? ''}
        />
      </Panel>
      <Panel title="Skills for every project" hint="The planner may attach these skills to the cards of every project.">
        <MultiSearchSelect
          label="Skills"
          id="tribe-skills"
          width="lg"
          max={30}
          placeholder={skillOptions.length === 0 ? 'No skills in the catalog' : 'Search skills'}
          options={skillOptions}
          bind:values={skillIds}
          disabled={!canEdit}
          error={failure.fields.skillIds ?? ''}
        />
      </Panel>
      <Panel title="Rules" hint="Company rules are on unless the tribe switches them off. A stack rule applies only to projects with that stack.">
        {#if rules.length === 0}
          <p class="text-sm text-muted">{loaded ? 'The catalog has no rules.' : ''}</p>
        {:else}
          <div class="flex flex-col gap-4">
            <Toolbar>
              <SearchBox label="Search rules" value={query} placeholder="Search rules" />
              <FilterSelect label="Category" options={categoryOptions} value={category} onchange={(next) => setParams({ category: next })} />
            </Toolbar>
            {#if shown.length === 0}
              <div class="flex flex-wrap items-center gap-3">
                <p class="text-sm text-muted">No rules match.</p>
                <Button type="button" variant="secondary" icon="close" onclick={() => setParams({ q: '', category: '' })}>Clear filters</Button>
              </div>
            {:else}
              <Table label="Rules">
                <thead>
                  <tr class="border-b border-line bg-ground/50">
                    <th class={th} scope="col">Rule</th>
                    <th class={th} scope="col">From</th>
                    <th class={th} scope="col">Category</th>
                    <th class={th} scope="col">This tribe</th>
                  </tr>
                </thead>
                <tbody>
                  {#each shown as rule (rule.id)}
                    <tr class={tr}>
                      <td class="{td} font-medium {rulesOff.includes(rule.id) ? 'text-muted line-through' : 'text-ink'}">{rule.title}</td>
                      <td class="{td} text-sm text-muted">{rule.scope === 'global' ? 'Global' : stackNames.get(rule.stackId)}</td>
                      <td class={td}><Badge>{categoryLabel(rule.category)}</Badge></td>
                      <td class={td}>
                        <Segmented
                          label={`${rule.title} for this tribe`}
                          options={switchOptions}
                          value={rulesOff.includes(rule.id) ? 'off' : 'on'}
                          disabled={!canEdit}
                          onchange={(value) => setRule(rule.id, value)}
                        />
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </Table>
            {/if}
          </div>
        {/if}
      </Panel>
    </div>
    {#if canEdit}
      <SaveBar {busy} note="Saved conventions apply to the tribe's projects from each card's next attempt." />
    {:else if loaded}
      <p class="mt-6 text-sm text-muted">Only a tribe admin or an admin can change the tribe's conventions.</p>
    {/if}
  </form>
</Page>
