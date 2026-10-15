<script lang="ts">
  import { ago, stamp } from '$lib/format';
  import { kindLook, notePath } from '$lib/notifications.svelte';
  import type { AppNotification } from '$lib/types';
  import Icon from '$lib/ui/Icon.svelte';

  let {
    note,
    now,
    roomy = false,
    count = 1,
    unread: anyUnread,
    onfollow
  }: {
    note: AppNotification;
    now: number;
    roomy?: boolean;
    /** How many notes of the same card and kind this row stands for. */
    count?: number;
    /** Set when repeats in the row are unread although `note` is read. */
    unread?: boolean;
    onfollow: (event: MouseEvent) => void;
  } = $props();

  const unread = $derived(anyUnread ?? note.readAt === null);
  const look = $derived(kindLook(note.kind));
</script>

<a
  href={notePath(note.url)}
  class="relative flex min-h-11 items-start transition-colors duration-150 hover:bg-ground/60 focus-visible:-outline-offset-2 {roomy
    ? 'gap-4 px-5 py-4'
    : 'gap-3 px-4 py-3'} {unread ? 'bg-accent/5' : ''}"
  onclick={onfollow}
>
  {#if unread}<span class="absolute inset-y-0 left-0 w-0.5 bg-accent" aria-hidden="true"></span>{/if}
  <span class="flex shrink-0 items-center justify-center rounded-full {roomy ? 'size-9' : 'size-8'} {look.chip} {unread ? '' : 'opacity-70'}">
    <Icon name={look.icon} class="size-4" />
  </span>
  <span class="min-w-0 flex-1">
    <span class="flex items-center gap-3">
      <span class="flex min-w-0 flex-1 items-center gap-2">
        <span class="min-w-0 truncate text-sm {unread ? 'font-semibold text-ink' : 'font-medium text-ink/75'}">{note.title}</span>
        {#if count > 1}
          <span class="shrink-0 rounded-full bg-ground px-1.5 text-[11px] font-medium tabular-nums text-muted"
            >{count}x<span class="sr-only"> times</span></span
          >
        {/if}
      </span>
      <span class="flex shrink-0 items-center gap-1.5">
        {#if unread}
          <span class="size-1.5 rounded-full bg-accent" aria-hidden="true"></span>
          <span class="sr-only">Unread</span>
        {/if}
        <time class="text-[11px] tabular-nums text-muted" datetime={note.createdAt} title={stamp(note.createdAt)}
          >{ago(note.createdAt, now)}</time
        >
      </span>
    </span>
    <span class="mt-0.5 block text-[13px] text-muted {roomy ? 'line-clamp-2' : 'truncate'}">{note.body}</span>
  </span>
</a>
