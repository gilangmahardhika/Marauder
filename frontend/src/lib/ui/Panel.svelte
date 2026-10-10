<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title = '',
    hint = '',
    mono = false,
    flush = false,
    actions,
    footer,
    children
  }: {
    title?: string;
    hint?: string;
    mono?: boolean;
    flush?: boolean;
    actions?: Snippet;
    footer?: Snippet;
    children: Snippet;
  } = $props();
</script>

<section class="rounded-panel border border-line bg-surface">
  {#if title || actions}
    <header class="flex items-start justify-between gap-4 border-b border-line px-5 py-3.5">
      <div class="min-w-0">
        {#if title}
          <h2 class="break-words font-semibold text-ink {mono ? 'font-mono text-sm' : 'text-[15px]'}">{title}</h2>
        {/if}
        {#if hint}<p class="mt-0.5 text-[13px] text-muted">{hint}</p>{/if}
      </div>
      {#if actions}<div class="-my-1.5 flex shrink-0 items-center gap-2">{@render actions()}</div>{/if}
    </header>
  {/if}
  <div class={flush ? '' : 'flex flex-col gap-5 p-5'}>
    {@render children()}
  </div>
  {#if footer}
    <footer
      class="flex flex-wrap items-center justify-end gap-2 rounded-b-panel border-t border-line bg-ground/50 px-5 py-3"
    >
      {@render footer()}
    </footer>
  {/if}
</section>
