<script lang="ts">
  import { page } from '$app/stores';
  import { addMember, getMembers, getTribeCandidates, getTribes, removeMember, setTribeAdmin } from '$lib/api';
  import { pickParam } from '$lib/filter';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { pager } from '$lib/pager.svelte';
  import { setParams } from '$lib/params';
  import { displayName, filterUsers, roleOptions } from '$lib/people';
  import { roleLabel } from '$lib/roles';
  import { shell, showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { TribeCandidate, TribeMember } from '$lib/types';
  import Avatar from '$lib/ui/Avatar.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { td, th, tr } from '$lib/ui/control';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import FilterSelect from '$lib/ui/FilterSelect.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SearchBox from '$lib/ui/SearchBox.svelte';
  import SearchSelect from '$lib/ui/SearchSelect.svelte';
  import ShowMore from '$lib/ui/ShowMore.svelte';
  import Table from '$lib/ui/Table.svelte';
  import Toolbar from '$lib/ui/Toolbar.svelte';

  let tribeName = $state('');
  let members = $state<TribeMember[]>([]);
  let users = $state<TribeCandidate[]>([]);
  let userId = $state('');
  const candidates = $derived(
    users
      .filter((user) => !members.some((member) => member.id === user.id))
      .map((user) => ({
        value: user.id,
        label: displayName(user),
        detail: user.name ? `${user.email} · ${roleLabel(user.role)}` : roleLabel(user.role)
      }))
  );
  let pending = $state('');
  let loading = $state('Loading members');
  let loadError = $state('');
  /** The action in flight: `add`, `admin:<userId>`, or `remove:<userId>`. */
  let busy = $state('');
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: tribeName || 'Tribe' }));

  const params = $derived($page.url.searchParams);
  const filter = $derived({
    q: params.get('q') ?? '',
    role: pickParam(params.get('role'), roleOptions.map((option) => option.value), ''),
    access: 'all' as const
  });
  const shown = $derived(filterUsers(members, filter));
  const paging = pager(() => `${filter.q}|${filter.role}`);

  async function refresh(id: string) {
    const [memberBody, userBody, tribeBody] = await Promise.all([getMembers(id), getTribeCandidates(id), getTribes()]);
    members = memberBody.members;
    users = userBody.users;
    tribeName = tribeBody.tribes.find((tribe) => tribe.id === id)?.name ?? 'Tribe';
  }

  function load(id: string) {
    loading = 'Loading members';
    loadError = '';
    refresh(id)
      .catch((error: unknown) => {
        loadError = failureText(error);
      })
      .finally(() => {
        loading = '';
      });
  }

  /** Reloads after an action; a failure leaves the list on screen. */
  async function reload(id: string) {
    try {
      await refresh(id);
    } catch (error) {
      toast.error(failureText(error));
    }
  }

  function actionFailed(error: unknown) {
    toast.error(failureText(error));
  }

  $effect(() => {
    const id = $page.params.id;
    if (id) load(id);
  });

  async function add(event: SubmitEvent) {
    event.preventDefault();
    const id = $page.params.id;
    if (!id || userId === '') return;
    busy = 'add';
    failure = noFailure;
    try {
      await addMember(id, userId);
      userId = '';
      toast.success('Member added');
      await reload(id);
    } catch (error) {
      failure = formFailure(error, ['userId']);
      void focusInvalid(form);
    } finally {
      busy = '';
    }
  }

  /** A tribe admin may not remove or demote themselves; a company admin may. */
  function isSelf(member: TribeMember) {
    return !shell.admin && member.email.toLowerCase() === shell.email.toLowerCase();
  }

  async function toggleAdmin(member: TribeMember) {
    const id = $page.params.id;
    if (!id) return;
    busy = `admin:${member.id}`;
    try {
      await setTribeAdmin(id, member.id, !member.tribeAdmin);
      toast.success(
        member.tribeAdmin
          ? `${displayName(member)} is no longer a tribe admin`
          : `${displayName(member)} is a tribe admin`
      );
      await reload(id);
    } catch (error) {
      actionFailed(error);
    } finally {
      busy = '';
    }
  }

  async function remove(memberId: string) {
    const id = $page.params.id;
    if (!id) return;
    busy = `remove:${memberId}`;
    try {
      await removeMember(id, memberId);
      pending = '';
      toast.success('Member removed');
      await reload(id);
    } catch (error) {
      actionFailed(error);
    } finally {
      busy = '';
    }
  }
</script>

<Page
  title={tribeName || 'Tribe'}
  description={`${members.length} ${members.length === 1 ? 'member' : 'members'}`}
  {loading}
  error={loadError}
  onretry={() => $page.params.id && load($page.params.id)}
>
  {#snippet actions()}
    <Button href={`/tribes/${$page.params.id}/usage`}>Token usage</Button>
    <Button href={`/tribes/${$page.params.id}/mcp`}>MCP servers</Button>
    <Button href="/tribes" icon="arrow-left">All tribes</Button>
  {/snippet}
  <div class="flex flex-col gap-6">
    <form onsubmit={add} bind:this={form} novalidate>
      <Panel title="Add member">
        <FormAlert message={failure.message} extra={failure.extra} />
        <div class="flex flex-wrap items-end gap-3">
          <SearchSelect
            label="User"
            id="member-search"
            placeholder="Search by name or email"
            options={candidates}
            error={failure.fields.userId}
            bind:value={userId}
          />
          <Button
            variant="primary"
            type="submit"
            icon="plus"
            loading={busy === 'add'}
            disabled={busy !== '' || userId === ''}>Add member</Button
          >
        </div>
      </Panel>
    </form>
    {#if members.length > 0}
      <div class="flex flex-col gap-4">
        <Toolbar>
          <SearchBox label="Search members" value={filter.q} placeholder="Search name or email" />
          <FilterSelect label="Role" options={roleOptions} value={filter.role} onchange={(role) => setParams({ role })} />
        </Toolbar>
        {#if shown.length === 0}
          <EmptyState title="No members match" description="Change the search or role to see more members.">
            <Button type="button" variant="secondary" icon="close" onclick={() => setParams({ q: '', role: '' })}>Clear filters</Button>
          </EmptyState>
        {:else}
          <Table label="Members">
            <thead>
              <tr class="border-b border-line bg-ground/50">
                <th class={th} scope="col">Member</th>
                <th class={th} scope="col">Role</th>
                <th class={th} scope="col"><span class="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody>
              {#each shown.slice(0, paging.limit) as member (member.id)}
                <tr class={tr}>
                  <td class={td}>
                    <span class="flex items-center gap-3">
                      <Avatar person={member} />
                      <span class="min-w-0">
                        <span class="block font-medium text-ink">{displayName(member)}</span>
                        {#if member.name}<span class="block break-all text-xs text-muted">{member.email}</span>{/if}
                      </span>
                      {#if member.tribeAdmin}<Badge tone="green">Tribe admin</Badge>{/if}
                      {#if member.disabled}<Badge tone="red">Disabled</Badge>{/if}
                    </span>
                  </td>
                  <td class="{td} text-muted">{roleLabel(member.role)}</td>
                  <td class="{td} text-right">
                    {#if pending === member.id}
                      <span class="inline-flex flex-wrap items-center justify-end gap-2">
                        <span class="text-sm text-ink">Remove from {tribeName}?</span>
                        <Button type="button" variant="ghost" disabled={busy !== ''} onclick={() => (pending = '')}>Cancel</Button>
                        <Button
                          type="button"
                          variant="danger"
                          icon="trash"
                          loading={busy === `remove:${member.id}`}
                          disabled={busy !== ''}
                          onclick={() => void remove(member.id)}>Remove</Button
                        >
                      </span>
                    {:else if !isSelf(member)}
                      <span class="inline-flex flex-wrap items-center justify-end gap-1">
                        <Button
                          type="button"
                          variant="ghost"
                          loading={busy === `admin:${member.id}`}
                          disabled={busy !== ''}
                          onclick={() => void toggleAdmin(member)}
                          >{member.tribeAdmin ? 'Remove tribe admin' : 'Make tribe admin'}</Button
                        >
                        <Button type="button" variant="ghost" icon="trash" disabled={busy !== ''} onclick={() => (pending = member.id)}
                          >Remove</Button
                        >
                      </span>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </Table>
          <ShowMore total={shown.length} limit={paging.limit} noun={['member', 'members']} onmore={paging.more} />
        {/if}
      </div>
    {:else}
      <EmptyState title="No members yet" description="Search for a user by name or email to add them." />
    {/if}
  </div>
</Page>
