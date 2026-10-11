<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getConventions, unreachable } from '$lib/api';
  import { filterSkills, skillStateOptions, skillStates } from '$lib/conventions';
  import { pickParam } from '$lib/filter';
  import { setParams } from '$lib/params';
  import { showShell } from '$lib/shell.svelte';
  import type { Skill } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';
  import Table from '$lib/ui/Table.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  let skills = $state<Skill[]>([]);
  let loaded = $state(false);
  let message = $state('Loading skills');

  showShell(() => ({ title: 'Skills', message }));

  const params = $derived($page.url.searchParams);
  const filter = $derived({ q: params.get('q') ?? '', state: pickParam(params.get('state'), skillStates, 'all') });
  const shown = $derived(filterSkills(skills, filter));

  $effect(() => {
    void getConventions()
      .then((body) => {
        skills = body.skills;
        loaded = true;
        message = 'Skills';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });
</script>

<Page
  title="Skills"
  description="Written procedures for one kind of integration, such as OAuth. A Lead Developer enables skills for a project, and the planner attaches them to the cards that need them."
>
  {#snippet actions()}
    <Button variant="primary" href="/admin/skills/new">Add skill</Button>
  {/snippet}
  {#if loaded && skills.length === 0}
    <EmptyState title="No skills yet" description="Write the steps, configuration, security rules, and test checklist for an integration.">
      <Button variant="primary" href="/admin/skills/new">Add skill</Button>
    </EmptyState>
  {:else if skills.length > 0}
    <div class="flex flex-col gap-4">
      <Toolbar>
        <SearchBox label="Search skills" value={filter.q} placeholder="Search name, slug, or text" />
        <Segmented
          label="State"
          options={skillStateOptions(skills)}
          value={filter.state}
          onchange={(state) => setParams({ state: state === 'all' ? '' : state })}
        />
      </Toolbar>
      {#if shown.length === 0}
        <EmptyState title="No skills match" description="Change the search or state to see more skills.">
          <Button type="button" variant="secondary" onclick={() => setParams({ q: '', state: '' })}>Clear filters</Button>
        </EmptyState>
      {:else}
        <Table label="Skills">
          <thead>
            <tr class="border-b border-line bg-ground/50">
              <th class={th} scope="col">Skill</th>
              <th class={th} scope="col">Use when</th>
              <th class={th} scope="col">Version</th>
              <th class={th} scope="col"><span class="sr-only">Edit</span></th>
            </tr>
          </thead>
          <tbody>
            {#each shown as skill (skill.id)}
              <tr class={tr}>
                <td class="{td} py-2.5">
                  <span class="block font-medium text-ink">{skill.title}</span>
                  <span class="mt-0.5 block font-mono text-xs text-muted">{skill.slug}</span>
                </td>
                <td class="{td} max-w-md text-sm text-muted"><span class="line-clamp-2">{skill.useWhen}</span></td>
                <td class="{td} text-sm text-muted">
                  v{skill.version}
                  {#if skill.disabled}<span class="ml-2"><Badge tone="amber">Disabled</Badge></span>{/if}
                </td>
                <td class="{td} text-right">
                  <a
                    class="inline-flex min-h-11 items-center px-2 text-sm font-medium text-accent hover:underline"
                    href={`/admin/skills/${encodeURIComponent(skill.id)}`}
                    aria-label={`Edit ${skill.title}`}>Edit</a
                  >
                </td>
              </tr>
            {/each}
          </tbody>
        </Table>
      {/if}
    </div>
  {/if}
</Page>
