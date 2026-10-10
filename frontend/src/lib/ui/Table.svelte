<script lang="ts">
  import type { Snippet } from 'svelte';

  let { label, children }: { label: string; children: Snippet } = $props();
</script>

<div class="relative overflow-x-auto rounded-panel border border-line bg-surface">
  <table class="w-full border-collapse text-left text-sm" aria-label={label}>
    {@render children()}
  </table>
</div>
