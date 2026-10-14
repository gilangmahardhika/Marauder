<script lang="ts">
  import { page } from '$app/stores';
  import { getUsers } from '$lib/api';
  import { pickParam } from '$lib/filter';
  import { failureText } from '$lib/forms';
  import { pager } from '$lib/pager.svelte';
  import { setParams } from '$lib/params';
  import { accessOptions, accessValues, displayName, filterUsers, roleOptions } from '$lib/people';
  import { roleLabel } from '$lib/roles';
  import { shell, showShell } from '$lib/shell.svelte';
  import type { SessionUser } from '$lib/types';
  import Avatar from '$lib/ui/Avatar.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import FilterSelect from '$lib/ui/FilterSelect.svelte';
  import Page from '$lib/ui/Page.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';
  import ShowMore from '$lib/ui/ShowMore.svelte';
  import Table from '$lib/ui/Table.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  let users = $state<SessionUser[]>([]);
  let loaded = $state(false);
  let loading = $state('Loading users');
  let loadError = $state('');

  showShell(() => ({ title: 'Users' }));

  const params = $derived($page.url.searchParams);
  const filter = $derived({
    q: params.get('q') ?? '',
    role: pickParam(params.get('role'), roleOptions.map((option) => option.value), ''),
    access: pickParam(params.get('access'), accessValues, 'all')
  });
  const shown = $derived(filterUsers(users, filter));
  const paging = pager(() => `${filter.q}|${filter.role}|${filter.access}`);

  function load() {
    loading = 'Loading users';
    loadError = '';
    void getUsers()
      .then((body) => {
        users = body.users;
        loaded = true;
      })
      .catch((error: unknown) => {
        loadError = failureText(error);
      })
      .finally(() => {
        loading = '';
      });
  }

  $effect(() => {
    load();
  });
</script>

<Page title="Users" description="People who can sign in. Admin access is separate from job role." {loading} error={loadError} onretry={load}>
  {#snippet actions()}
    {#if shell.admin}<Button variant="primary" href="/admin/users/new" icon="plus">New user</Button>{/if}
  {/snippet}
  {#if loaded && users.length === 0}
    <EmptyState title="No users yet" />
  {:else if users.length > 0}
    <div class="flex flex-col gap-4">
      <Toolbar>
        <SearchBox label="Search users" value={filter.q} placeholder="Search name or email" />
        <FilterSelect label="Role" options={roleOptions} value={filter.role} onchange={(role) => setParams({ role })} />
        <Segmented
          label="Access"
          options={accessOptions(users)}
          value={filter.access}
          onchange={(access) => setParams({ access: access === 'all' ? '' : access })}
        />
      </Toolbar>
      {#if shown.length === 0}
        <EmptyState title="No users match" description="Change the search or filters to see more users.">
          <Button type="button" variant="secondary" icon="close" onclick={() => setParams({ q: '', role: '', access: '' })}>Clear filters</Button>
        </EmptyState>
      {:else}
        <Table label="Users">
          <thead>
            <tr class="border-b border-line bg-ground/50">
              <th class={th} scope="col">User</th>
              <th class={th} scope="col">Role</th>
              <th class={th} scope="col">Access</th>
              <th class={th} scope="col"><span class="sr-only">Edit</span></th>
            </tr>
          </thead>
          <tbody>
            {#each shown.slice(0, paging.limit) as user (user.id)}
              <tr class={tr}>
                <td class={td}>
                  <span class="flex items-center gap-3">
                    <Avatar person={user} />
                    <span class="min-w-0">
                      <span class="block font-medium text-ink">{displayName(user)}</span>
                      {#if user.name}<span class="block break-all text-xs text-muted">{user.email}</span>{/if}
                    </span>
                  </span>
                </td>
                <td class="{td} text-muted">{roleLabel(user.role)}</td>
                <td class={td}>
                  <span class="flex flex-wrap gap-1.5">
                    {#if user.admin}<Badge tone="green">Admin</Badge>{/if}
                    {#if user.disabled}<Badge tone="red">Disabled</Badge>{:else}<Badge>Active</Badge>{/if}
                  </span>
                </td>
                <td class="{td} text-right">
                  <a
                    class="inline-flex min-h-11 items-center px-2 text-sm font-medium text-accent hover:underline"
                    href={`/admin/users/${user.id}`}
                    aria-label={`Edit ${displayName(user)}`}>Edit</a
                  >
                </td>
              </tr>
            {/each}
          </tbody>
        </Table>
        <ShowMore total={shown.length} limit={paging.limit} noun={['user', 'users']} onmore={paging.more} />
      {/if}
    </div>
  {/if}
</Page>
