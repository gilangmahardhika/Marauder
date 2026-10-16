<script lang="ts">
  import { highlightLine, langFromPath } from '$lib/codeHighlight';
  import { count } from '$lib/format';
  import { changeCount, diffLines } from '$lib/textdiff';
  import { clipLines, foldLines, type TerminalEdit } from '$lib/terminal';

  let { edit }: { edit: TerminalEdit } = $props();

  let expanded = $state(false);

  const lang = $derived(langFromPath(edit.path));
  const parts = $derived(diffLines(split(edit.old), split(edit.new)));
  const stats = $derived(changeCount(parts));
  const rows = $derived(
    parts.flatMap((part) => {
      const sign = part.kind === 'add' ? '+' : part.kind === 'del' ? '-' : ' ';
      const tone = part.kind === 'add' ? 'add' : part.kind === 'del' ? 'del' : 'same';
      return part.text.split('\n').map((text) => ({ sign, tone, text }));
    })
  );
  const clipped = $derived(expanded ? { shown: rows, hidden: 0 } : clipRows(rows));

  function split(text: string): string[] {
    if (text === '') return [];
    return text.replace(/\n$/, '').split('\n');
  }

  function clipRows<T>(list: T[], max = foldLines): { shown: T[]; hidden: number } {
    if (list.length <= max) return { shown: list, hidden: 0 };
    return { shown: list.slice(0, max), hidden: list.length - max };
  }
</script>

<div
  class="terminal-edit my-1 overflow-hidden rounded-lg border border-stone-600 bg-stone-950"
  data-testid="terminal-edit-card"
>
  <div class="flex flex-wrap items-center gap-2 border-b border-stone-700 px-3 py-1.5 text-[11px] text-stone-300">
    <span class="font-semibold tracking-wide text-stone-400 uppercase">{lang === 'plaintext' ? 'file' : lang}</span>
    <span class="min-w-0 truncate font-mono text-stone-200" title={edit.path}>{edit.path}</span>
    <span class="ml-auto font-mono">
      {#if stats.added > 0}<span class="text-emerald-400">+{count(stats.added)}</span>{/if}
      {#if stats.removed > 0}
        <span class="text-red-400">{stats.added > 0 ? ' ' : ''}-{count(stats.removed)}</span>
      {/if}
      {#if stats.added === 0 && stats.removed === 0}<span class="text-stone-500">no change</span>{/if}
    </span>
  </div>
  <div class="overflow-x-auto font-mono text-[11px] leading-5">
    {#each clipped.shown as row, index (index)}
      <div
        class="grid grid-cols-[1.5rem_1fr] {row.tone === 'add'
          ? 'bg-emerald-950/50'
          : row.tone === 'del'
            ? 'bg-red-950/40'
            : ''}"
      >
        <span
          class="select-none text-center {row.tone === 'add'
            ? 'text-emerald-400'
            : row.tone === 'del'
              ? 'text-red-400'
              : 'text-stone-600'}">{row.sign}</span
        >
        <code
          class="hl-line whitespace-pre text-stone-200 [&_.hl-comment]:text-stone-500 [&_.hl-keyword]:text-rose-300 [&_.hl-number]:text-amber-200 [&_.hl-string]:text-emerald-200/90 [&_.hl-type]:text-sky-300"
          >{@html highlightLine(row.text, lang)}</code
        >
      </div>
    {/each}
  </div>
  {#if clipped.hidden > 0}
    <button
      type="button"
      class="w-full border-t border-stone-700 px-3 py-2 text-left text-stone-400 hover:text-stone-200"
      onclick={() => (expanded = true)}
    >
      Show all {count(clipped.hidden + foldLines)} lines
    </button>
  {/if}
</div>
