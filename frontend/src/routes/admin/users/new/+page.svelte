<script lang="ts">
  import { goto } from '$app/navigation';
  import { createUser } from '$lib/api';
  import { focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { jobRoles } from '$lib/roles';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Select from '$lib/ui/Select.svelte';

  const shown = ['name', 'email', 'password', 'role', 'admin'];

  let name = $state('');
  let email = $state('');
  let password = $state('');
  let role = $state('developer');
  let admin = $state(false);
  let busy = $state(false);
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: 'New user' }));

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failure = noFailure;
    try {
      await createUser({ name, email, password, role, admin });
      toast.success('User created');
      await goto('/admin/users');
    } catch (error) {
      failure = formFailure(error, shown);
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }
</script>

<Page title="New user" description="The user signs in with this email and password, then can change the password on their account page.">
  <form onsubmit={submit} bind:this={form} novalidate>
    <div class="flex flex-col gap-6">
      <FormAlert message={failure.message} extra={failure.extra} />
      <Panel title="Sign-in">
        <Field label="Name" id="name" autocomplete="off" hint="Shown on the board and member lists." error={failure.fields.name} bind:value={name} />
        <Field label="Email" id="email" type="email" autocomplete="off" error={failure.fields.email} bind:value={email} />
        <Field
          label="Password"
          id="password"
          type="password"
          autocomplete="new-password"
          hint="At least 12 characters."
          error={failure.fields.password}
          bind:value={password}
        />
      </Panel>
      <Panel title="Role and access">
        <Select label="Job role" id="role" error={failure.fields.role} bind:value={role}>
          {#each jobRoles as item (item.value)}
            <option value={item.value}>{item.label}</option>
          {/each}
        </Select>
        <Checkbox id="admin" label="Admin" hint="Can manage users, tribes, and projects." error={failure.fields.admin} bind:checked={admin} />
        {#snippet footer()}
          <Button variant="ghost" href="/admin/users">Cancel</Button>
          <Button variant="primary" type="submit" icon="plus" loading={busy}>Create user</Button>
        {/snippet}
      </Panel>
    </div>
  </form>
</Page>
