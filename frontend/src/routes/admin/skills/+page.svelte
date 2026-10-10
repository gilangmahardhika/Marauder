<script lang="ts">
  import { ApiError, getConventions, unreachable } from '$lib/api';
  import { showShell } from '$lib/shell.svelte';
  import type { Skill } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Table from '$lib/ui/Table.svelte';

  let skills = $state<Skill[]>([]);
  let loaded = $state(false);
  let message = $state('Loading skills');

  showShell(() => ({ title: 'Skills', message }));

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
        {#each skills as skill (skill.id)}
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
</Page>
