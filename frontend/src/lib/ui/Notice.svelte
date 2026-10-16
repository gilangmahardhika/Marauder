<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';

  let {
    tone = 'default',
    ondismiss,
    children
  }: {
    tone?: 'default' | 'warning' | 'danger';
    ondismiss?: () => void;
    children: Snippet;
  } = $props();

  const look = {
    default: 'border-line bg-sidebar/60 text-muted',
    warning: 'border-amber-200 bg-amber-50 text-amber-900',
    danger: 'border-red-200 bg-red-50 text-red-900'
  };
</script>

{#if ondismiss}
  <div class="flex items-start gap-1 rounded-control border py-1 pl-4 pr-1 text-sm {look[tone]}">
    <div class="min-w-0 flex-1 py-2">{@render children()}</div>
    <button
      type="button"
      class="inline-flex size-11 shrink-0 items-center justify-center rounded-control text-muted transition-colors hover:bg-ground hover:text-ink"
      aria-label="Dismiss"
      onclick={ondismiss}
    >
      <Icon name="close" />
    </button>
  </div>
{:else}
  <div class="rounded-control border px-4 py-3 text-sm {look[tone]}">
    {@render children()}
  </div>
{/if}
