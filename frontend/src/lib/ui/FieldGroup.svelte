<script lang="ts">
  import type { Snippet } from 'svelte';
  import { labelClass } from './control';

  let { label, hint = '', children }: { label: string; hint?: string; children: Snippet } = $props();
</script>

<fieldset class="min-w-0">
  <legend class="{labelClass} mb-1.5">{label}</legend>
  {#if hint}<p class="mb-2 text-xs text-muted">{hint}</p>{/if}
  <div class="flex flex-col gap-2">
    {@render children()}
  </div>
</fieldset>
