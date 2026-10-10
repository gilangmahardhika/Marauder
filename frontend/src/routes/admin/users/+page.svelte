<script lang="ts">
  import { ApiError, getUsers, unreachable } from '$lib/api';
  import { initials, roleLabel } from '$lib/roles';
  import { shell, showShell } from '$lib/shell.svelte';
  import type { SessionUser } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Table from '$lib/ui/Table.svelte';

  let users = $state<SessionUser[]>([]);
  let loaded = $state(false);
  let message = $state('Loading users');

  showShell(() => ({ title: 'Users', message }));

  $effect(() => {
    void getUsers()
      .then((body) => {
        users = body.users;
        loaded = true;
        message = 'Users';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });
</script>

<Page title="Users" description="People who can sign in. Admin access is separate from job role.">
  {#snippet actions()}
    {#if shell.admin}<Button variant="primary" href="/admin/users/new">New user</Button>{/if}
  {/snippet}
  {#if loaded && users.length === 0}
    <EmptyState title="No users yet" />
  {:else if users.length > 0}
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
        {#each users as user (user.id)}
          <tr class={tr}>
            <td class={td}>
              <span class="flex items-center gap-3">
                <span
                  class="flex size-8 shrink-0 items-center justify-center rounded-full bg-sidebar text-xs font-medium text-ink"
                  aria-hidden="true">{initials(user.email)}</span
                >
                <span class="font-medium text-ink">{user.email}</span>
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
                aria-label={`Edit ${user.email}`}>Edit</a
              >
            </td>
          </tr>
        {/each}
      </tbody>
    </Table>
  {/if}
</Page>
