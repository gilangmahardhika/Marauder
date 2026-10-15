<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { deleteRequirement, getIntake, streamRefine, updateRequirement } from '$lib/api';
  import MentionList from '$lib/components/MentionList.svelte';
  import RequirementForm from '$lib/components/RequirementForm.svelte';
  import ReviewersPanel from '$lib/components/ReviewersPanel.svelte';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { refineModels } from '$lib/llm';
  import { fromLines, toLines } from '$lib/requirements';
  import { shell, showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { Intake, MCPServerMention, ModelChoice, Requirement, Reviewer } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  const shown = ['id', 'statement', 'acceptance'];

  let requirement = $state<Requirement | null>(null);
  let reviewerOptions = $state<Reviewer[]>([]);
  let servers = $state<MCPServerMention[]>([]);
  let models = $state<ModelChoice[]>([]);
  let implementModel = $state('');
  let missing = $state(false);
  let statement = $state('');
  let acceptance = $state('');
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();
  let confirming = $state(false);
  let loading = $state('Loading requirement');
  let loadError = $state('');
  let saving = $state(false);
  let deleting = $state(false);

  const requirementId = $derived($page.params.id ?? '');

  showShell(() => ({ title: requirementId || 'Requirement' }));

  function pick(intake: Intake) {
    const found = intake.requirements.find((item) => item.id === requirementId) ?? null;
    requirement = found;
    reviewerOptions = intake.reviewerOptions;
    servers = intake.mcpServers ?? [];
    missing = !found;
    if (found) {
      statement = found.statement;
      acceptance = fromLines(found.acceptance);
    }
  }

  function load() {
    loading = 'Loading requirement';
    loadError = '';
    void getIntake()
      .then(pick)
      .catch((error: unknown) => {
        loadError = failureText(error);
      })
      .finally(() => {
        loading = '';
      });
  }

  $effect(() => {
    if (requirementId === '' || shell.projectId === '') return;
    load();
  });

  onMount(() => {
    void refineModels().then((loaded) => {
      models = loaded.models;
      implementModel = loaded.implementModel;
    });
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    saving = true;
    failure = noFailure;
    const wasAccepted = Boolean(requirement?.accepted);
    try {
      const saved = await updateRequirement(requirementId, { statement, acceptance: toLines(acceptance) });
      pick(saved);
      toast.success(wasAccepted && !requirement?.accepted ? 'Requirement saved. It needs acceptance again.' : 'Requirement saved');
    } catch (error) {
      failure = formFailure(error, shown);
      void focusInvalid(form);
    } finally {
      saving = false;
    }
  }

  async function remove() {
    deleting = true;
    try {
      await deleteRequirement(requirementId);
      toast.success('Requirement deleted');
      await goto('/intake');
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      deleting = false;
      confirming = false;
    }
  }
</script>

{#if missing}
  <Page title="Requirement not found">
    <EmptyState title={`There is no requirement ${requirementId} in this project.`} href="/intake" link="Intake" />
  </Page>
{:else if requirement && requirement.sourceKind !== 'manual'}
  <Page title={requirementId}>
    {#snippet actions()}
      <Button href="/intake" icon="arrow-left">Intake</Button>
    {/snippet}
    <div class="flex flex-col gap-6">
      <EmptyState
        title="Edit this requirement in the repository."
        description={requirement.sourcePath
          ? `It comes from ${requirement.sourcePath}. Change the file, then run Ingest.`
          : 'The planner drafted it. Accept it or replace it in the PRD.'}
      />
      {#if (requirement.mentions ?? []).length > 0}
        <Panel title="MCP servers" hint="Cards of this requirement offer only the servers it mentions.">
          <MentionList mentions={requirement.mentions ?? []} {servers} />
        </Panel>
      {/if}
      <ReviewersPanel {requirement} options={reviewerOptions} onsaved={pick} />
    </div>
  </Page>
{:else if requirement}
  <Page title={requirementId}>
    {#snippet meta()}
      <Badge>Manual</Badge>
      <Badge tone={requirement!.accepted ? 'green' : 'default'}>{requirement!.accepted ? 'Accepted' : 'Not accepted'}</Badge>
    {/snippet}
    {#snippet actions()}
      <Button href="/intake" icon="arrow-left">Intake</Button>
    {/snippet}
    <div class="flex flex-col gap-6">
      <form onsubmit={save} bind:this={form} novalidate>
        <Panel title="Requirement" hint="Changing the text, by hand or with the refine buttons, clears acceptance when you save.">
          <FormAlert message={failure.message} extra={failure.extra} />
          <RequirementForm
            id={requirementId}
            idLocked
            bind:statement
            bind:acceptance
            fields={failure.fields}
            {servers}
            {models}
            {implementModel}
            refine={(body, onEvent, signal) => streamRefine({ ...body, requirementId }, onEvent, signal)}
          />
          {#snippet footer()}
            <Button variant="primary" type="submit" icon="save" loading={saving} disabled={deleting}>Save</Button>
          {/snippet}
        </Panel>
      </form>
      <ReviewersPanel requirement={requirement!} options={reviewerOptions} onsaved={pick} />
      <Panel title="Delete requirement" hint="A requirement that a card lists cannot be deleted.">
        {#if confirming}
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm text-ink">Delete {requirementId}?</span>
            <Button type="button" variant="ghost" disabled={deleting} onclick={() => (confirming = false)}>Cancel</Button>
            <Button type="button" variant="danger" icon="trash" loading={deleting} onclick={() => void remove()}>Delete</Button>
          </div>
        {:else}
          <Button type="button" variant="danger" icon="trash" disabled={saving} onclick={() => (confirming = true)}>Delete requirement</Button>
        {/if}
      </Panel>
    </div>
  </Page>
{:else}
  <Page title={requirementId || 'Requirement'} {loading} error={loadError} onretry={load}>
    <EmptyState title={`There is no requirement ${requirementId} in this project.`} href="/intake" link="Intake" />
  </Page>
{/if}
