<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements';
  import { control, controlBase, labelClass, widths, type Width } from './control';

  let {
    label,
    id,
    hint = '',
    error = '',
    type = 'text',
    value = $bindable(''),
    autocomplete,
    disabled = false,
    width = 'md',
    mono = false,
    placeholder = ''
  }: {
    label: string;
    id: string;
    hint?: string;
    error?: string;
    type?: string;
    value?: string | number;
    autocomplete?: HTMLInputAttributes['autocomplete'];
    disabled?: boolean;
    width?: Width;
    mono?: boolean;
    placeholder?: string;
  } = $props();

  const font = $derived(mono ? 'font-mono text-[13px]' : '');
  const described = $derived(hint ? `${id}-hint` : undefined);
</script>

<div class="flex w-full flex-col gap-1.5 {widths[width]}">
  <label for={id} class={labelClass}>{label}</label>
  {#if type === 'textarea'}
    <textarea {id} {disabled} {placeholder} aria-describedby={described} class="{controlBase} py-2.5 {font}" bind:value
    ></textarea>
  {:else}
    <input
      {id}
      {type}
      {autocomplete}
      {disabled}
      {placeholder}
      aria-describedby={described}
      class="{control} {font}"
      bind:value
    />
  {/if}
  {#if hint}<p id={described} class="text-xs text-muted">{hint}</p>{/if}
  {#if error}<p class="error text-xs">{error}</p>{/if}
</div>
