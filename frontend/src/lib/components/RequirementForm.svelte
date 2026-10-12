<script lang="ts">
  import type { Snippet } from 'svelte';
  import AcceptanceList from '$lib/components/AcceptanceList.svelte';
  import Markdown from '$lib/components/Markdown.svelte';
  import { acceptanceChecks } from '$lib/requirements';
  import { labelClass } from '$lib/ui/control';
  import Field from '$lib/ui/Field.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';

  let {
    id = $bindable(''),
    statement = $bindable(''),
    acceptance = $bindable(''),
    idLocked = false,
    fields = {}
  }: {
    id?: string;
    statement?: string;
    acceptance?: string;
    idLocked?: boolean;
    fields?: Record<string, string>;
  } = $props();

  let view = $state('write');
  const checks = $derived(acceptanceChecks(acceptance));

  const views = [
    { value: 'write', label: 'Write' },
    { value: 'preview', label: 'Preview' }
  ];
</script>

<Field
  label="Id"
  id="requirement-id"
  width="md"
  mono
  disabled={idLocked}
  placeholder={idLocked ? '' : 'REQ-012'}
  hint={idLocked ? 'Cards and pull requests refer to this id, so it cannot change.' : 'Leave blank to use the next free REQ- number.'}
  error={fields.id}
  bind:value={id}
/>
<div class="flex flex-col gap-2">
  <Segmented label="Requirement view" options={views} value={view} onchange={(next) => (view = next)} />
  <div class="flex flex-col gap-5">
    {#if view === 'write'}
      <Field
        label="Statement"
        id="requirement-statement"
        type="textarea"
        width="full"
        rows={8}
        grow
        hint="Markdown works: headings, lists, code, bold, italic, and links."
        error={fields.statement}
        bind:value={statement}
      />
      <Field
        label="Acceptance"
        id="requirement-acceptance"
        type="textarea"
        width="full"
        rows={6}
        grow
        hint="One check per line. If you paste a markdown list, only its items are saved. Markdown works inside a line: code, bold, italic, and links."
        error={fields.acceptance}
        bind:value={acceptance}
      />
    {:else}
      {@render preview('Statement', 'min-h-48', statement.trim() === '', fields.statement, statementPreview)}
      {@render preview('Acceptance', 'min-h-36', checks.lines.length === 0, fields.acceptance, acceptancePreview)}
    {/if}
  </div>
</div>

{#snippet statementPreview()}
  <Markdown text={statement} />
{/snippet}

{#snippet acceptancePreview()}
  <AcceptanceList lines={checks.lines} label="Acceptance lines" />
  {#if checks.skipped > 0}
    <p class="mt-3 border-t border-line pt-2 text-xs text-muted">
      {checks.skipped === 1
        ? '1 line is not a list item, so it is not saved.'
        : `${checks.skipped} lines are not list items, so they are not saved.`}
    </p>
  {/if}
{/snippet}

{#snippet preview(label: string, height: string, empty: boolean, error: string | undefined, body: Snippet)}
  <div class="flex flex-col gap-1.5">
    <span class={labelClass}>{label}</span>
    <div class="{height} rounded-control border border-line bg-surface px-3 py-2.5" aria-label={`${label} preview`} role="region">
      {#if empty}
        <p class="text-sm text-muted">Nothing to preview</p>
      {:else}
        {@render body()}
      {/if}
    </div>
    {#if error}<p class="error text-xs">{error}</p>{/if}
  </div>
{/snippet}
