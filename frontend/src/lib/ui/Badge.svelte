<script lang="ts">
  import type { Snippet } from 'svelte';

  let { tone = 'default', children }: { tone?: 'default' | 'green' | 'amber' | 'sky' | 'red'; children: Snippet } =
    $props();

  const look = $derived(
    tone === 'green'
      ? 'border-emerald-200 bg-emerald-50 text-emerald-800'
      : tone === 'amber'
        ? 'border-amber-200 bg-amber-50 text-amber-800'
        : tone === 'sky'
          ? 'border-sky-200 bg-sky-50 text-sky-800'
          : tone === 'red'
            ? 'border-red-200 bg-red-50 text-red-800'
            : 'border-line bg-ground text-muted'
  );
</script>

<span class="inline-flex items-center rounded-control border px-2 py-0.5 text-xs {look}">
  {@render children()}
</span>
