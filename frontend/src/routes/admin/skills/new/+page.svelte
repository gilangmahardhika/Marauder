<script lang="ts">
  import { goto } from '$app/navigation';
  import { saveSkill } from '$lib/api';
  import SkillForm, { skillFields } from '$lib/components/SkillForm.svelte';
  import { skillInput } from '$lib/conventions';
  import { focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import Button from '$lib/ui/Button.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';

  let skill = $state(skillInput());
  let busy = $state(false);
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: 'New skill' }));

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failure = noFailure;
    try {
      const saved = await saveSkill('', skill);
      toast.success('Skill saved');
      await goto(`/admin/skills/${encodeURIComponent(saved.id)}`);
    } catch (error) {
      failure = formFailure(error, skillFields);
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }
</script>

<Page title="New skill" description="A procedure for one kind of integration. The implementer follows it and the reviewer checks it.">
  {#snippet actions()}
    <Button href="/admin/skills">Cancel</Button>
  {/snippet}
  <form onsubmit={save} bind:this={form} novalidate>
    <Panel title="Skill">
      <FormAlert message={failure.message} extra={failure.extra} />
      <SkillForm bind:skill fields={failure.fields} />
    </Panel>
    <SaveBar {busy} label="Add skill" icon="plus" note="Only an admin can change skills. Each save is versioned." />
  </form>
</Page>
