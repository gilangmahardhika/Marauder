<script lang="ts" module>
  export type ChipOption = {
    value: string;
    label: string;
    detail?: string;
    /** A short muted pill after the label, such as "Implement". */
    tag?: string;
    /** Options with the same group sit under one header, in the order they first appear. */
    group?: string;
  };
</script>

<script lang="ts">
  import Icon from './Icon.svelte';
  import type { IconName } from './icons';

  let {
    label,
    id,
    value = $bindable(''),
    display,
    prefix = '',
    icon,
    disabled = false,
    options,
    onchange
  }: {
    /** The accessible name of the combobox and its list. */
    label: string;
    id: string;
    value?: string;
    /** The short text the chip shows for the selected option. */
    display: string;
    /** A muted word before the value, such as "Effort". */
    prefix?: string;
    icon?: IconName;
    disabled?: boolean;
    options: ChipOption[];
    onchange?: (value: string) => void;
  } = $props();

  let open = $state(false);
  let active = $state(0);
  let root = $state<HTMLDivElement | undefined>();
  const listId = $derived(`${id}-list`);

  const groups = $derived(
    options.reduce<{ name: string; items: { option: ChipOption; index: number }[] }[]>((all, option, index) => {
      const name = option.group ?? '';
      const found = all.find((group) => group.name === name);
      if (found) found.items.push({ option, index });
      else all.push({ name, items: [{ option, index }] });
      return all;
    }, [])
  );

  function show() {
    if (disabled || options.length === 0) return;
    active = Math.max(0, options.findIndex((option) => option.value === value));
    open = true;
  }

  function pick(index: number) {
    const option = options[index];
    open = false;
    if (!option || option.value === value) return;
    value = option.value;
    onchange?.(option.value);
  }

  function seek(key: string) {
    const letter = key.toLowerCase();
    for (let step = 1; step <= options.length; step++) {
      const index = (active + step) % options.length;
      if (options[index].label.toLowerCase().startsWith(letter)) {
        active = index;
        return;
      }
    }
  }

  function onkeydown(event: KeyboardEvent) {
    if (disabled) return;
    if (!open) {
      if (['Enter', ' ', 'ArrowDown', 'ArrowUp'].includes(event.key)) {
        event.preventDefault();
        show();
      }
      return;
    }
    if (event.key === 'ArrowDown') active = Math.min(active + 1, options.length - 1);
    else if (event.key === 'ArrowUp') active = Math.max(active - 1, 0);
    else if (event.key === 'Home') active = 0;
    else if (event.key === 'End') active = options.length - 1;
    else if (event.key === 'Enter' || event.key === ' ') pick(active);
    else if (event.key === 'Escape') {
      event.stopPropagation();
      open = false;
    } else if (event.key === 'Tab') {
      open = false;
      return;
    } else if (event.key.length === 1 && !event.metaKey && !event.ctrlKey && !event.altKey) seek(event.key);
    else return;
    event.preventDefault();
  }

  $effect(() => {
    if (!open) return;
    document.getElementById(`${id}-opt-${active}`)?.scrollIntoView?.({ block: 'nearest' });
  });

  $effect(() => {
    if (!open) return;
    function onPointer(event: PointerEvent) {
      if (root && !root.contains(event.target as Node)) open = false;
    }
    window.addEventListener('pointerdown', onPointer);
    return () => window.removeEventListener('pointerdown', onPointer);
  });
</script>

<div
  class="relative inline-flex max-w-full"
  bind:this={root}
  onfocusout={(event) => {
    if (!root?.contains(event.relatedTarget as Node | null)) open = false;
  }}
>
  <button
    {id}
    type="button"
    role="combobox"
    {disabled}
    aria-label={label}
    aria-haspopup="listbox"
    aria-expanded={open}
    aria-controls={listId}
    aria-activedescendant={open ? `${id}-opt-${active}` : undefined}
    class="group inline-flex min-h-11 max-w-full cursor-pointer items-center focus-visible:outline-hidden disabled:cursor-not-allowed"
    onclick={() => (open ? (open = false) : show())}
    {onkeydown}
  >
    <span
      class="inline-flex h-7 max-w-full items-center gap-1 rounded-full px-2.5 text-xs text-muted transition-colors duration-150 group-hover:bg-ground group-hover:text-ink group-focus-visible:bg-ground group-focus-visible:text-ink group-disabled:opacity-50 group-aria-expanded:bg-ground group-aria-expanded:text-ink"
    >
      {#if icon}
        <Icon name={icon} class="size-3.5 shrink-0" />
      {/if}
      {#if prefix}
        <span>{prefix}</span>
      {/if}
      <span class="max-w-40 truncate font-medium text-ink">{display}</span>
      <Icon name="chevron" class="size-3 shrink-0 group-aria-expanded:rotate-180 motion-safe:transition-transform" />
    </span>
  </button>
  {#if open}
    <ul
      id={listId}
      role="listbox"
      aria-label={label}
      class="chip-menu absolute top-full left-0 z-30 -mt-1 max-h-72 min-w-52 overflow-auto rounded-xl border border-line bg-surface p-1 shadow-lg"
    >
      {#each groups as group (group.name)}
        {#if group.name}
          <li role="presentation" aria-hidden="true" class="px-2.5 pt-2 pb-1 text-[11px] font-medium tracking-wide text-muted uppercase">
            {group.name}
          </li>
        {/if}
        <li role="presentation">
          <ul role="group" aria-label={group.name || label}>
            {#each group.items as { option, index } (option.value)}
              <!-- svelte-ignore a11y_click_events_have_key_events -->
              <li
                id={`${id}-opt-${index}`}
                role="option"
                aria-selected={option.value === value}
                class="flex min-h-11 cursor-pointer items-center gap-2 rounded-lg px-2.5 text-sm text-ink {index === active
                  ? 'bg-ground'
                  : ''}"
                onpointerdown={(event) => event.preventDefault()}
                onpointerenter={() => (active = index)}
                onclick={() => pick(index)}
              >
                <span class="min-w-0 flex-1">
                  <span class="block truncate">{option.label}</span>
                  {#if option.detail}<span class="block truncate text-xs text-muted">{option.detail}</span>{/if}
                </span>
                {#if option.tag}
                  <span class="shrink-0 rounded-full bg-ground px-2 py-0.5 text-[11px] font-medium text-muted">{option.tag}</span>
                {/if}
                <Icon
                  name="check"
                  class="size-4 shrink-0 text-accent {option.value === value ? '' : 'invisible'}"
                />
              </li>
            {/each}
          </ul>
        </li>
      {/each}
    </ul>
  {/if}
</div>
