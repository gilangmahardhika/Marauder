<script lang="ts">
  import type { Snippet } from 'svelte';

  let { tone = 'default', children }: { tone?: 'default' | 'warning' | 'danger'; children: Snippet } = $props();

  const look = {
    default: 'border-line bg-sidebar/60 text-muted',
    warning: 'border-amber-200 bg-amber-50 text-amber-900',
    danger: 'border-red-200 bg-red-50 text-red-900'
  };
</script>

<div class="rounded-control border px-4 py-3 text-sm {look[tone]}">
  {@render children()}
</div>
