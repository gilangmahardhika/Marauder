<script lang="ts">
  import { ApiError, setInstructions, unreachable } from '$lib/api';
  import { shell } from '$lib/shell.svelte';
  import type { Card } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Markdown from './Markdown.svelte';

  let { card = $bindable() }: { card: Card } = $props();

  let editing = $state(false);
  let text = $state('');
  let error = $state('');
  let busy = $state(false);

  const current = $derived(card.instructions ?? '');

  function edit() {
    text = current;
    error = '';
    editing = true;
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    error = '';
    shell.message = 'Saving instructions';
    try {
      card = await setInstructions(card.id, text);
      editing = false;
      shell.message = 'Instructions saved';
    } catch (failure) {
      error = failure instanceof ApiError ? (failure.fields.instructions ?? failure.message) : unreachable();
      shell.message = failure instanceof ApiError ? failure.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

{#snippet editButton()}
  <Button type="button" variant="secondary" onclick={edit}>Edit</Button>
{/snippet}

<Panel
  title="Instructions"
  hint="Every later attempt reads these with the newest note, until you change them. An attempt that already started keeps the prompt it started with."
  actions={editing ? undefined : editButton}
>
  {#if editing}
    <form class="flex flex-col items-start gap-3" onsubmit={save}>
      <Field
        label="Instructions for later attempts"
        id="card-instructions"
        type="textarea"
        width="full"
        rows={4}
        grow
        hint="Leave it empty to clear them. Markdown works."
        {error}
        bind:value={text}
      />
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="primary" type="submit" disabled={busy}>Save</Button>
        <Button variant="ghost" type="button" disabled={busy} onclick={() => (editing = false)}>Cancel</Button>
      </div>
    </form>
  {:else if current}
    <Markdown text={current} headingBase={4} />
  {:else}
    <p class="text-sm text-muted">No instructions. Attempts follow the requirements and the newest note.</p>
  {/if}
</Panel>
