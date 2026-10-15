<script lang="ts">
  import NotificationRow from '$lib/components/NotificationRow.svelte';
  import type { Option } from '$lib/filter';
  import { failureText } from '$lib/forms';
  import { collapseRepeats, dayGroups, facets, fetchInbox, follow, inbox, kindLook, markAllRead, type Facet } from '$lib/notifications.svelte';
  import { showShell } from '$lib/shell.svelte';
  import type { AppNotification } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';

  type Show = 'all' | 'unread';

  const subhead = 'text-[11px] font-semibold uppercase tracking-wider text-muted';

  let show = $state<Show>('all');
  let kind = $state('');
  let project = $state('');
  /** Unread rows stay listed after they are read, until the filter changes. */
  let unreadRows = $state<AppNotification[]>([]);
  let loading = $state('Loading notifications');
  let loaded = $state(false);
  let error = $state('');
  let marking = $state(false);
  let now = $state(Date.now());

  showShell(() => ({ title: 'Notifications' }));

  const rows = $derived(show === 'all' ? inbox.items : unreadRows);
  const busy = $derived(loading !== '');
  const counts = $derived(facets(rows));
  const filtered = $derived(rows.filter((item) => (!kind || item.kind === kind) && (!project || item.projectId === project)));
  const groups = $derived(dayGroups(filtered, now).map((group) => ({ ...group, rows: collapseRepeats(group.items) })));
  const active = $derived(
    [
      kind ? { label: counts.kinds.find((facet) => facet.value === kind)?.label ?? kind, clear: () => (kind = '') } : null,
      project ? { label: counts.projects.find((facet) => facet.value === project)?.label ?? project, clear: () => (project = '') } : null
    ].filter((chip) => chip !== null)
  );
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
      loaded = true;
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

  function clearFilters() {
    kind = '';
    project = '';
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
  loading={loaded ? '' : loading}
  {error}
  onretry={() => void load(show)}
>
  <div class="flex max-w-6xl flex-col gap-6">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex flex-wrap items-center gap-2">
        <Segmented label="Show" {options} value={show} onchange={choose} />
        {#each active as chip (chip.label)}
          <button
            type="button"
            class="inline-flex min-h-11 items-center gap-1.5 rounded-full border border-accent/30 bg-accent/5 pl-3 pr-2 text-sm font-medium text-accent hover:bg-accent/10"
            aria-label={`Remove filter ${chip.label}`}
            onclick={chip.clear}
          >
            {chip.label}
            <Icon name="close" class="size-3.5" />
          </button>
        {/each}
      </div>
      <Button variant="ghost" icon="check" loading={marking} disabled={inbox.unread === 0} onclick={() => void readAll()}>Mark all read</Button>
    </div>

    <div class="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_300px]">
      <div class="min-w-0">
        {#if rows.length === 0 && !busy}
          {#if show === 'all'}
            <EmptyState title="No notifications yet" description="You are told here when a card needs you." />
          {:else}
            <EmptyState title="You're all caught up" description="Every notification is read." />
          {/if}
        {:else if filtered.length === 0 && !busy}
          <EmptyState title="No notifications match" description="Change the filters to see more notifications.">
            <Button type="button" icon="close" onclick={clearFilters}>Clear filters</Button>
          </EmptyState>
        {:else}
          <div class="flex flex-col gap-6 transition-opacity duration-150 {busy ? 'opacity-60' : ''}" aria-busy={busy}>
            {#each groups as group (group.key)}
              <section aria-label={group.label}>
                <div class="mb-2 flex items-center gap-2 px-1">
                  <h2 class="text-[11px] font-semibold uppercase tracking-widest text-muted">{group.label}</h2>
                  <span class="rounded-full border border-line bg-surface px-1.5 text-[11px] tabular-nums text-muted"
                    >{group.items.length}<span class="sr-only"> notifications</span></span
                  >
                  <span class="h-px flex-1 bg-line" aria-hidden="true"></span>
                </div>
                <ul class="divide-y divide-line overflow-hidden rounded-panel border border-line bg-surface shadow-xs">
                  {#each group.rows as row (row.note.id)}
                    <li>
                      <NotificationRow
                        note={row.note}
                        {now}
                        roomy
                        count={row.count}
                        unread={row.unread}
                        onfollow={(event) => void follow(event, row.note, row.notes.slice(1))}
                      />
                    </li>
                  {/each}
                </ul>
              </section>
            {/each}
          </div>
        {/if}
      </div>

      <aside class="hidden flex-col gap-4 lg:sticky lg:top-6 lg:flex" aria-label="Notification summary">
        <div class="rounded-panel border border-line bg-surface p-4 shadow-xs">
          <p class="text-2xl font-semibold tabular-nums text-ink">{inbox.unread}</p>
          <p class="text-sm text-muted">unread, of the {inbox.items.length} latest</p>
        </div>
        {#if counts.kinds.length > 0}
          {@render facetGroup('By type', 'Filter by type', counts.kinds, kind, (value) => (kind = kind === value ? '' : value), true)}
        {/if}
        {#if counts.projects.length > 1}
          {@render facetGroup('By project', 'Filter by project', counts.projects, project, (value) => (project = project === value ? '' : value), false)}
        {/if}
      </aside>
    </div>
  </div>
</Page>

{#snippet facetGroup(title: string, label: string, list: Facet[], value: string, pick: (value: string) => void, kinds: boolean)}
  <div class="rounded-panel border border-line bg-surface p-2 shadow-xs">
    <h3 class="{subhead} px-2 pb-1 pt-2">{title}</h3>
    <div class="flex flex-col gap-0.5" role="group" aria-label={label}>
      {#each list as facet (facet.value)}
        {@const selected = value === facet.value}
        <button
          type="button"
          aria-pressed={selected}
          class="flex min-h-11 w-full items-center gap-3 rounded-control border px-2 text-left text-sm transition-colors duration-150 {selected
            ? 'border-accent bg-accent/5 font-medium text-ink'
            : 'border-transparent text-ink/80 hover:bg-ground/60 hover:text-ink'}"
          onclick={() => pick(facet.value)}
        >
          {#if kinds}
            {@const look = kindLook(facet.value)}
            <span class="flex size-7 shrink-0 items-center justify-center rounded-full {look.chip}"><Icon name={look.icon} class="size-3.5" /></span>
          {/if}
          <span class="min-w-0 flex-1 truncate">{facet.label}</span>
          <span class="shrink-0 text-xs tabular-nums text-muted">{facet.count}</span>
        </button>
      {/each}
    </div>
  </div>
{/snippet}
