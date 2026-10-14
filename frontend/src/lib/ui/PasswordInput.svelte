<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements';
  import { control } from './control';
  import Icon from './Icon.svelte';

  let {
    id,
    label,
    name,
    value = $bindable(''),
    autocomplete,
    disabled = false,
    placeholder = '',
    describedby,
    invalid,
    oninput,
    class: className = ''
  }: {
    id: string;
    label: string;
    name?: string;
    value?: string | number;
    autocomplete?: HTMLInputAttributes['autocomplete'];
    disabled?: boolean;
    placeholder?: string;
    describedby?: string;
    invalid?: boolean;
    oninput?: (event: Event) => void;
    class?: string;
  } = $props();

  let shown = $state(false);

  $effect(() => {
    if (value === '' || disabled) shown = false;
  });
</script>

<div class="relative w-full {className}">
  <input
    {id}
    {name}
    type={shown ? 'text' : 'password'}
    {autocomplete}
    {disabled}
    {placeholder}
    aria-describedby={describedby}
    aria-invalid={invalid || undefined}
    class="{control} pr-11"
    bind:value
    {oninput}
  />
  <button
    type="button"
    class="absolute inset-y-0 right-0 inline-flex size-11 items-center justify-center rounded-r-control text-muted hover:text-ink disabled:opacity-50"
    aria-label={`${shown ? 'Hide' : 'Show'} ${label}`}
    aria-pressed={shown}
    aria-controls={id}
    {disabled}
    onclick={() => (shown = !shown)}
  >
    <Icon name={shown ? 'eye-off' : 'eye'} />
  </button>
</div>
