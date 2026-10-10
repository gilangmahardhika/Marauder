<script lang="ts">
  import { ApiError, getGitHub, putGitHub, testGitHub, unreachable } from '$lib/api';
  import Live from '$lib/components/Live.svelte';
  import SecretField, { omitEmptySecret } from '$lib/components/SecretField.svelte';

  let host = $state('github.com');
  let owner = $state('');
  let repo = $state('');
  let baseBranch = $state('main');
  let approvers = $state('');
  let botLogin = $state('');
  let token = $state('');
  let clear = $state(false);
  let tokenSet = $state(false);
  let message = $state('Loading GitHub settings');
  let busy = $state(false);
  let fields = $state<Record<string, string>>({});

  $effect(() => {
    void getGitHub()
      .then((body) => {
        host = body.host;
        owner = body.owner;
        repo = body.repo;
        baseBranch = body.baseBranch;
        approvers = body.approvers.join('\n');
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
      approvers: approvers.split('\n').map((line) => line.trim()).filter(Boolean),
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
      const result = await testGitHub();
      message = result.ok ? (result.login ?? 'ok') : (result.message ?? 'authentication failed');
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<h1>GitHub</h1>
<Live {message} />
<form class="form" onsubmit={save}>
  <label for="host">Host <input id="host" bind:value={host} /></label>
  <label for="owner">Owner <input id="owner" bind:value={owner} /></label>
  {#if fields.owner}<p class="error">{fields.owner}</p>{/if}
  <label for="repo">Repository <input id="repo" bind:value={repo} /></label>
  <label for="base">Base branch <input id="base" bind:value={baseBranch} /></label>
  <label for="approvers">Approvers <textarea id="approvers" bind:value={approvers}></textarea></label>
  <label for="bot">Bot login <input id="bot" bind:value={botLogin} /></label>
  <SecretField label="Token" name="token" saved={tokenSet} bind:value={token} bind:clear />
  <button class="primary" type="submit" disabled={busy}>Save</button>
</form>
<button type="button" disabled={busy || (!tokenSet && token === '')} onclick={() => void test()}>Test connection</button>
