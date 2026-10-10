<script lang="ts">
  import { goto } from '$app/navigation';
  import { ApiError, addRequirement, unreachable } from '$lib/api';
  import RequirementForm from '$lib/components/RequirementForm.svelte';
  import { toLines } from '$lib/requirements';
  import { showShell } from '$lib/shell.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let id = $state('');
  let statement = $state('');
  let acceptance = $state('');
  let fields = $state<Record<string, string>>({});
  let message = $state('New requirement');
  let busy = $state(false);

  showShell(() => ({ title: 'New requirement', message }));

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    fields = {};
    if (statement.trim() === '') {
      fields = { statement: 'Enter a statement.' };
      return;
    }
    busy = true;
    message = 'Adding requirement';
    try {
      await addRequirement({ id: id.trim(), statement, acceptance: toLines(acceptance) });
      message = 'Requirement added';
      await goto('/intake');
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title="New requirement"
  description="Added to this project's intake. It is not accepted until a senior member accepts it."
>
  <form onsubmit={submit}>
    <Panel title="Requirement">
      <RequirementForm bind:id bind:statement bind:acceptance {fields} />
      {#snippet footer()}
        <Button variant="ghost" href="/intake">Cancel</Button>
        <Button variant="primary" type="submit" disabled={busy}>Add requirement</Button>
      {/snippet}
    </Panel>
  </form>
</Page>
