<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    description = '',
    href = '',
    link = '',
    children
  }: { title: string; description?: string; href?: string; link?: string; children?: Snippet } = $props();
</script>

<div class="rounded-panel border border-dashed border-line-strong bg-surface/60 px-6 py-10">
  <p class="text-sm font-medium text-ink">{title}</p>
  {#if description}<p class="mt-1 max-w-md text-sm text-muted">{description}</p>{/if}
  {#if href && link}
    <a class="mt-3 inline-flex min-h-11 items-center text-sm font-medium text-accent underline" {href}>{link}</a>
  {/if}
  {#if children}<div class="mt-4 flex flex-wrap gap-2">{@render children()}</div>{/if}
</div>
