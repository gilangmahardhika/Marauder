<script lang="ts">
  import type { Snippet } from 'svelte';
  import { control, labelClass, widths, type Width } from './control';

  let {
    label,
    id,
    value = $bindable(''),
    error = '',
    width = 'md',
    children
  }: {
    label: string;
    id: string;
    value?: string;
    error?: string;
    width?: Width;
    children: Snippet;
  } = $props();
</script>

<div class="flex w-full flex-col gap-1.5 {widths[width]}">
  <label for={id} class={labelClass}>{label}</label>
  <select {id} class={control} bind:value>
    {@render children()}
  </select>
  {#if error}<p class="error text-xs">{error}</p>{/if}
</div>
