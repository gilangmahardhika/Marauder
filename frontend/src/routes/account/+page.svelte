<script lang="ts">
  import { ApiError, changePassword, getMe, getMyGitHub, putMe, putMyGitHub, testMyGitHub, unreachable } from '$lib/api';
  import NotificationsPanel from '$lib/components/NotificationsPanel.svelte';
  import SecretField, { omitEmptySecret } from '$lib/components/SecretField.svelte';
  import { roleLabel } from '$lib/roles';
  import { shell, showShell } from '$lib/shell.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let email = $state('');
  let name = $state('');
  let savedName = $state('');
  let role = $state('');
  let admin = $state(false);
  let currentPassword = $state('');
  let newPassword = $state('');
  let host = $state('github.com');
  let login = $state('');
  let token = $state('');
  let clear = $state(false);
  let tokenSet = $state(false);
  let verified = $state(false);
  let message = $state('Loading account');
  let result = $state('');
  let busy = $state(false);
  let fields = $state<Record<string, string>>({});

  showShell(() => ({ title: 'Account', message }));

  $effect(() => {
    void Promise.all([getMe(), getMyGitHub()])
      .then(([me, github]) => {
        email = me.email;
        name = me.name;
        savedName = me.name;
        role = me.role;
        admin = me.admin;
        host = github.host || 'github.com';
        login = github.login;
        tokenSet = github.tokenSet;
        verified = github.verified;
        message = 'Account';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  async function saveName(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    message = 'Saving name';
    fields = {};
    try {
      const me = await putMe({ name });
      name = me.name;
      savedName = me.name;
      shell.name = me.name;
      message = 'Name saved';
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

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
      verified = saved.verified;
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
      const github = await getMyGitHub();
      login = github.login;
      verified = github.verified;
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title={savedName || email || 'Account'}
  description="Your sign-in and your own GitHub identity. Project bot and deploy tokens live in project settings."
>
  {#snippet actions()}
    <Button href="/account/usage">Token usage</Button>
  {/snippet}
  {#snippet meta()}
    {#if role}<Badge>{roleLabel(role)}</Badge>{/if}
    {#if admin}<Badge tone="green">Admin</Badge>{/if}
  {/snippet}
  <div class="flex flex-col gap-6">
    <form onsubmit={saveName}>
      <Panel title="Name" hint={`Shown on the board, cards, and member lists instead of your email. You sign in with ${email || 'your email'}.`}>
        <Field label="Name" id="name" autocomplete="name" error={fields.name} bind:value={name} />
        {#snippet footer()}
          <Button variant="secondary" type="submit" disabled={busy || name.trim() === savedName}>Save name</Button>
        {/snippet}
      </Panel>
    </form>
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
      <Panel
        title="GitHub"
        hint="Your own account. Cards built from requirements you bring in are committed, pushed, and opened as pull requests with this token. Test connection records the login GitHub reports."
      >
        {#snippet actions()}
          <Badge tone={verified ? 'green' : 'amber'}>{verified ? 'Verified' : 'Not verified'}</Badge>
        {/snippet}
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
    <NotificationsPanel />
  </div>
</Page>
