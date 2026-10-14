<script lang="ts">
  import { toast, type ToastItem } from '$lib/toast.svelte';
  import Icon from './Icon.svelte';
  import type { IconName } from './icons';

  const polite = $derived(toast.items.filter((item) => item.tone !== 'error'));
  const urgent = $derived(toast.items.filter((item) => item.tone === 'error'));

  const icons: Record<ToastItem['tone'], IconName> = { success: 'check', info: 'info', error: 'alert' };
  const tint: Record<ToastItem['tone'], string> = {
    success: 'text-accent',
    info: 'text-muted',
    error: 'text-danger'
  };

  function leave(event: Event, id: number) {
    const box = event.currentTarget as HTMLElement;
    if (!box.matches(':hover') && !box.contains(document.activeElement)) toast.resume(id);
  }
</script>

<div
  class="pointer-events-none fixed inset-x-0 bottom-0 z-50 flex flex-col gap-2 p-3 sm:left-auto sm:w-[24rem] sm:p-6"
>
  <div role="status" aria-live="polite" class="flex flex-col gap-2">
    {#each polite as item (item.id)}{@render card(item)}{/each}
  </div>
  <div role="alert" class="flex flex-col gap-2">
    {#each urgent as item (item.id)}{@render card(item)}{/each}
  </div>
</div>

{#snippet card(item: ToastItem)}
  <div
    class="toast pointer-events-auto flex items-start gap-3 rounded-panel border bg-surface py-1 pl-4 pr-1 shadow-lg shadow-ink/10 {item.tone ===
    'error'
      ? 'border-red-200'
      : 'border-line'}"
    role="group"
    onmouseenter={() => toast.pause(item.id)}
    onmouseleave={(event) => leave(event, item.id)}
    onfocusin={() => toast.pause(item.id)}
    onfocusout={(event) => leave(event, item.id)}
  >
    <Icon name={icons[item.tone]} class="mt-3.5 size-4 shrink-0 {tint[item.tone]}" />
    <div class="min-w-0 flex-1 py-2.5">
      <p class="wrap-break-word text-sm text-ink">{item.text}</p>
      {#if item.href}
        <a
          href={item.href}
          class="mt-1 inline-flex min-h-11 items-center text-sm font-medium text-accent hover:underline"
          onclick={() => toast.dismiss(item.id)}>{item.label ?? 'Open'}</a
        >
      {/if}
    </div>
    <button
      type="button"
      class="inline-flex size-11 shrink-0 items-center justify-center rounded-control text-muted transition-colors hover:bg-ground hover:text-ink"
      aria-label="Dismiss"
      onclick={() => toast.dismiss(item.id)}
    >
      <Icon name="close" />
    </button>
  </div>
{/snippet}
