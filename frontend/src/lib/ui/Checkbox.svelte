<script lang="ts">
  let {
    id,
    label,
    hint = '',
    checked = $bindable(false)
  }: { id: string; label: string; hint?: string; checked?: boolean } = $props();
</script>

<label for={id} class="flex min-h-11 w-fit items-start gap-3 py-1.5">
  <input {id} type="checkbox" class="mt-0.5 size-4 shrink-0 accent-accent" bind:checked />
  <span>
    <span class="block text-sm font-medium text-ink">{label}</span>
    {#if hint}<span class="mt-0.5 block text-xs text-muted">{hint}</span>{/if}
  </span>
</label>
