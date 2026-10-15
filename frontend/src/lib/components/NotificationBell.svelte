<script lang="ts">
  import { follow, inbox, loadInbox, markAllRead, notePath, receive } from '$lib/notifications.svelte';
  import { live } from '$lib/stream.svelte';
  import { toast } from '$lib/toast.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import NotificationRow from './NotificationRow.svelte';

  /** The panel lists this many; the rest are on the notifications page. */
  const shown = 8;

  let open = $state(false);
  let now = $state(Date.now());
  let root = $state<HTMLDivElement | undefined>();
  let trigger = $state<HTMLButtonElement | undefined>();

  const label = $derived(inbox.unread > 0 ? `Notifications, ${inbox.unread} unread` : 'Notifications');
  const rows = $derived(inbox.items.slice(0, shown));

  $effect(() => {
    void loadInbox();
  });

  $effect(() =>
    live.on((event) => {
      if (event.kind === 'ready') inbox.unread = event.unread;
      else if (event.kind === 'resync') void loadInbox();
      else if (event.kind === 'notification') {
        const note = event.notification;
        receive(note, event.unread);
        toast.info(`${note.title}: ${note.body}`, { href: notePath(note.url), label: 'Open' });
      }
    })
  );

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

<div class="relative" bind:this={root}>
  <button
    bind:this={trigger}
    type="button"
    class="relative inline-flex size-11 items-center justify-center rounded-control text-muted hover:bg-surface hover:text-ink"
    aria-label={label}
    aria-expanded={open}
    onclick={() => {
      open = !open;
      if (open) {
        now = Date.now();
        void loadInbox();
      }
    }}
  >
    <Icon name="bell" class="size-5" />
    {#if inbox.unread > 0}
      <span
        class="absolute right-1.5 top-1.5 min-w-4 rounded-full bg-accent px-1 text-center text-[10px] font-semibold leading-4 text-surface"
        aria-hidden="true">{inbox.unread > 99 ? '99+' : inbox.unread}</span
      >
    {/if}
  </button>
  {#if open}
    <div
      role="group"
      aria-label="Notifications"
      class="absolute right-0 top-full z-30 mt-2 flex w-96 max-w-[calc(100vw-1rem)] flex-col divide-y divide-line overflow-hidden rounded-panel border border-line bg-surface shadow-lg"
    >
      <div class="flex items-center justify-between gap-2 py-1 pl-4 pr-2">
        <div class="flex items-center gap-2">
          <h2 class="text-sm font-semibold text-ink">Notifications</h2>
          {#if inbox.unread > 0}
            <span class="rounded-full bg-accent/10 px-2 py-0.5 text-[11px] font-medium text-accent">{inbox.unread} new</span>
          {/if}
        </div>
        <button
          type="button"
          class="inline-flex min-h-11 items-center gap-1.5 rounded-control px-2 text-xs font-medium text-accent hover:bg-ground disabled:text-muted disabled:hover:bg-transparent"
          disabled={inbox.unread === 0}
          onclick={() => void markAllRead()}
        >
          <Icon name="check" class="size-3.5" />
          Mark all read
        </button>
      </div>
      {#if inbox.failed}
        <p class="px-4 py-6 text-sm text-muted">Notifications could not be loaded. Open the bell again to retry.</p>
      {:else if rows.length === 0}
        <div class="flex flex-col items-center gap-2 px-6 py-8 text-center">
          <span class="flex size-10 items-center justify-center rounded-full bg-ground text-muted">
            <Icon name="bell-off" class="size-5" />
          </span>
          <p class="text-sm text-muted">Nothing yet. You are told here when a card needs you.</p>
        </div>
      {:else}
        <ul class="max-h-[70vh] divide-y divide-line overflow-auto" aria-label="Latest notifications">
          {#each rows as item (item.id)}
            <li>
              <NotificationRow
                note={item}
                {now}
                onfollow={(event) => {
                  open = false;
                  void follow(event, item);
                }}
              />
            </li>
          {/each}
        </ul>
      {/if}
      <a
        href="/notifications"
        class="flex min-h-11 items-center justify-center gap-1.5 bg-ground/40 text-sm font-medium text-accent transition-colors duration-150 hover:bg-ground focus-visible:-outline-offset-2"
        onclick={() => (open = false)}
      >
        Open all notifications
        <Icon name="arrow-left" class="size-4 rotate-180" />
      </a>
    </div>
  {/if}
</div>
