<script lang="ts">
  import AcceptanceList from "$lib/components/AcceptanceList.svelte";
  import Markdown from "$lib/components/Markdown.svelte";
  import { changeCount, diffLines, diffWords } from "$lib/textdiff";
  import type { RefineField } from "$lib/types";
  import Icon from "$lib/ui/Icon.svelte";
  import Segmented from "$lib/ui/Segmented.svelte";

  let {
    field,
    before,
    after,
    modelId,
    stale = false,
    onapply,
    ondiscard,
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

  const name = $derived(field === "statement" ? "Statement" : "Acceptance");
  const splitLines = (text: string) => (text === "" ? [] : text.split("\n"));
  const beforeLines = $derived(splitLines(before));
  const afterLines = $derived(splitLines(after));

  let view = $state("changes");
  const words = $derived(field === "statement" ? diffWords(before, after) : []);
  const lines = $derived(
    field === "acceptance" ? diffLines(beforeLines, afterLines) : [],
  );
  const lineCount = $derived(changeCount(lines));
  const unchanged = $derived(
    field === "statement"
      ? words.every((part) => part.kind === "same")
      : lineCount.added === 0 && lineCount.removed === 0,
  );

  const views = [
    { value: "changes", label: "Changes" },
    { value: "before", label: "Before" },
    { value: "after", label: "After" },
  ];

  const lineLook = {
    same: "text-ink",
    add: "bg-green-50 text-green-950",
    del: "bg-red-50 text-red-950 line-through",
  };
  const lineMark = { same: " ", add: "+", del: "−" };
  const lineLabel = { same: "Unchanged", add: "Added", del: "Removed" };
</script>

<section class="flex flex-col gap-3" aria-label={`Refined ${field}`}>
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div class="flex flex-col gap-0.5">
      <h3 class="text-sm font-semibold text-ink">Refined by {modelId}</h3>
      {#if unchanged}
        <p class="text-xs text-muted">The model kept the text as it is.</p>
      {:else if field === "acceptance"}
        <p class="text-xs text-muted">
          {lineCount.added} added, {lineCount.removed} removed
        </p>
      {/if}
    </div>
    <Segmented
      label={`${name} comparison view`}
      options={views}
      value={view}
      onchange={(next) => (view = next)}
    />
  </div>

  <div class="overflow-hidden rounded-xl border border-line bg-surface">
    {#if view === "changes" && field === "statement"}
      <!-- prettier-ignore -->
      <p class="px-3.5 py-3 text-sm whitespace-pre-wrap text-ink" aria-label="Statement changes">{#each words as part, index (index)}{#if part.kind === 'add'}<ins class="bg-green-50 text-green-950 no-underline"><span class="sr-only">Added: </span>{part.text}</ins>{:else if part.kind === 'del'}<del class="bg-red-50 text-red-950"><span class="sr-only">Removed: </span>{part.text}</del>{:else}{part.text}{/if}{/each}</p>
    {:else if view === "changes"}
      {#if lines.length === 0}
        <p class="px-3.5 py-3 text-sm text-muted">
          No acceptance lines on either side.
        </p>
      {:else}
        <ul class="flex flex-col py-1.5" aria-label="Acceptance changes">
          {#each lines as line, index (index)}
            <li
              class="flex items-start gap-2 px-3.5 py-1.5 text-sm {lineLook[
                line.kind
              ]}"
            >
              <span class="w-3 shrink-0 font-mono text-muted" aria-hidden="true"
                >{lineMark[line.kind]}</span
              >
              <span class="sr-only">{lineLabel[line.kind]}: </span>
              <span class="min-w-0 wrap-break-word">{line.text}</span>
            </li>
          {/each}
        </ul>
      {/if}
    {:else if field === "statement"}
      <div class="px-3.5 py-3" role="region" aria-label={`Statement ${view}`}>
        <Markdown text={view === "before" ? before : after} />
      </div>
    {:else}
      {@const side = view === "before" ? beforeLines : afterLines}
      <div class="px-3.5 py-3">
        {#if side.length === 0}
          <p class="text-sm text-muted">No acceptance lines.</p>
        {:else}
          <AcceptanceList lines={side} label={`Acceptance ${view}`} />
        {/if}
      </div>
    {/if}

    <div
      class="flex flex-wrap items-center gap-2 border-t border-line bg-ground/40 py-1 pr-1 pl-3.5"
    >
      {#if stale}
        <p
          class="flex items-center gap-1.5 text-xs text-amber-800"
          role="status"
        >
          <Icon name="alert" class="size-3.5 shrink-0" />
          You edited this field after asking. Apply replaces those edits.
        </p>
      {:else}
        <p class="text-xs text-muted">Nothing is saved until you press Save.</p>
      {/if}
      <div class="ml-auto flex gap-1">
        <button
          type="button"
          class="inline-flex min-h-11 items-center rounded-control px-3 text-sm font-medium text-muted transition-colors duration-150 hover:bg-ground hover:text-ink"
          onclick={ondiscard}
        >
          Discard
        </button>
        <button
          type="button"
          class="inline-flex min-h-11 items-center gap-1.5 rounded-control bg-ink px-4 text-sm font-medium text-surface shadow-xs transition-colors duration-150 hover:bg-ink/85 active:translate-y-px"
          onclick={onapply}
        >
          <Icon name="check" class="size-4" />
          Apply
        </button>
      </div>
    </div>
  </div>
</section>
