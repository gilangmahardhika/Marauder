<script lang="ts">
  import AcceptanceList from '$lib/components/AcceptanceList.svelte';
  import Markdown from '$lib/components/Markdown.svelte';
  import { changeCount, diffLines, diffWords } from '$lib/textdiff';
  import type { RefineField } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';

  let {
    field,
    before,
    after,
    modelId,
    stale = false,
    onapply,
    ondiscard
  }: {
    field: RefineField;
    /** The field's text when the refine was asked; for acceptance, the lines joined by newlines. */
    before: string;
    /** The model's new text, in the same form as before. */
    after: string;
    modelId: string;
    stale?: boolean;
    onapply: () => void;
    ondiscard: () => void;
  } = $props();

  const name = $derived(field === 'statement' ? 'Statement' : 'Acceptance');
  const splitLines = (text: string) => (text === '' ? [] : text.split('\n'));
  const beforeLines = $derived(splitLines(before));
  const afterLines = $derived(splitLines(after));

  let view = $state('changes');
  const words = $derived(field === 'statement' ? diffWords(before, after) : []);
  const lines = $derived(field === 'acceptance' ? diffLines(beforeLines, afterLines) : []);
  const lineCount = $derived(changeCount(lines));
  const unchanged = $derived(
    field === 'statement' ? words.every((part) => part.kind === 'same') : lineCount.added === 0 && lineCount.removed === 0
  );

  const views = [
    { value: 'changes', label: 'Changes' },
    { value: 'before', label: 'Before' },
    { value: 'after', label: 'After' }
  ];

  const lineLook = {
    same: 'text-ink',
    add: 'bg-green-50 text-green-950',
    del: 'bg-red-50 text-red-950 line-through'
  };
  const lineMark = { same: ' ', add: '+', del: '−' };
  const lineLabel = { same: 'Unchanged', add: 'Added', del: 'Removed' };
</script>

<section class="flex flex-col gap-3" aria-label={`Refined ${field}`}>
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div class="flex flex-col gap-0.5">
      <h3 class="text-sm font-semibold text-ink">Refined by {modelId}</h3>
      <p class="text-xs text-muted">
        {#if unchanged}
          The model kept the text as it is.
        {:else if field === 'acceptance'}
          {lineCount.added} added, {lineCount.removed} removed. Nothing is saved until you press Save.
        {:else}
          Nothing is saved until you press Save.
        {/if}
      </p>
    </div>
    <Segmented label={`${name} comparison view`} options={views} value={view} onchange={(next) => (view = next)} />
  </div>

  {#if view === 'changes' && field === 'statement'}
    <!-- prettier-ignore -->
    <p class="rounded-control border border-line bg-surface px-3 py-2.5 text-sm whitespace-pre-wrap text-ink" aria-label="Statement changes">{#each words as part, index (index)}{#if part.kind === 'add'}<ins class="bg-green-50 text-green-950 no-underline"><span class="sr-only">Added: </span>{part.text}</ins>{:else if part.kind === 'del'}<del class="bg-red-50 text-red-950"><span class="sr-only">Removed: </span>{part.text}</del>{:else}{part.text}{/if}{/each}</p>
  {:else if view === 'changes'}
    {#if lines.length === 0}
      <p class="text-sm text-muted">No acceptance lines on either side.</p>
    {:else}
      <ul class="flex flex-col overflow-hidden rounded-control border border-line bg-surface" aria-label="Acceptance changes">
        {#each lines as line, index (index)}
          <li class="flex items-start gap-2 px-3 py-1.5 text-sm {lineLook[line.kind]}">
            <span class="w-3 shrink-0 font-mono text-muted" aria-hidden="true">{lineMark[line.kind]}</span>
            <span class="sr-only">{lineLabel[line.kind]}: </span>
            <span class="min-w-0 break-words">{line.text}</span>
          </li>
        {/each}
      </ul>
    {/if}
  {:else if field === 'statement'}
    <div class="rounded-control border border-line bg-surface px-3 py-2.5" role="region" aria-label={`Statement ${view}`}>
      <Markdown text={view === 'before' ? before : after} />
    </div>
  {:else}
    {@const side = view === 'before' ? beforeLines : afterLines}
    {#if side.length === 0}
      <p class="text-sm text-muted">No acceptance lines.</p>
    {:else}
      <AcceptanceList lines={side} label={`Acceptance ${view}`} />
    {/if}
  {/if}

  {#if stale}
    <p class="text-xs text-amber-800" role="status">You edited this field after asking. Apply replaces those edits.</p>
  {/if}

  <div class="flex flex-wrap gap-2">
    <Button onclick={onapply}>Apply</Button>
    <Button variant="ghost" onclick={ondiscard}>Discard</Button>
  </div>
</section>
