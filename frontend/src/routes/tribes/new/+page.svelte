<script lang="ts">
  import { goto } from '$app/navigation';
  import { createTribe } from '$lib/api';
  import { focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  const shown = ['name'];

  let name = $state('');
  let busy = $state(false);
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: 'New tribe' }));

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failure = noFailure;
    try {
      const tribe = await createTribe(name);
      toast.success('Tribe created');
      await goto(`/tribes/${tribe.id}`);
    } catch (error) {
      failure = formFailure(error, shown);
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }
</script>

<Page title="New tribe" description="You add members on the next page.">
  <form onsubmit={submit} bind:this={form} novalidate>
    <Panel title="Tribe">
      <FormAlert message={failure.message} extra={failure.extra} />
      <Field label="Name" id="name" placeholder="Platform" error={failure.fields.name} bind:value={name} />
      {#snippet footer()}
        <Button variant="ghost" href="/tribes">Cancel</Button>
        <Button variant="primary" type="submit" icon="plus" loading={busy}>Create tribe</Button>
      {/snippet}
    </Panel>
  </form>
</Page>
