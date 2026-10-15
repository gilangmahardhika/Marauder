<script lang="ts">
  import { session } from '$lib/session';
  import { getWorkflow, putWorkflow } from '$lib/api';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { shell, showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { WorkflowSettings } from '$lib/types';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';

  const shown = ['implementationCap', 'soloApproval'];

  let implementationCap = $state(2);
  let soloApproval = $state(false);
  let loading = $state('Loading workflow settings');
  let loadError = $state('');
  let failure = $state(noFailure);
  let busy = $state(false);
  let canEdit = $state(true);
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: 'Workflow' }));

  function show(body: WorkflowSettings) {
    implementationCap = body.implementationCap || 2;
    soloApproval = body.soloApproval;
    canEdit = body.canEdit;
  }

  function load() {
    loading = 'Loading workflow settings';
    loadError = '';
    void getWorkflow()
      .then(show)
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
      show(await putWorkflow({ implementationCap: Number(implementationCap), soloApproval }));
      toast.success('Workflow settings saved');
    } catch (error) {
      failure = formFailure(error, shown);
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title="Workflow"
  description="How many cards this project works on at once, and who may approve them. Other projects keep their own workflow settings."
  {loading}
  error={loadError}
  onretry={load}
>
  <form onsubmit={save} bind:this={form} novalidate>
    <fieldset class="flex min-w-0 flex-col gap-6" disabled={!canEdit}>
      <FormAlert message={failure.message} extra={failure.extra} />
      <Panel title="Jobs">
        <Field
          label="Implementation cap"
          id="cap"
          type="number"
          width="sm"
          hint="Plan and implement jobs that run at once, 1 to 8. A queued card waits for a free slot."
          error={failure.fields.implementationCap}
          bind:value={implementationCap}
        />
      </Panel>
      <Panel title="Approval">
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
