<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { addRequirement, getIntake, setReviewers, streamRefine } from '$lib/api';
  import AIReviewModeField from '$lib/components/AIReviewModeField.svelte';
  import RequirementForm from '$lib/components/RequirementForm.svelte';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { refineModels } from '$lib/llm';
  import { canSetSelfReview, reviewerChoices, toLines } from '$lib/requirements';
  import { shell, showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { AIReviewMode, MCPServerMention, ModelChoice, Reviewer } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import MultiSearchSelect from '$lib/ui/MultiSearchSelect.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let id = $state('');
  let statement = $state('');
  let acceptance = $state('');
  let reviewers = $state<string[]>([]);
  let selfReview = $state(false);
  let aiReviewMode = $state<AIReviewMode>('continue');
  let options = $state<Reviewer[]>([]);
  let known = $state<string[]>([]);
  let servers = $state<MCPServerMention[]>([]);
  let models = $state<ModelChoice[]>([]);
  let implementModel = $state('');
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();
  let busy = $state(false);

  const choices = $derived(reviewerChoices(options, shell.email));
  const mayChooseSelfReview = $derived(canSetSelfReview(shell.role, shell.admin));
  const shown = $derived([
    'id',
    'statement',
    'acceptance',
    'userIds',
    ...(mayChooseSelfReview ? ['selfReview', 'aiReviewMode'] : [])
  ]);

  showShell(() => ({ title: 'New requirement' }));

  $effect(() => {
    if (shell.projectId === '') return;
    void getIntake()
      .then((body) => {
        options = body.reviewerOptions;
        known = body.requirements.map((requirement) => requirement.id);
        servers = body.mcpServers ?? [];
      })
      .catch(() => {
        options = [];
      });
  });

  onMount(() => {
    void refineModels().then((loaded) => {
      models = loaded.models;
      implementModel = loaded.implementModel;
    });
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failure = noFailure;
    if (statement.trim() === '') {
      failure = { ...noFailure, fields: { statement: 'Enter a statement.' } };
      void focusInvalid(form);
      return;
    }
    busy = true;
    let added = '';
    try {
      const body = await addRequirement({
        id: id.trim(),
        statement,
        acceptance: toLines(acceptance),
        selfReview: mayChooseSelfReview && selfReview,
        aiReviewMode: mayChooseSelfReview ? aiReviewMode : 'continue'
      });
      added = id.trim() || (body.requirements.find((requirement) => !known.includes(requirement.id))?.id ?? '');
      if (reviewers.length > 0 && added !== '') await setReviewers(added, reviewers);
      toast.success('Requirement added');
      await goto('/intake');
    } catch (error) {
      if (added !== '') {
        toast.error(`${added} was added, but its reviewers were not saved. ${failureText(error)}`);
        await goto(`/intake/${encodeURIComponent(added)}`);
        return;
      }
      failure = formFailure(error, shown);
      void focusInvalid(form);
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title="New requirement"
  description="Added to this project's intake. It is not accepted until a senior member accepts it."
>
  <form onsubmit={submit} bind:this={form} novalidate>
    <Panel title="Requirement">
      <FormAlert message={failure.message} extra={failure.extra} />
      <RequirementForm
        bind:id
        bind:statement
        bind:acceptance
        fields={failure.fields}
        {servers}
        {models}
        {implementModel}
        refine={(body, onEvent, signal) => streamRefine({ ...body, requirementId: id.trim() }, onEvent, signal)}
      />
      <MultiSearchSelect
        label="Reviewers"
        id="requirement-reviewers"
        width="lg"
        options={choices}
        placeholder={choices.length === 0 ? 'No other tribe members yet' : 'Search by name, login, or email'}
        max={15}
        error={failure.fields.userIds}
        bind:values={reviewers}
      />
      <p class="text-xs text-muted">Someone without a verified GitHub login is asked to review on GitHub after they verify.</p>
      {#if mayChooseSelfReview}
        <Checkbox
          id="requirement-self-review"
          label="Self review"
          hint="You review and approve this requirement's cards yourself. The approval is recorded as not independent."
          error={failure.fields.selfReview}
          bind:checked={selfReview}
        />
        <AIReviewModeField error={failure.fields.aiReviewMode} bind:value={aiReviewMode} />
      {/if}
      {#snippet footer()}
        <Button variant="ghost" href="/intake">Cancel</Button>
        <Button variant="primary" type="submit" icon="plus" loading={busy}>Add requirement</Button>
      {/snippet}
    </Panel>
  </form>
</Page>
