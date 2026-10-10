<script lang="ts">
  import { ApiError, changePassword, getMe, getMyGitHub, putMyGitHub, testMyGitHub, unreachable } from '$lib/api';
  import SecretField, { omitEmptySecret } from '$lib/components/SecretField.svelte';
  import { roleLabel } from '$lib/roles';
  import { showShell } from '$lib/shell.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let email = $state('');
  let role = $state('');
  let admin = $state(false);
  let currentPassword = $state('');
  let newPassword = $state('');
  let host = $state('github.com');
  let login = $state('');
  let token = $state('');
  let clear = $state(false);
  let tokenSet = $state(false);
  let message = $state('Loading account');
  let result = $state('');
  let busy = $state(false);
  let fields = $state<Record<string, string>>({});

  showShell(() => ({ title: 'Account', message }));

  $effect(() => {
    void Promise.all([getMe(), getMyGitHub()])
      .then(([me, github]) => {
        email = me.email;
        role = me.role;
        admin = me.admin;
        host = github.host || 'github.com';
        login = github.login;
        tokenSet = github.tokenSet;
        message = 'Account';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  async function savePassword(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    message = 'Changing password';
    fields = {};
    try {
      await changePassword(currentPassword, newPassword);
      currentPassword = '';
      newPassword = '';
      message = 'Password changed';
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  async function saveGitHub(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    message = 'Saving GitHub settings';
    fields = {};
    try {
      const saved = await putMyGitHub({ host, login, ...omitEmptySecret('token', token, clear) });
      tokenSet = saved.tokenSet;
      login = saved.login;
      token = '';
      clear = false;
      message = 'GitHub settings';
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  async function test() {
    busy = true;
    try {
      const outcome = await testMyGitHub();
      result = outcome.ok ? `Signed in as ${outcome.login ?? 'ok'}` : (outcome.message ?? 'authentication failed');
      message = result;
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title={email || 'Account'}
  description="Your sign-in and your own GitHub identity. Project bot and deploy tokens live in project settings."
>
  {#snippet meta()}
    {#if role}<Badge>{roleLabel(role)}</Badge>{/if}
    {#if admin}<Badge tone="green">Admin</Badge>{/if}
  {/snippet}
  <div class="flex flex-col gap-6">
    <form onsubmit={savePassword}>
      <Panel title="Password" hint="Use at least 12 characters.">
        <Field
          label="Current password"
          id="current"
          type="password"
          autocomplete="current-password"
          error={fields.currentPassword}
          bind:value={currentPassword}
        />
        <Field
          label="New password"
          id="next"
          type="password"
          autocomplete="new-password"
          error={fields.newPassword}
          bind:value={newPassword}
        />
        {#snippet footer()}
          <Button variant="secondary" type="submit" disabled={busy}>Change password</Button>
        {/snippet}
      </Panel>
    </form>
    <form onsubmit={saveGitHub}>
      <Panel title="GitHub" hint="Your own login, used to approve cards. It is not the company GitHub account.">
        <div class="grid gap-5 sm:grid-cols-2">
          <Field label="Host" id="host" width="full" error={fields.host} bind:value={host} />
          <Field label="Login" id="login" width="full" bind:value={login} />
        </div>
        <SecretField label="Token" name="token" saved={tokenSet} error={fields.token} bind:value={token} bind:clear />
        {#snippet footer()}
          {#if result}<span class="mr-auto text-sm text-muted">{result}</span>{/if}
          <Button type="button" variant="secondary" disabled={busy || (!tokenSet && token === '')} onclick={() => void test()}>
            Test connection
          </Button>
          <Button variant="primary" type="submit" disabled={busy}>Save GitHub settings</Button>
        {/snippet}
      </Panel>
    </form>
  </div>
</Page>
