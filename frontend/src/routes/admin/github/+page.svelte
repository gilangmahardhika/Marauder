<script lang="ts">
  import { getCompanyGitHub, putCompanyGitHub, testCompanyGitHub } from '$lib/api';
  import SecretField, { omitEmptySecret } from '$lib/components/SecretField.svelte';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { labelClass } from '$lib/ui/control';
  import Field from '$lib/ui/Field.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  const shown = ['host', 'org', 'token'];

  let host = $state('github.com');
  let org = $state('');
  let botLogin = $state('');
  let token = $state('');
  let clear = $state(false);
  let tokenSet = $state(false);
  let loading = $state('Loading company GitHub');
  let loadError = $state('');
  let result = $state('');
  let hint = $state('');
  let saving = $state(false);
  let testing = $state(false);
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: 'Company GitHub' }));

  function load() {
    loading = 'Loading company GitHub';
    loadError = '';
    void getCompanyGitHub()
      .then((body) => {
        host = body.host;
        org = body.org;
        botLogin = body.botLogin;
        tokenSet = body.tokenSet;
      })
      .catch((error: unknown) => {
        loadError = failureText(error);
      })
      .finally(() => {
        loading = '';
      });
  }

  $effect(() => {
    load();
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    saving = true;
    failure = noFailure;
    try {
      const saved = await putCompanyGitHub({ host, org, ...omitEmptySecret('token', token, clear) });
      tokenSet = saved.tokenSet;
      botLogin = saved.botLogin;
      token = '';
      clear = false;
      result = '';
      hint = '';
      toast.success('Company GitHub saved');
    } catch (error) {
      failure = formFailure(error, shown);
      void focusInvalid(form);
    } finally {
      saving = false;
    }
  }

  async function test() {
    testing = true;
    hint = '';
    try {
      const outcome = await testCompanyGitHub();
      if (outcome.ok) {
        botLogin = outcome.login ?? botLogin;
        const count = outcome.repoCount ?? 0;
        result = org
          ? `Signed in as ${botLogin}. Sees ${count} ${count === 1 ? 'repository' : 'repositories'} in ${org} (${outcome.privateCount ?? 0} private).`
          : `Signed in as ${botLogin}`;
        hint = outcome.hint ?? '';
      } else {
        result = outcome.message ?? 'GitHub refused the token';
      }
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      testing = false;
    }
  }
</script>

<Page
  title="Company GitHub"
  description="One account for the whole company. Every project clones, pushes, and opens pull requests through it."
  {loading}
  error={loadError}
  onretry={load}
>
  <form onsubmit={save} bind:this={form} novalidate>
    <div class="flex flex-col gap-6">
      <FormAlert message={failure.message} extra={failure.extra} />
      <Panel title="Organization" hint="Each project picks one repository from this organization.">
        <div class="grid gap-5 sm:grid-cols-2">
          <Field label="Host" id="host" width="full" mono error={failure.fields.host} bind:value={host} />
          <Field label="Organization" id="org" width="full" mono error={failure.fields.org} bind:value={org} />
        </div>
      </Panel>
      <Panel title="Bot account" hint="A machine user in the organization. Its login cannot approve cards.">
        <SecretField label="Token" name="token" width="lg" saved={tokenSet} error={failure.fields.token} bind:value={token} bind:clear />
        <div class="flex flex-col gap-1.5">
          <p class={labelClass}>Bot login</p>
          <p class="font-mono text-sm {botLogin ? 'text-ink' : 'text-muted'}">
            {botLogin || 'Unknown until Test connection succeeds'}
          </p>
        </div>
        {#if hint}<Notice tone="warning">{hint}</Notice>{/if}
        {#snippet footer()}
          {#if result || !tokenSet}
            <span class="mr-auto text-sm text-muted">{result || 'Save a token, then test the connection.'}</span>
          {/if}
          <Button
            type="button"
            variant="secondary"
            icon="zap"
            loading={testing}
            disabled={saving || !tokenSet}
            onclick={() => void test()}
          >
            Test connection
          </Button>
          <Button variant="primary" type="submit" icon="save" loading={saving} disabled={testing}>Save</Button>
        {/snippet}
      </Panel>
    </div>
  </form>
</Page>
