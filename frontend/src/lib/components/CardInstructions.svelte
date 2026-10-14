<script lang="ts">
  import { setInstructions } from '$lib/api';
  import { focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { toast } from '$lib/toast.svelte';
  import type { Card } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Markdown from './Markdown.svelte';

  const shown = ['instructions'];

  let { card = $bindable() }: { card: Card } = $props();

  let editing = $state(false);
  let text = $state('');
  let failure = $state(noFailure);
  let busy = $state(false);
  let form = $state<HTMLFormElement>();

  const current = $derived(card.instructions ?? '');

  function edit() {
    text = current;
    failure = noFailure;
    editing = true;
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failure = noFailure;
    try {
      card = await setInstructions(card.id, text);
      editing = false;
      toast.success('Instructions saved');
    } catch (error) {
      failure = formFailure(error, shown);
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }
</script>

{#snippet editButton()}
  <Button type="button" variant="secondary" icon="pencil" onclick={edit}>Edit</Button>
{/snippet}

<Panel
  title="Instructions"
  hint="Every later attempt reads these with the newest note, until you change them. An attempt that already started keeps the prompt it started with."
  actions={editing ? undefined : editButton}
>
  {#if editing}
    <form class="flex flex-col items-start gap-3" onsubmit={save} bind:this={form} novalidate>
      <FormAlert message={failure.message} extra={failure.extra} />
      <Field
        label="Instructions for later attempts"
        id="card-instructions"
        type="textarea"
        width="full"
        rows={4}
        grow
        hint="Leave it empty to clear them. Markdown works."
        error={failure.fields.instructions}
        bind:value={text}
      />
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="primary" type="submit" icon="save" loading={busy}>Save</Button>
        <Button variant="ghost" type="button" disabled={busy} onclick={() => (editing = false)}>Cancel</Button>
      </div>
    </form>
  {:else if current}
    <Markdown text={current} headingBase={4} />
  {:else}
    <p class="text-sm text-muted">No instructions. Attempts follow the requirements and the newest note.</p>
  {/if}
</Panel>
