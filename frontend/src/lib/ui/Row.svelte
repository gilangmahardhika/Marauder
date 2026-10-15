<script lang="ts">
  import Icon from './Icon.svelte';

  let {
    id,
    label,
    value = $bindable(''),
    placeholder = '',
    onremove
  }: {
    id: string;
    label: string;
    value?: string;
    placeholder?: string;
    onremove: () => void;
  } = $props();
</script>

<div class="flex items-center border-b border-line last:border-b-0">
  <label for={id} class="sr-only">{label}</label>
  <input
    {id}
    {placeholder}
    class="h-11 min-w-0 flex-1 bg-transparent px-3 font-mono text-[13px] text-ink placeholder:text-muted/60 focus-visible:shadow-[inset_0_-1px_0_var(--color-muted)] focus-visible:outline-hidden"
    bind:value
  />
  <button
    type="button"
    class="inline-flex size-11 shrink-0 items-center justify-center text-muted hover:bg-ground hover:text-red-800 focus-visible:-outline-offset-2"
    aria-label="Remove {value || label}"
    onclick={onremove}
  >
    <Icon name="close" />
  </button>
</div>
