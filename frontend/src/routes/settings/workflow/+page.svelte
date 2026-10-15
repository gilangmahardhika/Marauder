<script lang="ts">
  import { session } from '$lib/session';
  import { getWorkflow, putWorkflow } from '$lib/api';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { shell, showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { WorkflowSettings } from '$lib/types';
  import ChoiceCards, { type Choice } from '$lib/ui/ChoiceCards.svelte';
  import { labelClass } from '$lib/ui/control';
  import FieldError from '$lib/ui/FieldError.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';

  const shown = ['implementationCap', 'soloApproval'];

  const capOptions = Array.from({ length: 8 }, (_, index) => ({ value: String(index + 1), label: String(index + 1) }));

  const approvalOptions: Choice[] = [
    {
      value: 'team',
      label: 'Second reviewer',
      description: 'Someone other than the card owner approves. The company bot login cannot approve.'
    },
    {
      value: 'solo',
      label: 'Solo',
      description:
        'The card owner, or someone signed in with the company bot login, may approve. The approval is recorded as not independent.'
    }
  ];

  let implementationCap = $state(2);
  let soloApproval = $state(false);
  let loading = $state('Loading workflow settings');
  let loadError = $state('');
  let failure = $state(noFailure);
  let busy = $state(false);
  let canEdit = $state(true);
  let form = $state<HTMLFormElement>();

  const capSummary = $derived(
    implementationCap === 1
      ? 'One card runs at a time. Others wait in the queue for a free slot.'
      : `Up to ${implementationCap} cards run at once. Others wait in the queue for a free slot.`
  );

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
      show(await putWorkflow({ implementationCap, soloApproval }));
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
      <Panel flush>
        <div class="divide-y divide-line">
          <div class="grid gap-4 p-5 md:grid-cols-[minmax(0,15rem)_1fr] md:gap-8">
            <div class="flex flex-col gap-1">
              <h2 class={labelClass}>Implementation cap</h2>
              <p class="text-xs text-muted">Plan and implement jobs that run at the same time in this project.</p>
            </div>
            <div class="flex min-w-0 flex-col gap-2">
              <div
                aria-invalid={failure.fields.implementationCap ? true : undefined}
                aria-describedby={failure.fields.implementationCap ? 'cap-error' : undefined}
              >
                <Segmented
                  label="Implementation cap"
                  options={capOptions}
                  value={String(implementationCap)}
                  onchange={(next) => (implementationCap = Number(next))}
                />
              </div>
              <p class="text-xs text-muted">{capSummary}</p>
              <FieldError id="cap" error={failure.fields.implementationCap} />
            </div>
          </div>
          <div class="grid gap-4 p-5 md:grid-cols-[minmax(0,15rem)_1fr] md:gap-8">
            <div class="flex flex-col gap-1">
              <h2 class={labelClass}>Approval</h2>
              <p class="text-xs text-muted">Who may approve a card's pull request.</p>
            </div>
            <div class="flex min-w-0 flex-col gap-2">
              <ChoiceCards
                id="solo"
                label="Solo approval"
                columns={2}
                options={approvalOptions}
                value={soloApproval ? 'solo' : 'team'}
                onchange={(next) => (soloApproval = next === 'solo')}
              />
              <FieldError id="solo" error={failure.fields.soloApproval} />
            </div>
          </div>
        </div>
      </Panel>
    </fieldset>
    <SaveBar
      {busy}
      disabled={!canEdit}
      note={canEdit ? '' : 'Only a Lead Developer, a tribe admin, or an admin can change these settings.'}
    />
  </form>
</Page>
