<script lang="ts">
  import { changePassword, getMe, getMyGitHub, putMe, putMyGitHub, testMyGitHub } from '$lib/api';
  import NotificationsPanel from '$lib/components/NotificationsPanel.svelte';
  import SecretField, { omitEmptySecret } from '$lib/components/SecretField.svelte';
  import { failureText, focusInvalid, formFailure, noFailure, type FormFailure } from '$lib/forms';
  import { roleLabel } from '$lib/roles';
  import { shell, showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  type FormName = 'name' | 'password' | 'github';

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
  let loading = $state('Loading account');
  let loadError = $state('');
  let result = $state('');
  let busy = $state<FormName | 'test' | ''>('');
  let failures = $state<Record<FormName, FormFailure>>({ name: noFailure, password: noFailure, github: noFailure });
  let forms = $state<Partial<Record<FormName, HTMLFormElement>>>({});

  showShell(() => ({ title: 'Account' }));

  function load() {
    loading = 'Loading account';
    loadError = '';
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
      })
      .catch((error: unknown) => {
        loadError = failureText(error);
      })
      .finally(() => {
        loading = '';
      });
  }

  $effect(load);

  async function run(form: FormName, shown: string[], work: () => Promise<void>) {
    busy = form;
    failures[form] = noFailure;
    try {
      await work();
    } catch (error) {
      failures[form] = formFailure(error, shown);
      void focusInvalid(forms[form]);
    } finally {
      busy = '';
    }
  }

  function saveName(event: SubmitEvent) {
    event.preventDefault();
    void run('name', ['name'], async () => {
      const me = await putMe({ name });
      name = me.name;
      savedName = me.name;
      shell.name = me.name;
      toast.success('Name saved');
    });
  }

  function savePassword(event: SubmitEvent) {
    event.preventDefault();
    void run('password', ['currentPassword', 'newPassword'], async () => {
      await changePassword(currentPassword, newPassword);
      currentPassword = '';
      newPassword = '';
      toast.success('Password changed');
    });
  }

  function saveGitHub(event: SubmitEvent) {
    event.preventDefault();
    void run('github', ['host', 'login', 'token'], async () => {
      const saved = await putMyGitHub({ host, login, ...omitEmptySecret('token', token, clear) });
      tokenSet = saved.tokenSet;
      verified = saved.verified;
      login = saved.login;
      token = '';
      clear = false;
      toast.success('GitHub settings saved');
    });
  }

  async function test() {
    busy = 'test';
    try {
      const outcome = await testMyGitHub();
      result = outcome.ok ? `Signed in as ${outcome.login ?? 'ok'}` : (outcome.message ?? 'authentication failed');
      if (outcome.ok) toast.success(result);
      else toast.error(`GitHub: ${result}`);
      const github = await getMyGitHub();
      login = github.login;
      verified = github.verified;
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      busy = '';
    }
  }
</script>

<Page
  title={savedName || email || 'Account'}
  description="Your sign-in and your own GitHub identity. Project bot and deploy tokens live in project settings."
  {loading}
  error={loadError}
  onretry={load}
>
  {#snippet actions()}
    <Button href="/account/usage" icon="chart">Token usage</Button>
  {/snippet}
  {#snippet meta()}
    {#if role}<Badge>{roleLabel(role)}</Badge>{/if}
    {#if admin}<Badge tone="green">Admin</Badge>{/if}
  {/snippet}
  <div class="flex flex-col gap-6">
    <form onsubmit={saveName} bind:this={forms.name} novalidate>
      <Panel title="Name" hint={`Shown on the board, cards, and member lists instead of your email. You sign in with ${email || 'your email'}.`}>
        <FormAlert message={failures.name.message} extra={failures.name.extra} />
        <Field label="Name" id="name" autocomplete="name" error={failures.name.fields.name} bind:value={name} />
        {#snippet footer()}
          <Button
            variant="secondary"
            type="submit"
            icon="save"
            loading={busy === 'name'}
            disabled={busy !== '' || name.trim() === savedName}>Save name</Button
          >
        {/snippet}
      </Panel>
    </form>
    <form onsubmit={savePassword} bind:this={forms.password} novalidate>
      <Panel title="Password" hint="Use at least 12 characters.">
        <FormAlert message={failures.password.message} extra={failures.password.extra} />
        <Field
          label="Current password"
          id="current"
          type="password"
          autocomplete="current-password"
          error={failures.password.fields.currentPassword}
          bind:value={currentPassword}
        />
        <Field
          label="New password"
          id="next"
          type="password"
          autocomplete="new-password"
          error={failures.password.fields.newPassword}
          bind:value={newPassword}
        />
        {#snippet footer()}
          <Button
            variant="secondary"
            type="submit"
            icon="save"
            loading={busy === 'password'}
            disabled={busy !== ''}>Change password</Button
          >
        {/snippet}
      </Panel>
    </form>
    <form onsubmit={saveGitHub} bind:this={forms.github} novalidate>
      <Panel
        title="GitHub"
        hint="Your own account. Cards built from requirements you bring in are committed, pushed, and opened as pull requests with this token. Test connection records the login GitHub reports."
      >
        {#snippet actions()}
          <Badge tone={verified ? 'green' : 'amber'}>{verified ? 'Verified' : 'Not verified'}</Badge>
        {/snippet}
        <FormAlert message={failures.github.message} extra={failures.github.extra} />
        <div class="grid gap-5 sm:grid-cols-2">
          <Field label="Host" id="host" width="full" error={failures.github.fields.host} bind:value={host} />
          <Field label="Login" id="login" width="full" error={failures.github.fields.login} bind:value={login} />
        </div>
        <SecretField
          label="Token"
          name="token"
          saved={tokenSet}
          error={failures.github.fields.token}
          bind:value={token}
          bind:clear
        />
        {#snippet footer()}
          {#if result}<span class="mr-auto text-sm text-muted">{result}</span>{/if}
          <Button
            type="button"
            variant="secondary"
            icon="zap"
            loading={busy === 'test'}
            disabled={busy !== '' || (!tokenSet && token === '')}
            onclick={() => void test()}
          >
            Test connection
          </Button>
          <Button variant="primary" type="submit" icon="save" loading={busy === 'github'} disabled={busy !== ''}
            >Save GitHub settings</Button
          >
        {/snippet}
      </Panel>
    </form>
    <NotificationsPanel />
  </div>
</Page>
