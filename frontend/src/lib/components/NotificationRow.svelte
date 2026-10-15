<script lang="ts">
  import { ago, stamp } from '$lib/format';
  import { kindLook, notePath } from '$lib/notifications.svelte';
  import type { AppNotification } from '$lib/types';
  import Icon from '$lib/ui/Icon.svelte';

  let {
    note,
    now,
    roomy = false,
    onfollow
  }: { note: AppNotification; now: number; roomy?: boolean; onfollow: (event: MouseEvent) => void } = $props();

  const unread = $derived(note.readAt === null);
  const look = $derived(kindLook(note.kind));
</script>

<a
  href={notePath(note.url)}
  class="flex min-h-11 items-start gap-3 transition-colors duration-150 hover:bg-ground focus-visible:-outline-offset-2 {roomy
    ? 'px-5 py-3.5'
    : 'px-4 py-3'} {unread ? 'bg-accent/5' : ''}"
  onclick={onfollow}
>
  <span class="flex size-8 shrink-0 items-center justify-center rounded-full {look.chip}">
    <Icon name={look.icon} class="size-4" />
  </span>
  <span class="min-w-0 flex-1">
    <span class="block truncate text-sm {unread ? 'font-semibold text-ink' : 'font-medium text-ink/80'}">{note.title}</span>
    <span class="mt-0.5 block truncate text-xs text-muted">{note.body}</span>
  </span>
  <span class="flex shrink-0 flex-col items-end gap-2 pt-0.5">
    <time class="text-[11px] tabular-nums text-muted" datetime={note.createdAt} title={stamp(note.createdAt)}
      >{ago(note.createdAt, now)}</time
    >
    {#if unread}
      <span class="size-2 rounded-full bg-accent" aria-hidden="true"></span>
      <span class="sr-only">Unread</span>
    {/if}
  </span>
</a>
