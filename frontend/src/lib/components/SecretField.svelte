<script lang="ts" module>
  export function omitEmptySecret(name: string, value: string, clear: boolean): Record<string, string | boolean> {
    if (clear) return { clear: true };
    if (value === '') return {};
    return { [name]: value };
  }
</script>

<script lang="ts">
  import { labelClass, widths, type Width } from '$lib/ui/control';
  import FieldError from '$lib/ui/FieldError.svelte';
  import PasswordInput from '$lib/ui/PasswordInput.svelte';

  let {
    label,
    name,
    saved = false,
    value = $bindable(''),
    clear = $bindable(false),
    error = '',
    width = 'md'
  }: {
    label: string;
    name: string;
    saved?: boolean;
    value?: string;
    clear?: boolean;
    error?: string;
    width?: Width;
  } = $props();
  const inputId = $derived(`secret-${name}`);
</script>

<div class="flex w-full flex-col gap-1.5 {widths[width]}">
  <label for={inputId} class={labelClass}>{label}</label>
  <div class="flex items-center gap-2">
    <PasswordInput
      id={inputId}
      {label}
      {name}
      autocomplete="off"
      describedby={error ? `${inputId}-error` : undefined}
      invalid={error !== ''}
      placeholder={clear ? 'Removed when you save' : saved ? 'A value is saved' : ''}
      class="min-w-0 flex-1"
      bind:value
      oninput={() => (clear = false)}
    />
    {#if saved || clear}
      <button
        type="button"
        class="inline-flex min-h-11 shrink-0 items-center rounded-control px-3 text-sm text-muted hover:bg-ground hover:text-ink"
        onclick={() => {
          clear = !clear;
          value = '';
        }}>{clear ? 'Undo' : 'Remove'}</button
      >
    {/if}
  </div>
  <FieldError id={inputId} {error} />
</div>
