<script lang="ts">
  import FieldError from './FieldError.svelte';

  let {
    id,
    label,
    hint = '',
    error = '',
    checked = $bindable(false)
  }: { id: string; label: string; hint?: string; error?: string; checked?: boolean } = $props();
</script>

<div class="flex w-fit flex-col gap-1">
  <label for={id} class="flex min-h-11 w-fit items-start gap-3 py-1.5">
    <input
      {id}
      type="checkbox"
      class="mt-0.5 size-4 shrink-0 accent-accent"
      aria-invalid={error ? true : undefined}
      aria-describedby={error ? `${id}-error` : undefined}
      bind:checked
    />
    <span>
      <span class="block text-sm font-medium text-ink">{label}</span>
      {#if hint}<span class="mt-0.5 block text-xs text-muted">{hint}</span>{/if}
    </span>
  </label>
  <FieldError {id} {error} />
</div>
