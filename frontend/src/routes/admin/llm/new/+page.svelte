<script lang="ts">
  import { goto } from '$app/navigation';
  import { getLLM, putLLM } from '$lib/api';
  import FamilyForm, { familyFields } from '$lib/components/FamilyForm.svelte';
  import { focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { blankFamily, fromDraft, keepKey } from '$lib/llm';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import Button from '$lib/ui/Button.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let family = $state(blankFamily());
  let busy = $state(false);
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: 'New LLM family' }));

  function refuse(id: string) {
    failure = { ...noFailure, fields: { id } };
    void focusInvalid(form);
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failure = noFailure;
    const id = family.id.trim();
    if (id === '') return refuse('Enter an id.');
    if (id === 'new') return refuse('Choose another id. "new" is reserved.');
    busy = true;
    try {
      const current = await getLLM();
      if (current.families.some((item) => item.id === id)) return refuse('A family with this id already exists.');
      await putLLM({ families: [...current.families.map(keepKey), fromDraft(family)] });
      toast.success('Family created');
      await goto('/admin/llm');
    } catch (error) {
      failure = formFailure(error, familyFields(family));
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }
</script>

<Page title="New LLM family" description="One provider endpoint and its key. Add every model id it serves.">
  <form onsubmit={submit} bind:this={form} novalidate>
    <Panel title="Family">
      <FormAlert message={failure.message} extra={failure.extra} />
      <FamilyForm bind:family fields={failure.fields} />
      {#snippet footer()}
        <Button variant="ghost" href="/admin/llm">Cancel</Button>
        <Button variant="primary" type="submit" icon="plus" loading={busy}>Create family</Button>
      {/snippet}
    </Panel>
  </form>
</Page>
