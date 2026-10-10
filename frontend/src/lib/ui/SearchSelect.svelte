<script lang="ts" module>
  export type SearchOption = { value: string; label: string; detail?: string };
</script>

<script lang="ts">
  import Icon from './Icon.svelte';
  import { control, labelClass, widths, type Width } from './control';

  let {
    label,
    id,
    options,
    value = $bindable(''),
    placeholder = '',
    width = 'md',
    hideLabel = false,
    emptyLabel,
    error = '',
    onchange
  }: {
    label: string;
    id: string;
    options: SearchOption[];
    value?: string;
    placeholder?: string;
    width?: Width;
    hideLabel?: boolean | 'sm';
    emptyLabel?: string;
    error?: string;
    onchange?: (value: string) => void;
  } = $props();

  let open = $state(false);
  let typing = $state(false);
  let query = $state('');
  let active = $state(0);
  let root = $state<HTMLDivElement | undefined>();

  const listId = $derived(`${id}-list`);
  const display = $derived(options.find((option) => option.value === value)?.label ?? value);

  const choices = $derived.by(() => {
    const needle = typing ? query.trim().toLowerCase() : '';
    const found =
      needle === ''
        ? options
        : options.filter(
            (option) =>
              option.label.toLowerCase().includes(needle) || (option.detail ?? '').toLowerCase().includes(needle)
          );
    const shown = found.slice(0, 50);
    return emptyLabel !== undefined && needle === '' ? [{ value: '', label: emptyLabel }, ...shown] : shown;
  });

  const labelLook = $derived(
    hideLabel === true ? 'sr-only' : hideLabel === 'sm' ? `${labelClass} sm:sr-only` : labelClass
  );

  function show() {
    typing = false;
    query = '';
    open = true;
    active = Math.max(0, choices.findIndex((option) => option.value === value));
  }

  function close() {
    open = false;
    typing = false;
    query = '';
  }

  function pick(option: SearchOption) {
    const changed = option.value !== value;
    value = option.value;
    close();
    if (changed) onchange?.(option.value);
  }

  function onkeydown(event: KeyboardEvent) {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      if (!open) show();
      else active = Math.min(active + 1, choices.length - 1);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      if (!open) show();
      else active = Math.max(active - 1, 0);
    } else if (event.key === 'Enter' && open) {
      event.preventDefault();
      const option = choices[active];
      if (option) pick(option);
    } else if (event.key === 'Escape' && open) {
      event.preventDefault();
      event.stopPropagation();
      close();
    } else if (event.key === 'Tab') {
      close();
    }
  }

  $effect(() => {
    if (!open) return;
    document.getElementById(`${id}-opt-${active}`)?.scrollIntoView?.({ block: 'nearest' });
  });
</script>

<div
  class="flex w-full flex-col gap-1.5 {widths[width]}"
  bind:this={root}
  onfocusout={(event) => {
    if (!root?.contains(event.relatedTarget as Node | null)) close();
  }}
>
  <label for={id} class={labelLook}>{label}</label>
  <div class="relative">
    <input
      {id}
      type="text"
      role="combobox"
      autocomplete="off"
      aria-expanded={open}
      aria-controls={listId}
      aria-autocomplete="list"
      aria-activedescendant={open && choices.length > 0 ? `${id}-opt-${active}` : undefined}
      {placeholder}
      class="{control} pr-9"
      value={typing ? query : display}
      onclick={() => {
        if (!open) show();
      }}
      oninput={(event) => {
        query = event.currentTarget.value;
        typing = true;
        open = true;
        active = 0;
      }}
      {onkeydown}
    />
    <Icon name="chevron" class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-muted" />
    {#if open}
      <ul
        id={listId}
        role="listbox"
        aria-label={label}
        class="absolute top-full z-30 mt-1 max-h-64 w-full overflow-auto rounded-panel border border-line bg-surface p-1 shadow-lg"
      >
        {#each choices as option, index}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <li
            id={`${id}-opt-${index}`}
            role="option"
            tabindex="-1"
            aria-selected={option.value === value}
            class="flex min-h-11 cursor-pointer items-center gap-2 rounded-control px-3 py-1.5 {index === active
              ? 'bg-ground'
              : ''}"
            onpointerdown={(event) => event.preventDefault()}
            onpointerenter={() => (active = index)}
            onclick={() => pick(option)}
          >
            <span class="min-w-0 flex-1">
              <span class="block truncate text-sm {option.value === '' ? 'text-muted' : 'text-ink'}">{option.label}</span>
              {#if option.detail}<span class="block truncate text-xs text-muted">{option.detail}</span>{/if}
            </span>
            {#if option.value === value}<Icon name="check" class="size-4 shrink-0 text-accent" />{/if}
          </li>
        {:else}
          <li class="px-3 py-2 text-sm text-muted">No matches</li>
        {/each}
      </ul>
    {/if}
  </div>
  {#if error}<p class="error text-xs">{error}</p>{/if}
</div>
