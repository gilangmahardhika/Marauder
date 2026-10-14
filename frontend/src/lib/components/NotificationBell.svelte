<script lang="ts">
  import { getNotifications, readNotifications } from '$lib/api';
  import { stamp } from '$lib/format';
  import { session } from '$lib/session';
  import { live } from '$lib/stream.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { AppNotification } from '$lib/types';
  import Icon from '$lib/ui/Icon.svelte';

  let items = $state<AppNotification[]>([]);
  let unread = $state(0);
  let failed = $state(false);
  let open = $state(false);
  let root = $state<HTMLDivElement | undefined>();
  let trigger = $state<HTMLButtonElement | undefined>();

  const label = $derived(unread > 0 ? `Notifications, ${unread} unread` : 'Notifications');

  /** Only a path on this site is followed. */
  function target(url: string) {
    return url.startsWith('/') && !url.startsWith('//') ? url : '/';
  }

  async function load() {
    try {
      const body = await getNotifications();
      items = body.notifications;
      unread = body.unread;
      failed = false;
    } catch {
      failed = true;
    }
  }

  $effect(() => {
    void load();
  });

  $effect(() =>
    live.on((event) => {
      if (event.kind === 'ready') unread = event.unread;
      else if (event.kind === 'resync') void load();
      else if (event.kind === 'notification') {
        const note = event.notification;
        items = [note, ...items.filter((item) => item.id !== note.id)].slice(0, 50);
        unread = event.unread;
        toast.info(`${note.title}: ${note.body}`, { href: target(note.url), label: 'Open' });
      }
    })
  );

  async function markRead(note: AppNotification) {
    open = false;
    if (note.readAt !== null) return;
    note.readAt = new Date().toISOString();
    unread = Math.max(0, unread - 1);
    try {
      unread = (await readNotifications({ ids: [note.id] })).unread;
    } catch {
      /* The next read of the list corrects the count. */
    }
  }

  /** A card in another project loads the page fresh, so the whole shell switches to that project. */
  async function follow(event: MouseEvent, note: AppNotification) {
    if (note.projectId === session.project()) {
      void markRead(note);
      return;
    }
    event.preventDefault();
    await markRead(note);
    window.location.assign(target(note.url));
  }

  async function markAll() {
    const now = new Date().toISOString();
    for (const item of items) item.readAt ??= now;
    unread = 0;
    try {
      unread = (await readNotifications({ all: true })).unread;
    } catch {
      void load();
    }
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

<div class="relative" bind:this={root}>
  <button
    bind:this={trigger}
    type="button"
    class="relative inline-flex size-11 items-center justify-center rounded-control text-muted hover:bg-surface hover:text-ink"
    aria-label={label}
    aria-expanded={open}
    onclick={() => {
      open = !open;
      if (open) void load();
    }}
  >
    <Icon name="bell" class="size-5" />
    {#if unread > 0}
      <span
        class="absolute right-1.5 top-1.5 min-w-4 rounded-full bg-accent px-1 text-center text-[10px] font-semibold leading-4 text-surface"
        aria-hidden="true">{unread > 99 ? '99+' : unread}</span
      >
    {/if}
  </button>
  {#if open}
    <div
      role="group"
      aria-label="Notifications"
      class="absolute right-0 top-full z-30 mt-2 w-80 max-w-[calc(100vw-1rem)] rounded-panel border border-line bg-surface p-1 shadow-lg"
    >
      <div class="flex items-center justify-between gap-2 px-3 py-1">
        <h2 class="text-sm font-semibold text-ink">Notifications</h2>
        <button
          type="button"
          class="min-h-11 px-1 text-xs font-medium text-accent hover:underline disabled:text-muted disabled:no-underline"
          disabled={unread === 0}
          onclick={() => void markAll()}>Mark all read</button
        >
      </div>
      {#if failed}
        <p class="px-3 py-4 text-sm text-muted">Notifications could not be loaded. Open the bell again to retry.</p>
      {:else if items.length === 0}
        <p class="px-3 py-4 text-sm text-muted">Nothing yet. You are told here when a card needs you.</p>
      {:else}
        <ul class="max-h-[60vh] overflow-auto">
          {#each items as item (item.id)}
            <li>
              <a
                href={target(item.url)}
                class="flex min-h-11 items-start gap-2 rounded-control px-3 py-2 hover:bg-ground"
                onclick={(event) => void follow(event, item)}
              >
                <span
                  class="mt-1.5 size-2 shrink-0 rounded-full {item.readAt === null ? 'bg-accent' : 'bg-transparent'}"
                  aria-hidden="true"
                ></span>
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-sm text-ink {item.readAt === null ? 'font-semibold' : ''}">{item.title}</span>
                  <span class="block truncate text-xs text-muted">{item.body}</span>
                  <span class="block text-[11px] text-muted">{stamp(item.createdAt)}{item.readAt === null ? ' · unread' : ''}</span>
                </span>
              </a>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}
</div>
