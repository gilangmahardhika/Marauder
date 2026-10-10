<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    description = '',
    meta,
    actions,
    children
  }: {
    title: string;
    description?: string;
    meta?: Snippet;
    actions?: Snippet;
    children: Snippet;
  } = $props();
</script>

<div class="w-full min-w-0">
  <div class="mb-8 flex flex-col gap-4 border-b border-line pb-6 sm:flex-row sm:items-end sm:justify-between sm:gap-8">
    <div class="min-w-0 flex-1">
      <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
        <h1 class="break-words text-2xl font-semibold tracking-tight text-ink">{title}</h1>
        {#if meta}{@render meta()}{/if}
      </div>
      {#if description}<p class="mt-1.5 max-w-2xl text-sm text-muted">{description}</p>{/if}
    </div>
    {#if actions}<div class="flex shrink-0 flex-wrap items-center gap-2">{@render actions()}</div>{/if}
  </div>
  {@render children()}
</div>
