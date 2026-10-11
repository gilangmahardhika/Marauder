<script lang="ts">
  import type { Option } from '$lib/filter';

  let {
    label,
    options,
    value,
    onchange
  }: { label: string; options: Option[]; value: string; onchange: (value: string) => void } = $props();
</script>

<div class="inline-flex overflow-hidden rounded-control border border-line bg-surface" role="group" aria-label={label}>
  {#each options as option, index (option.value)}
    <button
      type="button"
      aria-pressed={value === option.value}
      class="min-h-11 px-3 text-sm whitespace-nowrap {index > 0 ? 'border-l border-line' : ''} {value === option.value
        ? 'bg-ground font-medium text-ink'
        : 'text-muted hover:text-ink'}"
      onclick={() => onchange(option.value)}
    >
      {option.label}
      {#if option.count !== undefined}<span class="ml-1 text-xs text-muted">{option.count}</span>{/if}
    </button>
  {/each}
</div>
