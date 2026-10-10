<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { ApiError, deleteRequirement, getIntake, unreachable, updateRequirement } from '$lib/api';
  import RequirementForm from '$lib/components/RequirementForm.svelte';
  import ReviewersPanel from '$lib/components/ReviewersPanel.svelte';
  import { fromLines, toLines } from '$lib/requirements';
  import { shell, showShell } from '$lib/shell.svelte';
  import type { Intake, Requirement, Reviewer } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let requirement = $state<Requirement | null>(null);
  let reviewerOptions = $state<Reviewer[]>([]);
  let missing = $state(false);
  let statement = $state('');
  let acceptance = $state('');
  let fields = $state<Record<string, string>>({});
  let confirming = $state(false);
  let message = $state('Loading requirement');
  let busy = $state(false);

  const requirementId = $derived($page.params.id ?? '');

  showShell(() => ({ title: requirementId || 'Requirement', message }));

  function pick(intake: Intake) {
    const found = intake.requirements.find((item) => item.id === requirementId) ?? null;
    requirement = found;
    reviewerOptions = intake.reviewerOptions;
    missing = !found;
    if (found) {
      statement = found.statement;
      acceptance = fromLines(found.acceptance);
    }
  }

  $effect(() => {
    const id = requirementId;
    if (id === '' || shell.projectId === '') return;
    void getIntake()
      .then((body) => {
        pick(body);
        message = missing ? 'Requirement not found' : id;
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    fields = {};
    message = 'Saving requirement';
    const wasAccepted = Boolean(requirement?.accepted);
    try {
      const saved = await updateRequirement(requirementId, { statement, acceptance: toLines(acceptance) });
      pick(saved);
      message = wasAccepted && !requirement?.accepted ? 'Requirement saved. It needs acceptance again.' : 'Requirement saved';
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  async function remove() {
    busy = true;
    message = 'Deleting requirement';
    try {
      await deleteRequirement(requirementId);
      message = 'Requirement deleted';
      await goto('/intake');
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
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
      <Button href="/intake">Intake</Button>
    {/snippet}
    <div class="flex flex-col gap-6">
      <EmptyState
        title="Edit this requirement in the repository."
        description={requirement.sourcePath
          ? `It comes from ${requirement.sourcePath}. Change the file, then run Ingest.`
          : 'The planner drafted it. Accept it or replace it in the PRD.'}
      />
      <ReviewersPanel {requirement} options={reviewerOptions} onsaved={pick} onmessage={(text) => (message = text)} />
    </div>
  </Page>
{:else if requirement}
  <Page title={requirementId}>
    {#snippet meta()}
      <Badge>Manual</Badge>
      <Badge tone={requirement!.accepted ? 'green' : 'default'}>{requirement!.accepted ? 'Accepted' : 'Not accepted'}</Badge>
    {/snippet}
    {#snippet actions()}
      <Button href="/intake">Intake</Button>
    {/snippet}
    <div class="flex flex-col gap-6">
      <form onsubmit={save}>
        <Panel title="Requirement" hint="Changing the text clears acceptance.">
          <RequirementForm id={requirementId} idLocked bind:statement bind:acceptance {fields} />
          {#snippet footer()}
            <Button variant="primary" type="submit" disabled={busy}>Save</Button>
          {/snippet}
        </Panel>
      </form>
      <ReviewersPanel requirement={requirement!} options={reviewerOptions} onsaved={pick} onmessage={(text) => (message = text)} />
      <Panel title="Delete requirement" hint="A requirement that a card lists cannot be deleted.">
        {#if confirming}
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm text-ink">Delete {requirementId}?</span>
            <Button type="button" variant="ghost" disabled={busy} onclick={() => (confirming = false)}>Cancel</Button>
            <Button type="button" variant="danger" disabled={busy} onclick={() => void remove()}>Delete</Button>
          </div>
        {:else}
          <Button type="button" variant="danger" disabled={busy} onclick={() => (confirming = true)}>Delete requirement</Button>
        {/if}
      </Panel>
    </div>
  </Page>
{/if}
