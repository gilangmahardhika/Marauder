<script lang="ts">
  import { ApiError, getGitHub, putGitHub, testGitHub, unreachable } from '$lib/api';
  import SecretField, { omitEmptySecret } from '$lib/components/SecretField.svelte';
  import { shell, showShell } from '$lib/shell.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';

  let host = $state('github.com');
  let owner = $state('');
  let repo = $state('');
  let baseBranch = $state('main');
  let botLogin = $state('');
  let token = $state('');
  let clear = $state(false);
  let tokenSet = $state(false);
  let message = $state('Loading GitHub settings');
  let result = $state('');
  let busy = $state(false);
  let fields = $state<Record<string, string>>({});

  showShell(() => ({ title: 'GitHub', message }));

  $effect(() => {
    if (shell.projectId === '') return;
    void getGitHub()
      .then((body) => {
        host = body.host;
        owner = body.owner;
        repo = body.repo;
        baseBranch = body.baseBranch;
        botLogin = body.botLogin;
        tokenSet = body.tokenSet;
        message = 'GitHub settings';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    message = 'Saving GitHub settings';
    fields = {};
    const body: Record<string, unknown> = {
      host,
      owner,
      repo,
      baseBranch,
      botLogin,
      ...omitEmptySecret('token', token, clear)
    };
    try {
      const saved = await putGitHub(body);
      tokenSet = saved.tokenSet;
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
      const outcome = await testGitHub();
      result = outcome.ok ? `Signed in as ${outcome.login ?? 'ok'}` : (outcome.message ?? 'authentication failed');
      message = result;
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page title="GitHub" description="The repository this project works in, and the bot that opens its pull requests." width="form">
  <form onsubmit={save}>
    <Panel title="Repository">
      <div class="grid gap-5 sm:grid-cols-2">
        <Field label="Host" id="host" width="full" error={fields.host} bind:value={host} />
        <Field label="Base branch" id="base" width="full" mono error={fields.baseBranch} bind:value={baseBranch} />
        <Field label="Owner" id="owner" width="full" error={fields.owner} bind:value={owner} />
        <Field label="Repository" id="repo" width="full" error={fields.repo} bind:value={repo} />
      </div>
    </Panel>
    <div class="mt-6">
      <Panel title="Bot" hint="The account that pushes branches and opens pull requests.">
        <Field label="Bot login" id="bot" error={fields.botLogin} bind:value={botLogin} />
        <SecretField label="Bot token" name="token" saved={tokenSet} error={fields.token} bind:value={token} bind:clear />
        {#snippet footer()}
          {#if result}<span class="mr-auto text-sm text-muted">{result}</span>{/if}
          <Button type="button" variant="secondary" disabled={busy || (!tokenSet && token === '')} onclick={() => void test()}>
            Test connection
          </Button>
        {/snippet}
      </Panel>
    </div>
    <SaveBar {busy} />
  </form>
</Page>
