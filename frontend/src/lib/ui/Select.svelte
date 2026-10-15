<script lang="ts">
  import type { Snippet } from 'svelte';
  import { control, describedBy, labelClass, widths, type Width } from './control';
  import FieldError from './FieldError.svelte';

  let {
    label,
    id,
    value = $bindable(''),
    hint = '',
    error = '',
    width = 'md',
    children
  }: {
    label: string;
    id: string;
    value?: string;
    hint?: string;
    error?: string;
    width?: Width;
    children: Snippet;
  } = $props();
</script>

<div class="flex w-full flex-col gap-1.5 {widths[width]}">
  <label for={id} class={labelClass}>{label}</label>
  {#if hint}<p id="{id}-hint" class="text-xs text-muted">{hint}</p>{/if}
  <select
    {id}
    class={control}
    aria-invalid={error ? true : undefined}
    aria-describedby={describedBy(id, hint, error)}
    bind:value
  >
    {@render children()}
  </select>
  <FieldError {id} {error} />
</div>
