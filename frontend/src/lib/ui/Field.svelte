<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLInputAttributes } from 'svelte/elements';
  import { autogrow } from './autogrow';
  import { control, controlBase, describedBy, labelClass, widths, type Width } from './control';
  import FieldError from './FieldError.svelte';
  import PasswordInput from './PasswordInput.svelte';

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
    placeholder = '',
    rows,
    grow = false,
    labelAction
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
    rows?: number;
    grow?: boolean;
    /** A control at the right of the label row, such as an icon button. */
    labelAction?: Snippet;
  } = $props();

  const font = $derived(mono ? 'font-mono text-[13px]' : '');
  const described = $derived(describedBy(id, hint, error));
  const invalid = $derived(error ? true : undefined);
</script>

<div class="flex w-full flex-col gap-1.5 {widths[width]}">
  {#if labelAction}
    <div class="flex items-center justify-between gap-2">
      <label for={id} class={labelClass}>{label}</label>
      {@render labelAction()}
    </div>
  {:else}
    <label for={id} class={labelClass}>{label}</label>
  {/if}
  {#if hint}<p id="{id}-hint" class="text-xs text-muted">{hint}</p>{/if}
  {#if type === 'textarea'}
    <textarea
      {id}
      {disabled}
      {placeholder}
      {rows}
      aria-describedby={described}
      aria-invalid={invalid}
      class="{controlBase} py-2.5 {font} {grow ? 'resize-none overflow-hidden' : ''}"
      use:autogrow={{ on: grow, value }}
      bind:value
    ></textarea>
  {:else if type === 'password'}
    <PasswordInput {id} {label} {autocomplete} {disabled} {placeholder} describedby={described} {invalid} bind:value />
  {:else}
    <input
      {id}
      {type}
      {autocomplete}
      {disabled}
      {placeholder}
      aria-describedby={described}
      aria-invalid={invalid}
      class="{control} {font}"
      bind:value
    />
  {/if}
  <FieldError {id} {error} />
</div>
