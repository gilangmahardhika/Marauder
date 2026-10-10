<script lang="ts">
  import { goto } from '$app/navigation';
  import { ApiError, getLLM, putLLM, unreachable } from '$lib/api';
  import FamilyForm from '$lib/components/FamilyForm.svelte';
  import { blankFamily, fromDraft, keepKey } from '$lib/llm';
  import { showShell } from '$lib/shell.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let family = $state(blankFamily());
  let fields = $state<Record<string, string>>({});
  let message = $state('New LLM family');
  let busy = $state(false);

  showShell(() => ({ title: 'New LLM family', message }));

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    fields = {};
    const id = family.id.trim();
    if (id === '') {
      fields = { id: 'Enter an id.' };
      return;
    }
    if (id === 'new') {
      fields = { id: 'Choose another id. "new" is reserved.' };
      return;
    }
    busy = true;
    message = 'Creating family';
    try {
      const current = await getLLM();
      if (current.families.some((item) => item.id === id)) {
        fields = { id: 'A family with this id already exists.' };
        message = 'New LLM family';
        return;
      }
      await putLLM({ families: [...current.families.map(keepKey), fromDraft(family)] });
      message = 'Family created';
      await goto('/admin/llm');
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page title="New LLM family" description="One provider endpoint and its key. Add every model id it serves.">
  <form onsubmit={submit}>
    <Panel title="Family">
      <FamilyForm bind:family {fields} />
      {#snippet footer()}
        <Button variant="ghost" href="/admin/llm">Cancel</Button>
        <Button variant="primary" type="submit" disabled={busy}>Create family</Button>
      {/snippet}
    </Panel>
  </form>
</Page>
