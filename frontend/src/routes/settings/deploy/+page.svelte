<script lang="ts">
  import { session } from '$lib/session';
  import { getDeploy, putDeploy } from '$lib/api';
  import SecretField, { omitEmptySecret } from '$lib/components/SecretField.svelte';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { shell, showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';
  import Select from '$lib/ui/Select.svelte';

  const shown = [
    'devUrl',
    'healthUrl',
    'devHosts',
    'devCommand',
    'rollbackCommand',
    'deployToken',
    'localCheck',
    'devCheck',
    'allowExecutables',
    'commandExecutables',
    'sandboxEngine',
    'sandboxImage',
    'implementationCap',
    'prdPath',
    'rfcPath',
    'deployEnabled',
    'soloApproval'
  ];

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
  let commands = $state('');
  let sandboxEngine = $state('podman');
  let sandboxImage = $state('');
  let implementationCap = $state(2);
  let prdPath = $state('docs/prd');
  let rfcPath = $state('docs/rfc');
  let soloApproval = $state(false);
  let deployEnabled = $state(true);
  let loading = $state('Loading deploy settings');
  let loadError = $state('');
  let failure = $state(noFailure);
  let busy = $state(false);
  let canEdit = $state(true);
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: 'Deploy' }));

  function lines(value: string) {
    return value.split('\n').map((line) => line.trim()).filter(Boolean);
  }

  function load() {
    loading = 'Loading deploy settings';
    loadError = '';
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
        commands = body.commandExecutables.join('\n');
        sandboxEngine = body.sandboxEngine;
        sandboxImage = body.sandboxImage;
        implementationCap = body.implementationCap || 2;
        prdPath = body.prdPath;
        rfcPath = body.rfcPath;
        soloApproval = body.soloApproval;
        deployEnabled = body.deployEnabled;
        canEdit = body.canEdit;
      })
      .catch((error: unknown) => {
        loadError = failureText(error);
      })
      .finally(() => {
        loading = '';
      });
  }

  $effect(() => {
    if (shell.projectId === '') {
      if (session.project() === '') {
        loading = '';
        loadError = 'Choose a project in the sidebar first.';
      }
      return;
    }
    load();
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failure = noFailure;
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
        commandExecutables: lines(commands),
        sandboxEngine,
        sandboxImage,
        implementationCap,
        prdPath,
        rfcPath,
        soloApproval,
        deployEnabled,
        ...omitEmptySecret('deployToken', token, clear)
      });
      tokenSet = saved.deployTokenSet;
      canEdit = saved.canEdit;
      token = '';
      clear = false;
      toast.success('Deploy settings saved');
    } catch (error) {
      failure = formFailure(error, shown);
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title="Deploy"
  description="How this project reaches its dev server, which checks gate a card, and where intake reads its docs. Other projects keep their own deploy settings."
  {loading}
  error={loadError}
  onretry={load}
>
  <form onsubmit={save} bind:this={form} novalidate>
    <fieldset class="flex min-w-0 flex-col gap-6" disabled={!canEdit}>
      <FormAlert message={failure.message} extra={failure.extra} />
      <Panel title="Dev server" hint="Empty values can be saved. Deploy stops with deploy_config until they are set.">
        <div class="grid gap-5 sm:grid-cols-2">
          <Field label="Dev URL" id="dev-url" width="full" mono error={failure.fields.devUrl} bind:value={devUrl} />
          <Field label="Health URL" id="health" width="full" mono error={failure.fields.healthUrl} bind:value={healthUrl} />
        </div>
        <Field
          label="Dev hosts"
          id="hosts"
          type="textarea"
          width="lg"
          mono
          hint="One host per line. The dev URL and health URL hosts must be listed."
          error={failure.fields.devHosts}
          bind:value={devHosts}
        />
        <Field
          label="Deploy command"
          id="deploy-command"
          width="full"
          mono
          error={failure.fields.devCommand}
          hint="Absolute path to a script outside the target repo and the data directory. Not group or world writable."
          bind:value={devCommand}
        />
        <Field label="Rollback command" id="rollback" width="full" mono error={failure.fields.rollbackCommand} bind:value={rollbackCommand} />
        <SecretField label="Deploy token" name="deployToken" saved={tokenSet} error={failure.fields.deployToken} bind:value={token} bind:clear />
      </Panel>
      <Panel title="Checks" hint="A check passes when its command exits 0.">
        <div class="grid gap-5 sm:grid-cols-2">
          <Field label="Local check" id="local" width="full" mono error={failure.fields.localCheck} bind:value={localCheck} />
          <Field label="Dev check" id="dev-check" width="full" mono error={failure.fields.devCheck} bind:value={devCheck} />
        </div>
        <Field
          label="Allowed executables"
          id="allow"
          type="textarea"
          width="lg"
          mono
          error={failure.fields.allowExecutables}
          hint="One executable per line."
          bind:value={allow}
        />
        <Field
          label="Allowed commands"
          id="commands"
          type="textarea"
          width="lg"
          mono
          error={failure.fields.commandExecutables}
          hint="Programs the implementer may run with run_command, one bare name per line, such as go or npx. Each runs in the sandbox on a throwaway copy with no network, and never counts as a check. Shells and programs that run another program, such as bash, env, or xargs, are refused. Leave empty to turn run_command off."
          bind:value={commands}
        />
        <Select label="Sandbox engine" id="engine" width="sm" error={failure.fields.sandboxEngine} bind:value={sandboxEngine}>
          <option value="podman">Podman</option>
          <option value="docker">Docker</option>
        </Select>
        <Field
          label="Sandbox image"
          id="image"
          width="full"
          mono
          hint="Pin by digest, for example name@sha256:..."
          error={failure.fields.sandboxImage}
          bind:value={sandboxImage}
        />
      </Panel>
      <Panel title="Board">
        <div class="grid gap-5 sm:grid-cols-2">
          <Field
            label="Implementation cap"
            id="cap"
            type="number"
            width="sm"
            hint="Plan and implement jobs at once, 1 to 8"
            error={failure.fields.implementationCap}
            bind:value={implementationCap}
          />
          <Field label="PRD path" id="prd" width="full" mono error={failure.fields.prdPath} bind:value={prdPath} />
          <Field label="RFC path" id="rfc" width="full" mono error={failure.fields.rfcPath} bind:value={rfcPath} />
        </div>
        <Checkbox
          id="deploy-enabled"
          label="Deploy approved cards"
          hint="When off, an approved card gets Squash and merge instead of Deploy. The merge runs on GitHub with the GitHub account of the person who presses it."
          error={failure.fields.deployEnabled}
          bind:checked={deployEnabled}
        />
        <Checkbox
          id="solo"
          label="Solo approval"
          hint="The card owner, and a person whose GitHub login is the company bot, may approve. The approval is recorded as not independent. Leave off when a second person can review."
          error={failure.fields.soloApproval}
          bind:checked={soloApproval}
        />
      </Panel>
    </fieldset>
    <SaveBar
      {busy}
      disabled={!canEdit}
      note={canEdit ? '' : 'Only a Lead Developer, a tribe admin, or an admin can change these settings.'}
    />
  </form>
</Page>
