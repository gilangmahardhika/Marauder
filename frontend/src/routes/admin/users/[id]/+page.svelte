<script lang="ts">
  import { page } from '$app/stores';
  import { ApiError, getUsers, unreachable, updateUser } from '$lib/api';
  import { jobRoles } from '$lib/roles';
  import { showShell } from '$lib/shell.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import Field from '$lib/ui/Field.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';
  import Select from '$lib/ui/Select.svelte';

  let email = $state('');
  let role = $state('developer');
  let admin = $state(false);
  let disabled = $state(false);
  let password = $state('');
  let message = $state('Loading user');
  let busy = $state(false);
  let fields = $state<Record<string, string>>({});

  showShell(() => ({ title: email || 'Edit user', message }));

  $effect(() => {
    const id = $page.params.id;
    if (!id) return;
    void getUsers()
      .then((body) => {
        const user = body.users.find((item) => item.id === id);
        if (!user) {
          message = 'User not found';
          return;
        }
        email = user.email;
        role = user.role;
        admin = user.admin;
        disabled = Boolean(user.disabled);
        message = 'Edit user';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    const id = $page.params.id;
    if (!id) return;
    busy = true;
    message = 'Saving user';
    fields = {};
    try {
      const saved = await updateUser(id, {
        role,
        admin,
        disabled,
        ...(password === '' ? {} : { password })
      });
      email = saved.email;
      role = saved.role;
      admin = saved.admin;
      disabled = Boolean(saved.disabled);
      password = '';
      message = 'User saved';
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page title={email || 'Edit user'} width="form">
  {#snippet meta()}
    {#if disabled}<Badge tone="red">Disabled</Badge>{/if}
  {/snippet}
  {#snippet actions()}
    <Button href="/admin/users">All users</Button>
  {/snippet}
  <form onsubmit={submit}>
    <div class="flex flex-col gap-6">
      <Panel title="Role and access">
        <Select label="Job role" id="role" error={fields.role} bind:value={role}>
          {#each jobRoles as item (item.value)}
            <option value={item.value}>{item.label}</option>
          {/each}
        </Select>
        <div class="flex flex-col">
          <Checkbox id="admin" label="Admin" hint="Can manage users, tribes, and projects." bind:checked={admin} />
          <Checkbox id="disabled" label="Disabled" hint="Cannot sign in. Existing sessions end." bind:checked={disabled} />
        </div>
      </Panel>
      <Panel title="Password">
        <Field
          label="New password"
          id="password"
          type="password"
          autocomplete="new-password"
          hint="Leave blank to keep the current password."
          error={fields.password}
          bind:value={password}
        />
      </Panel>
    </div>
    <SaveBar {busy} />
  </form>
</Page>
