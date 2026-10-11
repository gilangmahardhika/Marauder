<script lang="ts">
  import Icon from './Icon.svelte';
  import SearchSelect, { type SearchOption } from './SearchSelect.svelte';
  import { labelClass, type Width } from './control';

  let {
    label,
    id,
    options,
    values = $bindable([]),
    placeholder = '',
    width = 'md',
    max,
    disabled = false,
    error = '',
    onchange
  }: {
    label: string;
    id: string;
    options: SearchOption[];
    values?: string[];
    placeholder?: string;
    width?: Width;
    max?: number;
    disabled?: boolean;
    error?: string;
    onchange?: (values: string[]) => void;
  } = $props();

  let pick = $state('');

  const chosen = $derived(
    values.map((value) => options.find((option) => option.value === value) ?? { value, label: value })
  );
  const remaining = $derived(options.filter((option) => !values.includes(option.value)));
  const full = $derived(max !== undefined && values.length >= max);

  function add(value: string) {
    pick = '';
    if (value === '' || values.includes(value) || full) return;
    values = [...values, value];
    onchange?.(values);
  }

  function remove(value: string) {
    values = values.filter((item) => item !== value);
    onchange?.(values);
  }
</script>

<div class="flex w-full flex-col gap-2">
  {#if !disabled && !full}
    <SearchSelect {label} {id} {width} {error} {placeholder} options={remaining} bind:value={pick} onchange={add} />
  {:else}
    <p class={labelClass}>{label}</p>
    {#if error}<p class="error text-xs">{error}</p>{/if}
  {/if}
  {#if chosen.length > 0}
    <ul class="flex flex-wrap gap-2" aria-label="{label} chosen">
      {#each chosen as option (option.value)}
        <li
          class="inline-flex min-h-8 items-center gap-1 rounded-control border border-line bg-ground py-0.5 pl-2.5 pr-1 text-sm text-ink"
        >
          <span class="truncate">{option.label}</span>
          {#if !disabled}
            <button
              type="button"
              class="inline-flex size-7 items-center justify-center rounded-control text-muted hover:bg-surface hover:text-ink"
              aria-label="Remove {option.label}"
              onclick={() => remove(option.value)}
            >
              <Icon name="close" class="size-3.5" />
            </button>
          {/if}
        </li>
      {/each}
    </ul>
  {:else if disabled}
    <p class="text-sm text-muted">None</p>
  {/if}
</div>
