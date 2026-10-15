<script lang="ts" module>
  export type Choice = { value: string; label: string; description: string };
</script>

<script lang="ts">
  let {
    label,
    id,
    options,
    value,
    disabled = false,
    columns = 1,
    onchange
  }: {
    label: string;
    id: string;
    options: Choice[];
    value: string;
    disabled?: boolean;
    columns?: 1 | 2;
    onchange: (value: string) => void;
  } = $props();
</script>

<div class="grid gap-2 {columns === 2 ? 'sm:grid-cols-2' : ''}" role="group" aria-label={label}>
  {#each options as option (option.value)}
    {@const selected = value === option.value}
    <button
      type="button"
      aria-pressed={selected}
      aria-label={option.label}
      aria-describedby="{id}-{option.value}-desc"
      {disabled}
      class="flex min-h-11 items-start gap-3 rounded-control border p-3 text-left transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-60 {selected
        ? 'border-accent bg-accent/5 ring-1 ring-accent'
        : 'border-line bg-surface hover:border-line-strong'}"
      onclick={() => onchange(option.value)}
    >
      <span
        class="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border {selected ? 'border-accent' : 'border-line-strong'}"
        aria-hidden="true"
      >
        {#if selected}<span class="size-2 rounded-full bg-accent"></span>{/if}
      </span>
      <span class="flex min-w-0 flex-col gap-0.5">
        <span class="text-sm font-medium text-ink">{option.label}</span>
        <span id="{id}-{option.value}-desc" class="text-xs text-muted">{option.description}</span>
      </span>
    </button>
  {/each}
</div>
