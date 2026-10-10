<script lang="ts">
  import { ApiError, getDeploy, putDeploy, unreachable } from '$lib/api';
  import Live from '$lib/components/Live.svelte';
  import SecretField, { omitEmptySecret } from '$lib/components/SecretField.svelte';

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
  let operatorLogin = $state('');
  let message = $state('Loading deploy settings');
  let fields = $state<Record<string, string>>({});
  let busy = $state(false);

  function lines(value: string) {
    return value.split('\n').map((line) => line.trim()).filter(Boolean);
  }

  $effect(() => {
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
        operatorLogin = body.operatorLogin;
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
        operatorLogin,
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

<h1>Deploy</h1>
<Live {message} />
<form class="form" onsubmit={save}>
  <fieldset>
    <legend>Empty values can be saved. Deploy stops with deploy_config until they are set.</legend>
    <label for="dev-url">Dev URL <input id="dev-url" bind:value={devUrl} /></label>
    <label for="hosts">Dev hosts <textarea id="hosts" bind:value={devHosts}></textarea></label>
    <p>Health URL and dev URL hosts must be listed here.</p>
    <label for="deploy-command">Deploy command <input id="deploy-command" bind:value={devCommand} /></label>
    {#if fields.devCommand}<p class="error">{fields.devCommand}</p>{/if}
    <label for="rollback">Rollback command <input id="rollback" bind:value={rollbackCommand} /></label>
    <p>Absolute path to a script outside the target repo and the data directory. Not group or world writable.</p>
    <label for="health">Health URL <input id="health" bind:value={healthUrl} /></label>
    <SecretField label="Deploy token" name="deployToken" saved={tokenSet} bind:value={token} bind:clear />
  </fieldset>
  <fieldset>
    <legend>Checks</legend>
    <label for="local">Local check <input id="local" bind:value={localCheck} /></label>
    <label for="dev-check">Dev check <input id="dev-check" bind:value={devCheck} /></label>
    <label for="allow">Allowed executables <textarea id="allow" bind:value={allow}></textarea></label>
    <label for="engine">
      Sandbox engine
      <select id="engine" bind:value={sandboxEngine}>
        <option value="podman">podman</option>
        <option value="docker">docker</option>
      </select>
    </label>
    <label for="image">Sandbox image <input id="image" bind:value={sandboxImage} /></label>
    <p>Pin by digest, for example name@sha256:...</p>
  </fieldset>
  <fieldset>
    <legend>Board</legend>
    <label for="cap">Implementation cap <input id="cap" type="number" bind:value={implementationCap} /></label>
    <label for="prd">PRD path <input id="prd" bind:value={prdPath} /></label>
    <label for="rfc">RFC path <input id="rfc" bind:value={rfcPath} /></label>
    <label for="operator">Operator login <input id="operator" bind:value={operatorLogin} /></label>
  </fieldset>
  <button class="primary" type="submit" disabled={busy}>Save</button>
</form>
