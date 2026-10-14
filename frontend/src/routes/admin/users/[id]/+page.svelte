<script lang="ts">
  import { page } from '$app/stores';
  import { getUsers, updateUser } from '$lib/api';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { jobRoles } from '$lib/roles';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';
  import Select from '$lib/ui/Select.svelte';

  const shown = ['name', 'role', 'admin', 'disabled', 'password'];

  let email = $state('');
  let name = $state('');
  let savedName = $state('');
  let role = $state('developer');
  let admin = $state(false);
  let disabled = $state(false);
  let password = $state('');
  let loading = $state('Loading user');
  let loadError = $state('');
  let busy = $state(false);
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: savedName || email || 'Edit user' }));

  function load(id: string) {
    loading = 'Loading user';
    loadError = '';
    void getUsers()
      .then((body) => {
        const user = body.users.find((item) => item.id === id);
        if (!user) {
          loadError = 'User not found';
          return;
        }
        email = user.email;
        name = user.name;
        savedName = user.name;
        role = user.role;
        admin = user.admin;
        disabled = Boolean(user.disabled);
      })
      .catch((error: unknown) => {
        loadError = failureText(error);
      })
      .finally(() => {
        loading = '';
      });
  }

  $effect(() => {
    const id = $page.params.id;
    if (id) load(id);
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    const id = $page.params.id;
    if (!id) return;
    busy = true;
    failure = noFailure;
    try {
      const saved = await updateUser(id, {
        ...(name.trim() === '' && savedName === '' ? {} : { name }),
        role,
        admin,
        disabled,
        ...(password === '' ? {} : { password })
      });
      email = saved.email;
      name = saved.name;
      savedName = saved.name;
      role = saved.role;
      admin = saved.admin;
      disabled = Boolean(saved.disabled);
      password = '';
      toast.success('User saved');
    } catch (error) {
      failure = formFailure(error, shown);
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title={savedName || email || 'Edit user'}
  description={savedName ? email : undefined}
  {loading}
  error={loadError}
  onretry={() => $page.params.id && load($page.params.id)}
>
  {#snippet meta()}
    {#if disabled}<Badge tone="red">Disabled</Badge>{/if}
  {/snippet}
  {#snippet actions()}
    <Button href="/admin/users" icon="arrow-left">All users</Button>
  {/snippet}
  <form onsubmit={submit} bind:this={form} novalidate>
    <div class="flex flex-col gap-6">
      <FormAlert message={failure.message} extra={failure.extra} />
      <Panel title="Name">
        <Field
          label="Name"
          id="name"
          autocomplete="off"
          hint={savedName ? 'Shown on the board and member lists.' : 'Not set yet. Until it is, pages show the GitHub login or the email.'}
          error={failure.fields.name}
          bind:value={name}
        />
      </Panel>
      <Panel title="Role and access">
        <Select label="Job role" id="role" error={failure.fields.role} bind:value={role}>
          {#each jobRoles as item (item.value)}
            <option value={item.value}>{item.label}</option>
          {/each}
        </Select>
        <div class="flex flex-col">
          <Checkbox
            id="admin"
            label="Admin"
            hint="Can manage users, tribes, and projects."
            error={failure.fields.admin}
            bind:checked={admin}
          />
          <Checkbox
            id="disabled"
            label="Disabled"
            hint="Cannot sign in. Existing sessions end."
            error={failure.fields.disabled}
            bind:checked={disabled}
          />
        </div>
      </Panel>
      <Panel title="Password">
        <Field
          label="New password"
          id="password"
          type="password"
          autocomplete="new-password"
          hint="Leave blank to keep the current password."
          error={failure.fields.password}
          bind:value={password}
        />
      </Panel>
    </div>
    <SaveBar {busy} />
  </form>
</Page>
