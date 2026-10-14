<script lang="ts">
  import Icon from './Icon.svelte';

  let { id, error = '' }: { id: string; error?: string } = $props();
</script>

{#if error}
  <p id="{id}-error" class="flex items-start gap-1.5 text-xs text-danger">
    <Icon name="alert" class="mt-px size-3.5 shrink-0" />
    <span>{error}</span>
  </p>
{/if}
