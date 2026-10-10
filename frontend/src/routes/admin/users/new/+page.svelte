<script lang="ts">
  import { goto } from '$app/navigation';
  import { ApiError, createUser, unreachable } from '$lib/api';
  import { jobRoles } from '$lib/roles';
  import { showShell } from '$lib/shell.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import Field from '$lib/ui/Field.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Select from '$lib/ui/Select.svelte';

  let email = $state('');
  let password = $state('');
  let role = $state('developer');
  let admin = $state(false);
  let message = $state('New user');
  let busy = $state(false);
  let fields = $state<Record<string, string>>({});

  showShell(() => ({ title: 'New user', message }));

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    message = 'Creating user';
    fields = {};
    try {
      await createUser({ email, password, role, admin });
      message = 'User created';
      await goto('/admin/users');
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page title="New user" description="The user signs in with this email and password, then can change the password on their account page." width="form">
  <form onsubmit={submit}>
    <div class="flex flex-col gap-6">
      <Panel title="Sign-in">
        <Field label="Email" id="email" type="email" autocomplete="off" error={fields.email} bind:value={email} />
        <Field
          label="Password"
          id="password"
          type="password"
          autocomplete="new-password"
          hint="At least 12 characters."
          error={fields.password}
          bind:value={password}
        />
      </Panel>
      <Panel title="Role and access">
        <Select label="Job role" id="role" error={fields.role} bind:value={role}>
          {#each jobRoles as item (item.value)}
            <option value={item.value}>{item.label}</option>
          {/each}
        </Select>
        <Checkbox id="admin" label="Admin" hint="Can manage users, tribes, and projects." bind:checked={admin} />
        {#snippet footer()}
          <Button variant="ghost" href="/admin/users">Cancel</Button>
          <Button variant="primary" type="submit" disabled={busy}>Create user</Button>
        {/snippet}
      </Panel>
    </div>
  </form>
</Page>
