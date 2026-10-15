<script lang="ts">
  import NotificationRow from '$lib/components/NotificationRow.svelte';
  import type { Option } from '$lib/filter';
  import { failureText } from '$lib/forms';
  import { dayGroups, fetchInbox, follow, inbox, markAllRead } from '$lib/notifications.svelte';
  import { showShell } from '$lib/shell.svelte';
  import type { AppNotification } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';

  type Show = 'all' | 'unread';

  let show = $state<Show>('all');
  /** Unread rows stay listed after they are read, until the filter changes. */
  let unreadRows = $state<AppNotification[]>([]);
  let loading = $state('Loading notifications');
  let error = $state('');
  let marking = $state(false);
  let now = $state(Date.now());

  showShell(() => ({ title: 'Notifications' }));

  const rows = $derived(show === 'all' ? inbox.items : unreadRows);
  const groups = $derived(dayGroups(rows, now));
  const options = $derived<Option<Show>[]>([
    { value: 'all', label: 'All' },
    { value: 'unread', label: 'Unread', count: inbox.unread }
  ]);

  async function load(which: Show) {
    loading = 'Loading notifications';
    error = '';
    try {
      const list = await fetchInbox(which === 'unread');
      if (which === 'unread') unreadRows = list;
      now = Date.now();
    } catch (failure) {
      error = failureText(failure);
    } finally {
      loading = '';
    }
  }

  function choose(value: string) {
    show = value === 'unread' ? 'unread' : 'all';
    void load(show);
  }

  async function readAll() {
    marking = true;
    try {
      await markAllRead([unreadRows]);
    } finally {
      marking = false;
    }
  }

  $effect(() => {
    void load('all');
  });

  $effect(() => {
    const timer = setInterval(() => (now = Date.now()), 60_000);
    return () => clearInterval(timer);
  });
</script>

<Page
  title="Notifications"
  description="The newest 50 notifications in the projects you can see. Open one to go to its card."
  {loading}
  {error}
  onretry={() => void load(show)}
>
  {#snippet actions()}
    <Segmented label="Show" {options} value={show} onchange={choose} />
    <Button icon="check" loading={marking} disabled={inbox.unread === 0} onclick={() => void readAll()}>Mark all read</Button>
  {/snippet}
  {#if rows.length === 0}
    {#if show === 'all'}
      <EmptyState title="No notifications yet" description="You are told here when a card needs you." />
    {:else}
      <EmptyState title="You're all caught up" description="Every notification is read." />
    {/if}
  {:else}
    <div class="flex max-w-3xl flex-col gap-6">
      {#each groups as group (group.key)}
        <section aria-label={group.label}>
          <h2 class="mb-2 px-1 text-[11px] font-semibold uppercase tracking-widest text-muted">{group.label}</h2>
          <ul class="divide-y divide-line overflow-hidden rounded-panel border border-line bg-surface">
            {#each group.items as item (item.id)}
              <li>
                <NotificationRow note={item} {now} roomy onfollow={(event) => void follow(event, item)} />
              </li>
            {/each}
          </ul>
        </section>
      {/each}
    </div>
  {/if}
</Page>
