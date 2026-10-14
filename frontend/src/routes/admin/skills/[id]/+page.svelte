<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { deleteConvention, getConventions, saveSkill } from '$lib/api';
  import SkillForm, { skillFields } from '$lib/components/SkillForm.svelte';
  import { editedBy, skillInput } from '$lib/conventions';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { Skill, SkillInput } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';

  const id = $derived($page.params.id ?? '');

  let current = $state<Skill | null>(null);
  let skill = $state<SkillInput | null>(null);
  let saving = $state(false);
  let deleting = $state(false);
  let confirming = $state(false);
  let loading = $state('Loading skill');
  let loadError = $state('');
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: current?.title ?? 'Skill' }));

  function load(skillId: string) {
    loading = 'Loading skill';
    loadError = '';
    void getConventions()
      .then((body) => {
        current = body.skills.find((item) => item.id === skillId) ?? null;
        skill = current ? skillInput(current) : null;
        if (!current) loadError = `There is no skill with id ${skillId}.`;
      })
      .catch((error: unknown) => {
        loadError = failureText(error);
      })
      .finally(() => {
        loading = '';
      });
  }

  $effect(() => {
    if (id !== '') load(id);
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    if (!skill) return;
    saving = true;
    failure = noFailure;
    try {
      current = await saveSkill(id, skill);
      skill = skillInput(current);
      toast.success('Skill saved');
    } catch (error) {
      failure = formFailure(error, skillFields);
      void focusInvalid(form);
    } finally {
      saving = false;
    }
  }

  async function remove() {
    deleting = true;
    try {
      await deleteConvention('skills', id);
      toast.success('Skill deleted');
      await goto('/admin/skills');
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      deleting = false;
      confirming = false;
    }
  }
</script>

<Page title={current?.title ?? 'Skill'} {loading} error={loadError} onretry={() => id && load(id)}>
  {#snippet meta()}
    {#if current}
      <Badge>v{current.version}</Badge>
      {#if current.disabled}<Badge tone="amber">Disabled</Badge>{/if}
    {/if}
  {/snippet}
  {#snippet actions()}
    <Button href="/admin/skills" icon="arrow-left">All skills</Button>
  {/snippet}
  {#if current && skill}
    <div class="flex flex-col gap-6">
      <form onsubmit={save} bind:this={form} novalidate>
        <Panel title="Skill" hint={editedBy(current)}>
          <FormAlert message={failure.message} extra={failure.extra} />
          <SkillForm bind:skill={skill!} fields={failure.fields} />
        </Panel>
        <SaveBar busy={saving} disabled={deleting} note="Changing the text makes a new version. Earlier versions stay in the revision history." />
      </form>
      <Panel title="Delete skill" hint="A skill that a project or card still names is disabled instead.">
        {#if confirming}
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm text-ink">Delete {current.title}?</span>
            <Button type="button" variant="ghost" disabled={deleting} onclick={() => (confirming = false)}>Cancel</Button>
            <Button type="button" variant="danger" icon="trash" loading={deleting} disabled={saving} onclick={() => void remove()}>Delete</Button>
          </div>
        {:else}
          <Button type="button" variant="danger" icon="trash" disabled={saving || deleting} onclick={() => (confirming = true)}>Delete skill</Button>
        {/if}
      </Panel>
    </div>
  {/if}
</Page>
