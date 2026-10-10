<script lang="ts">
  import { ApiError, getCompanyGitHub, putCompanyGitHub, testCompanyGitHub, unreachable } from '$lib/api';
  import SecretField, { omitEmptySecret } from '$lib/components/SecretField.svelte';
  import { showShell } from '$lib/shell.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { labelClass } from '$lib/ui/control';
  import Field from '$lib/ui/Field.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let host = $state('github.com');
  let org = $state('');
  let botLogin = $state('');
  let token = $state('');
  let clear = $state(false);
  let tokenSet = $state(false);
  let message = $state('Loading company GitHub');
  let result = $state('');
  let busy = $state(false);
  let fields = $state<Record<string, string>>({});

  showShell(() => ({ title: 'Company GitHub', message }));

  $effect(() => {
    void getCompanyGitHub()
      .then((body) => {
        host = body.host;
        org = body.org;
        botLogin = body.botLogin;
        tokenSet = body.tokenSet;
        message = 'Company GitHub';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    message = 'Saving company GitHub';
    fields = {};
    try {
      const saved = await putCompanyGitHub({ host, org, ...omitEmptySecret('token', token, clear) });
      tokenSet = saved.tokenSet;
      botLogin = saved.botLogin;
      token = '';
      clear = false;
      result = '';
      message = 'Company GitHub saved';
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  async function test() {
    busy = true;
    message = 'Testing connection';
    try {
      const outcome = await testCompanyGitHub();
      if (outcome.ok) {
        botLogin = outcome.login ?? botLogin;
        result = `Signed in as ${botLogin}`;
      } else {
        result = outcome.message ?? 'GitHub refused the token';
      }
      message = result;
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title="Company GitHub"
  description="One account for the whole company. Every project clones, pushes, and opens pull requests through it."
>
  <form onsubmit={save}>
    <div class="flex flex-col gap-6">
      <Panel title="Organization" hint="Each project picks one repository from this organization.">
        <div class="grid gap-5 sm:grid-cols-2">
          <Field label="Host" id="host" width="full" mono error={fields.host} bind:value={host} />
          <Field label="Organization" id="org" width="full" mono error={fields.org} bind:value={org} />
        </div>
      </Panel>
      <Panel title="Bot account" hint="A machine user in the organization. Its login cannot approve cards.">
        <SecretField label="Token" name="token" width="lg" saved={tokenSet} error={fields.token} bind:value={token} bind:clear />
        <div class="flex flex-col gap-1.5">
          <p class={labelClass}>Bot login</p>
          <p class="font-mono text-sm {botLogin ? 'text-ink' : 'text-muted'}">
            {botLogin || 'Unknown until Test connection succeeds'}
          </p>
        </div>
        {#snippet footer()}
          {#if result || !tokenSet}
            <span class="mr-auto text-sm text-muted">{result || 'Save a token, then test the connection.'}</span>
          {/if}
          <Button type="button" variant="secondary" disabled={busy || !tokenSet} onclick={() => void test()}>
            Test connection
          </Button>
          <Button variant="primary" type="submit" disabled={busy}>Save</Button>
        {/snippet}
      </Panel>
    </div>
  </form>
</Page>
