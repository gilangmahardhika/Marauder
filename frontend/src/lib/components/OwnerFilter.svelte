<script lang="ts">
  import type { OwnerChoice } from '$lib/board';
  import { displayName } from '$lib/people';
  import Avatar from '$lib/ui/Avatar.svelte';

  let {
    choices,
    selected,
    onchange,
    max = 6
  }: { choices: OwnerChoice[]; selected: string[]; onchange: (next: string[]) => void; max?: number } = $props();

  let open = $state(false);
  let root = $state<HTMLDivElement | undefined>();
  let trigger = $state<HTMLButtonElement | undefined>();

  const shown = $derived(choices.length > max ? choices.slice(0, max - 1) : choices);
  const hidden = $derived(choices.slice(shown.length));
  const hiddenPicked = $derived(hidden.filter((choice) => selected.includes(choice.id)).length);

  function name(choice: OwnerChoice) {
    return choice.person ? displayName(choice.person) : 'No owner';
  }

  function toggle(id: string) {
    onchange(selected.includes(id) ? selected.filter((value) => value !== id) : [...selected, id]);
  }

  $effect(() => {
    if (!open) return;
    function onKey(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        open = false;
        trigger?.focus();
      }
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

<div class="relative flex items-center" role="group" aria-label="Owner" bind:this={root}>
  {#each shown as choice, index (choice.id)}
    {@const on = selected.includes(choice.id)}
    <button
      type="button"
      class="relative flex size-11 shrink-0 items-center justify-center rounded-full {index > 0 ? '-ml-4' : ''} {on
        ? 'z-10'
        : 'hover:z-10'}"
      aria-pressed={on}
      aria-label={choice.person ? `Cards owned by ${name(choice)}` : 'Cards with no owner'}
      title={`${name(choice)} · ${choice.count} ${choice.count === 1 ? 'card' : 'cards'}`}
      onclick={() => toggle(choice.id)}
    >
      <span class="rounded-full ring-2 {on ? 'ring-accent' : 'ring-surface'}">
        <Avatar person={choice.person} size="lg" />
      </span>
    </button>
  {/each}
  {#if hidden.length > 0}
    <button
      bind:this={trigger}
      type="button"
      class="relative -ml-4 flex size-11 shrink-0 items-center justify-center rounded-full"
      aria-haspopup="true"
      aria-expanded={open}
      aria-label={`${hidden.length} more owners${hiddenPicked ? `, ${hiddenPicked} selected` : ''}`}
      onclick={() => (open = !open)}
    >
      <span
        class="flex size-9 items-center justify-center rounded-full bg-ground text-xs font-medium text-ink ring-2 {hiddenPicked
          ? 'ring-accent'
          : 'ring-surface'}">+{hidden.length}</span
      >
    </button>
    {#if open}
      <div
        class="absolute left-0 top-full z-20 mt-2 max-h-80 min-w-56 overflow-y-auto rounded-panel border border-line bg-surface p-1 shadow-lg"
      >
        {#each hidden as choice (choice.id)}
          <label class="flex min-h-11 cursor-pointer items-center gap-3 rounded-control px-3 text-sm hover:bg-ground">
            <input
              type="checkbox"
              class="size-4 accent-accent"
              checked={selected.includes(choice.id)}
              onchange={() => toggle(choice.id)}
            />
            <Avatar person={choice.person} />
            <span class="min-w-0 flex-1 truncate text-ink">{name(choice)}</span>
            <span class="text-xs text-muted">{choice.count}</span>
          </label>
        {/each}
      </div>
    {/if}
  {/if}
</div>
