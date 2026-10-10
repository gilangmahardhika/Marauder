<script lang="ts">
  import type { Snippet } from 'svelte';

  let { tone = 'default', children }: { tone?: 'default' | 'danger'; children: Snippet } = $props();
</script>

<div
  class="rounded-control border px-4 py-3 text-sm {tone === 'danger'
    ? 'border-red-200 bg-red-50 text-red-900'
    : 'border-line bg-sidebar/60 text-muted'}"
>
  {@render children()}
</div>
