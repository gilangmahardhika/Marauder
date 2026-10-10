<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, addMember, getMembers, getTribes, getUsers, removeMember, unreachable } from '$lib/api';
  import { initials, roleLabel } from '$lib/roles';
  import { showShell } from '$lib/shell.svelte';
  import type { SessionUser } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SearchSelect from '$lib/ui/SearchSelect.svelte';
  import Table from '$lib/ui/Table.svelte';

  let tribeName = $state('');
  let members = $state<SessionUser[]>([]);
  let users = $state<SessionUser[]>([]);
  let userId = $state('');
  const candidates = $derived(
    users
      .filter((user) => !members.some((member) => member.id === user.id))
      .map((user) => ({ value: user.id, label: user.email, detail: roleLabel(user.role) }))
  );
  let pending = $state('');
  let message = $state('Loading members');
  let busy = $state(false);

  showShell(() => ({ title: tribeName || 'Tribe', message }));

  async function load(id: string) {
    try {
      const [memberBody, userBody, tribeBody] = await Promise.all([getMembers(id), getUsers(), getTribes()]);
      members = memberBody.members;
      users = userBody.users;
      tribeName = tribeBody.tribes.find((tribe) => tribe.id === id)?.name ?? 'Tribe';
      message = 'Members';
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    }
  }

  $effect(() => {
    const id = $page.params.id;
    if (id) void load(id);
  });

  async function add(event: SubmitEvent) {
    event.preventDefault();
    const id = $page.params.id;
    if (!id || userId === '') return;
    busy = true;
    message = 'Adding member';
    try {
      await addMember(id, userId);
      userId = '';
      await load(id);
      message = 'Member added';
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  async function remove(memberId: string) {
    const id = $page.params.id;
    if (!id) return;
    busy = true;
    message = 'Removing member';
    try {
      await removeMember(id, memberId);
      pending = '';
      await load(id);
      message = 'Member removed';
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title={tribeName || 'Tribe'}
  description={`${members.length} ${members.length === 1 ? 'member' : 'members'}`}
  width="wide"
>
  {#snippet actions()}
    <Button href="/tribes">All tribes</Button>
  {/snippet}
  <div class="flex flex-col gap-6">
    <form onsubmit={add}>
      <Panel title="Add member">
        <div class="flex flex-wrap items-end gap-3">
          <SearchSelect
            label="Email"
            id="member-search"
            placeholder="Search by email"
            options={candidates}
            bind:value={userId}
          />
          <Button variant="primary" type="submit" disabled={busy || userId === ''}>Add member</Button>
        </div>
      </Panel>
    </form>
    {#if members.length > 0}
      <Table label="Members">
        <thead>
          <tr class="border-b border-line bg-ground/50">
            <th class={th} scope="col">Member</th>
            <th class={th} scope="col">Role</th>
            <th class={th} scope="col"><span class="sr-only">Remove</span></th>
          </tr>
        </thead>
        <tbody>
          {#each members as member (member.id)}
            <tr class={tr}>
              <td class={td}>
                <span class="flex items-center gap-3">
                  <span
                    class="flex size-8 shrink-0 items-center justify-center rounded-full bg-sidebar text-xs font-medium text-ink"
                    aria-hidden="true">{initials(member.email)}</span
                  >
                  <span class="font-medium text-ink">{member.email}</span>
                  {#if member.disabled}<Badge tone="red">Disabled</Badge>{/if}
                </span>
              </td>
              <td class="{td} text-muted">{roleLabel(member.role)}</td>
              <td class="{td} text-right">
                {#if pending === member.id}
                  <span class="inline-flex flex-wrap items-center justify-end gap-2">
                    <span class="text-sm text-ink">Remove from {tribeName}?</span>
                    <Button type="button" variant="ghost" disabled={busy} onclick={() => (pending = '')}>Cancel</Button>
                    <Button type="button" variant="danger" disabled={busy} onclick={() => void remove(member.id)}>Remove</Button>
                  </span>
                {:else}
                  <Button type="button" variant="ghost" disabled={busy} onclick={() => (pending = member.id)}>Remove</Button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </Table>
    {:else}
      <EmptyState title="No members yet" description="Search for a user by email to add them." />
    {/if}
  </div>
</Page>
