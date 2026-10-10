<script lang="ts">
  import Icon from './Icon.svelte';

  type Item = {
    label: string;
    href?: string;
    onclick?: () => void;
  };

  let {
    label,
    detail = '',
    initials = '',
    placement = 'down',
    block = true,
    items
  }: {
    label: string;
    detail?: string;
    initials?: string;
    placement?: 'down' | 'up';
    block?: boolean;
    items: Item[];
  } = $props();

  let open = $state(false);
  let root = $state<HTMLDivElement | undefined>();
  let trigger = $state<HTMLButtonElement | undefined>();

  function close(focusTrigger: boolean) {
    open = false;
    if (focusTrigger) trigger?.focus();
  }

  function choose(item: Item) {
    close(false);
    item.onclick?.();
  }

  $effect(() => {
    if (!open) return;
    function onKey(event: KeyboardEvent) {
      if (event.key === 'Escape') close(true);
    }
    function onPointer(event: PointerEvent) {
      if (root && !root.contains(event.target as Node)) open = false;
    }
    window.addEventListener('keydown', onKey);
    window.addEventListener('pointerdown', onPointer);
    return () => {
      window.removeEventListener('keydown', onKey);
      window.removeEventListener('pointerdown', onPointer);
    };
  });
</script>

<div class="relative" bind:this={root}>
  <button
    bind:this={trigger}
    type="button"
    class="flex min-h-11 items-center gap-3 rounded-control px-2 text-left {block ? 'w-full hover:bg-surface' : 'w-fit border border-line bg-surface px-3 hover:border-line-strong hover:bg-ground/60'}"
    aria-expanded={open}
    aria-haspopup="menu"
    onclick={() => (open = !open)}
  >
    {#if initials}
      <span class="flex size-8 shrink-0 items-center justify-center rounded-full bg-accent text-xs font-medium text-surface">
        {initials}
      </span>
    {/if}
    <span class="min-w-0 flex-1">
      <span class="block truncate text-sm font-medium text-ink">{label}</span>
      {#if detail}<span class="block truncate text-xs text-muted">{detail}</span>{/if}
    </span>
    <Icon name="chevron" class="size-4 shrink-0 text-muted" />
  </button>
  {#if open}
    <div
      role="group"
      aria-label={label}
      class="absolute z-20 min-w-48 rounded-panel border border-line bg-surface p-1 shadow-lg {placement === 'up'
        ? 'bottom-full mb-2'
        : 'top-full mt-2'}"
    >
      {#each items as item (item.label)}
        {#if item.href}
          <a
            href={item.href}
            class="flex min-h-11 items-center rounded-control px-3 text-sm hover:bg-ground"
            onclick={() => (open = false)}>{item.label}</a
          >
        {:else}
          <button
            type="button"
            class="flex min-h-11 w-full items-center rounded-control px-3 text-left text-sm hover:bg-ground"
            onclick={() => choose(item)}>{item.label}</button
          >
        {/if}
      {/each}
    </div>
  {/if}
</div>
