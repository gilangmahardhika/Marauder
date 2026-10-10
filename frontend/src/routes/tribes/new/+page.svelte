<script lang="ts">
  import { goto } from '$app/navigation';
  import { ApiError, createTribe, unreachable } from '$lib/api';
  import { showShell } from '$lib/shell.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let name = $state('');
  let message = $state('New tribe');
  let busy = $state(false);
  let fields = $state<Record<string, string>>({});

  showShell(() => ({ title: 'New tribe', message }));

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    message = 'Creating tribe';
    fields = {};
    try {
      const tribe = await createTribe(name);
      message = 'Tribe created';
      await goto(`/tribes/${tribe.id}`);
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page title="New tribe" description="You add members on the next page." width="form">
  <form onsubmit={submit}>
    <Panel title="Tribe">
      <Field label="Name" id="name" placeholder="Platform" error={fields.name} bind:value={name} />
      {#snippet footer()}
        <Button variant="ghost" href="/tribes">Cancel</Button>
        <Button variant="primary" type="submit" disabled={busy}>Create tribe</Button>
      {/snippet}
    </Panel>
  </form>
</Page>
