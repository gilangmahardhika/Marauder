<script lang="ts">
  import { tick } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { clock, count } from '$lib/format';
  import { clipLines, foldLines, pageEvents, terminalLines, type TerminalTone } from '$lib/terminal';
  import type { TraceAttempt, TraceEvent } from '$lib/types';

  let {
    events,
    attempts = [],
    follow = $bindable(true)
  }: { events: TraceEvent[]; attempts?: TraceAttempt[]; follow?: boolean } = $props();

  let limit = $state(pageEvents);
  let box = $state<HTMLDivElement | null>(null);
  const expanded = new SvelteSet<string>();

  const entries = $derived(terminalLines(events, attempts));
  const start = $derived(Math.max(0, entries.length - limit));
  const visible = $derived(entries.slice(start));

  const tones: Record<TerminalTone, string> = {
    model: 'text-sky-300',
    tool: 'text-ground',
    ok: 'text-emerald-300',
    error: 'text-red-300',
    transition: 'text-amber-300',
    muted: 'text-stone-400'
  };

  /** How far from the bottom still counts as reading the latest output. */
  const nearBottom = 24;

  async function toBottom() {
    await tick();
    if (box) box.scrollTop = box.scrollHeight;
  }

  function onscroll() {
    if (!box) return;
    if (box.scrollHeight - box.scrollTop - box.clientHeight > nearBottom) follow = false;
  }

  function jump() {
    follow = true;
    void toBottom();
  }

  $effect(() => {
    void entries.length;
    if (follow) void toBottom();
  });
</script>

<div class="flex flex-col gap-2">
  <!-- A scrolling log takes focus so the keyboard can scroll it. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div
    bind:this={box}
    {onscroll}
    role="log"
    aria-label="Terminal output, oldest first"
    tabindex="0"
    class="max-h-[75vh] overflow-auto rounded-panel bg-ink px-4 py-3 font-mono text-xs leading-5 text-ground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
  >
    {#if start > 0}
      <button
        type="button"
        class="mb-2 min-h-11 rounded-control border border-stone-600 px-3 text-stone-300 hover:text-ground"
        onclick={() => (limit += pageEvents)}
      >
        Show earlier events ({count(start)} hidden)
      </button>
    {/if}
    {#each visible as entry (entry.id)}
      {@const clipped = expanded.has(entry.id) ? { shown: entry.body, hidden: 0 } : clipLines(entry.body)}
      <div class="grid grid-cols-[auto_1fr] gap-x-3">
        <time class="text-stone-400 select-none" datetime={entry.ts}>{clock(entry.ts)}</time>
        <p class="m-0 break-words whitespace-pre-wrap {tones[entry.tone]}">{entry.head}</p>
        {#if clipped.shown}
          <pre class="col-start-2 m-0 mb-1 font-mono break-words whitespace-pre-wrap text-stone-300">{clipped.shown}</pre>
        {/if}
        {#if clipped.hidden > 0}
          <button
            type="button"
            class="col-start-2 mb-1 min-h-11 justify-self-start rounded-control border border-stone-600 px-3 text-stone-300 hover:text-ground"
            onclick={() => expanded.add(entry.id)}
          >
            Show all {count(clipped.hidden + foldLines)} lines
          </button>
        {/if}
      </div>
    {/each}
  </div>
  {#if !follow}
    <button
      type="button"
      class="min-h-11 self-end rounded-control border border-line bg-surface px-3 text-sm text-ink hover:bg-ground"
      onclick={jump}
    >
      Jump to latest
    </button>
  {/if}
</div>
