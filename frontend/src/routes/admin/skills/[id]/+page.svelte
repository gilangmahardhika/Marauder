<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { ApiError, deleteConvention, getConventions, saveSkill, unreachable } from '$lib/api';
  import SkillForm from '$lib/components/SkillForm.svelte';
  import { editedBy, skillInput } from '$lib/conventions';
  import { showShell } from '$lib/shell.svelte';
  import type { Skill, SkillInput } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';

  const id = $derived($page.params.id ?? '');

  let current = $state<Skill | null>(null);
  let skill = $state<SkillInput | null>(null);
  let missing = $state(false);
  let fields = $state<Record<string, string>>({});
  let busy = $state(false);
  let confirming = $state(false);
  let message = $state('Loading skill');

  showShell(() => ({ title: current?.title ?? 'Skill', message }));

  $effect(() => {
    if (id === '') return;
    void getConventions()
      .then((body) => {
        current = body.skills.find((item) => item.id === id) ?? null;
        skill = current ? skillInput(current) : null;
        missing = current === null;
        message = current?.title ?? 'Skill not found';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    if (!skill) return;
    busy = true;
    fields = {};
    message = 'Saving skill';
    try {
      current = await saveSkill(id, skill);
      skill = skillInput(current);
      message = 'Skill saved';
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  async function remove() {
    busy = true;
    message = 'Deleting skill';
    try {
      await deleteConvention('skills', id);
      message = 'Skill deleted';
      await goto('/admin/skills');
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
      confirming = false;
    }
  }
</script>

{#if missing}
  <Page title="Skill not found">
    <EmptyState title={`There is no skill with id ${id}.`} href="/admin/skills" link="All skills" />
  </Page>
{:else if current && skill}
  <Page title={current.title}>
    {#snippet meta()}
      <Badge>v{current!.version}</Badge>
      {#if current!.disabled}<Badge tone="amber">Disabled</Badge>{/if}
    {/snippet}
    {#snippet actions()}
      <Button href="/admin/skills">All skills</Button>
    {/snippet}
    <div class="flex flex-col gap-6">
      <form onsubmit={save}>
        <Panel title="Skill" hint={editedBy(current)}>
          <SkillForm bind:skill={skill!} {fields} />
        </Panel>
        <SaveBar {busy} note="Changing the text makes a new version. Earlier versions stay in the revision history." />
      </form>
      <Panel title="Delete skill" hint="A skill that a project or card still names is disabled instead.">
        {#if confirming}
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm text-ink">Delete {current.title}?</span>
            <Button type="button" variant="ghost" disabled={busy} onclick={() => (confirming = false)}>Cancel</Button>
            <Button type="button" variant="danger" disabled={busy} onclick={() => void remove()}>Delete</Button>
          </div>
        {:else}
          <Button type="button" variant="danger" disabled={busy} onclick={() => (confirming = true)}>Delete skill</Button>
        {/if}
      </Panel>
    </div>
  </Page>
{/if}
