<script lang="ts">
  import { goto } from '$app/navigation';
  import { ApiError, saveSkill, unreachable } from '$lib/api';
  import SkillForm from '$lib/components/SkillForm.svelte';
  import { skillInput } from '$lib/conventions';
  import { showShell } from '$lib/shell.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';

  let skill = $state(skillInput());
  let fields = $state<Record<string, string>>({});
  let busy = $state(false);
  let message = $state('New skill');

  showShell(() => ({ title: 'New skill', message }));

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    fields = {};
    message = 'Saving skill';
    try {
      const saved = await saveSkill('', skill);
      message = 'Skill saved';
      await goto(`/admin/skills/${encodeURIComponent(saved.id)}`);
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page title="New skill" description="A procedure for one kind of integration. The implementer follows it and the reviewer checks it.">
  {#snippet actions()}
    <Button href="/admin/skills">Cancel</Button>
  {/snippet}
  <form onsubmit={save}>
    <Panel title="Skill">
      <SkillForm bind:skill {fields} />
    </Panel>
    <SaveBar {busy} label="Add skill" note="Only an admin can change skills. Each save is versioned." />
  </form>
</Page>
