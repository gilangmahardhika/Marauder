<script lang="ts">
  import { ApiError, getDeploy, putDeploy, unreachable } from '$lib/api';
  import SecretField, { omitEmptySecret } from '$lib/components/SecretField.svelte';
  import { shell, showShell } from '$lib/shell.svelte';
  import Field from '$lib/ui/Field.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';
  import Select from '$lib/ui/Select.svelte';

  let devUrl = $state('');
  let devHosts = $state('');
  let devCommand = $state('');
  let rollbackCommand = $state('');
  let healthUrl = $state('');
  let token = $state('');
  let clear = $state(false);
  let tokenSet = $state(false);
  let localCheck = $state('');
  let devCheck = $state('');
  let allow = $state('');
  let sandboxEngine = $state('podman');
  let sandboxImage = $state('');
  let implementationCap = $state(2);
  let prdPath = $state('docs/prd');
  let rfcPath = $state('docs/rfc');
  let message = $state('Loading deploy settings');
  let fields = $state<Record<string, string>>({});
  let busy = $state(false);

  showShell(() => ({ title: 'Deploy', message }));

  function lines(value: string) {
    return value.split('\n').map((line) => line.trim()).filter(Boolean);
  }

  $effect(() => {
    if (shell.projectId === '') return;
    void getDeploy()
      .then((body) => {
        devUrl = body.devUrl;
        devHosts = body.devHosts.join('\n');
        devCommand = body.devCommand;
        rollbackCommand = body.rollbackCommand;
        healthUrl = body.healthUrl;
        tokenSet = body.deployTokenSet;
        localCheck = body.localCheck;
        devCheck = body.devCheck;
        allow = body.allowExecutables.join('\n');
        sandboxEngine = body.sandboxEngine;
        sandboxImage = body.sandboxImage;
        implementationCap = body.implementationCap || 2;
        prdPath = body.prdPath;
        rfcPath = body.rfcPath;
        message = 'Deploy settings';
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    message = 'Saving deploy settings';
    fields = {};
    try {
      const saved = await putDeploy({
        environment: 'dev',
        devUrl,
        devHosts: lines(devHosts),
        devCommand,
        rollbackCommand,
        healthUrl,
        localCheck,
        devCheck,
        allowExecutables: lines(allow),
        sandboxEngine,
        sandboxImage,
        implementationCap,
        prdPath,
        rfcPath,
        ...omitEmptySecret('deployToken', token, clear)
      });
      tokenSet = saved.deployTokenSet;
      token = '';
      clear = false;
      message = 'Deploy settings';
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page title="Deploy" description="How this project reaches its dev server, which checks gate a card, and where intake reads its docs. Other projects keep their own deploy settings. Only a Lead Developer can save.">
  <form onsubmit={save}>
    <div class="flex flex-col gap-6">
      <Panel title="Dev server" hint="Empty values can be saved. Deploy stops with deploy_config until they are set.">
        <div class="grid gap-5 sm:grid-cols-2">
          <Field label="Dev URL" id="dev-url" width="full" mono error={fields.devUrl} bind:value={devUrl} />
          <Field label="Health URL" id="health" width="full" mono error={fields.healthUrl} bind:value={healthUrl} />
        </div>
        <Field
          label="Dev hosts"
          id="hosts"
          type="textarea"
          width="lg"
          mono
          hint="One host per line. The dev URL and health URL hosts must be listed."
          bind:value={devHosts}
        />
        <Field
          label="Deploy command"
          id="deploy-command"
          width="full"
          mono
          error={fields.devCommand}
          hint="Absolute path to a script outside the target repo and the data directory. Not group or world writable."
          bind:value={devCommand}
        />
        <Field label="Rollback command" id="rollback" width="full" mono error={fields.rollbackCommand} bind:value={rollbackCommand} />
        <SecretField label="Deploy token" name="deployToken" saved={tokenSet} error={fields.deployToken} bind:value={token} bind:clear />
      </Panel>
      <Panel title="Checks" hint="A check passes when its command exits 0.">
        <div class="grid gap-5 sm:grid-cols-2">
          <Field label="Local check" id="local" width="full" mono bind:value={localCheck} />
          <Field label="Dev check" id="dev-check" width="full" mono bind:value={devCheck} />
        </div>
        <Field label="Allowed executables" id="allow" type="textarea" width="lg" mono hint="One executable per line." bind:value={allow} />
        <Select label="Sandbox engine" id="engine" width="sm" bind:value={sandboxEngine}>
          <option value="podman">Podman</option>
          <option value="docker">Docker</option>
        </Select>
        <Field label="Sandbox image" id="image" width="full" mono hint="Pin by digest, for example name@sha256:..." bind:value={sandboxImage} />
      </Panel>
      <Panel title="Board">
        <div class="grid gap-5 sm:grid-cols-2">
          <Field label="Implementation cap" id="cap" type="number" width="sm" hint="Plan and implement jobs at once, 1 to 8" bind:value={implementationCap} />
          <Field label="PRD path" id="prd" width="full" mono bind:value={prdPath} />
          <Field label="RFC path" id="rfc" width="full" mono bind:value={rfcPath} />
        </div>
      </Panel>
    </div>
    <SaveBar {busy} />
  </form>
</Page>
