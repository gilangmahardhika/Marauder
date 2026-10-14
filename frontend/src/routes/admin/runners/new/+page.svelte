<script lang="ts">
  import { apiOrigin, mintRunnerToken } from '$lib/api';
  import { stamp } from '$lib/format';
  import { focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { installCommand } from '$lib/runners';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { RunnerToken } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let origin = $state(apiOrigin);
  let name = $state('');
  let slots = $state<string | number>(2);
  let minted = $state<RunnerToken | null>(null);
  let busy = $state(false);
  let copied = $state(false);
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: 'Add runner' }));

  const slotCount = $derived(Math.min(16, Math.max(1, Math.round(Number(slots)) || 1)));
  const command = $derived(minted ? installCommand(origin, minted.token, name, slotCount) : '');

  async function mint(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    copied = false;
    failure = noFailure;
    try {
      minted = await mintRunnerToken();
      toast.success('Registration token created');
    } catch (error) {
      failure = formFailure(error);
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(command);
      copied = true;
      toast.success('Install command copied');
    } catch {
      toast.error('Select the command and copy it.');
    }
  }
</script>

<Page title="Add runner" description="Run the install command on a Linux machine with Podman. The runner registers itself and shows up on the Runners page.">
  {#snippet actions()}
    <Button variant="ghost" href="/admin/runners/guide">Runner guide</Button>
  {/snippet}
  <div class="flex flex-col gap-6">
    <form onsubmit={mint} bind:this={form} novalidate>
      <Panel title="Runner">
        <FormAlert message={failure.message} extra={failure.extra} />
        <Field label="Control plane URL" id="origin" hint="The HTTPS address the runner machine uses to reach this server." bind:value={origin} width="lg" />
        <Field label="Name" id="name" hint="Shown on the Runners page. Defaults to the machine's host name." bind:value={name} />
        <Field label="Slots" id="slots" type="number" hint="How many implement jobs it runs at once, 1 to 16." bind:value={slots} width="sm" />
        <div class="mt-2">
          <Button type="submit" variant="primary" icon="plus" loading={busy}>{minted ? 'Make a new token' : 'Create install command'}</Button>
        </div>
      </Panel>
    </form>

    {#if minted}
      <Panel title="Install command" hint={`The token works once and expires ${stamp(minted.expiresAt)}. It is not shown again after you leave this page.`}>
        <div class="flex flex-col gap-3">
          <pre class="overflow-x-auto rounded-control border border-line bg-ground p-3 font-mono text-xs whitespace-pre-wrap break-all text-ink">{command}</pre>
          <div class="flex flex-wrap items-center gap-2">
            <Button type="button" variant="secondary" icon={copied ? 'check' : 'copy'} onclick={() => void copy()}>{copied ? 'Copied' : 'Copy command'}</Button>
            <Button type="button" variant="ghost" icon="arrow-left" href="/admin/runners">Back to runners</Button>
          </div>
          <Notice>The script installs the orchestrator binary, a systemd service, and a config file readable only by the service user.</Notice>
        </div>
      </Panel>
    {/if}
  </div>
</Page>
