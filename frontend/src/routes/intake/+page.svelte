<script lang="ts">
  import { ApiError, acceptRequirements, getIntake, ingest, unreachable } from '$lib/api';
  import IntakeActions from '$lib/components/IntakeActions.svelte';
  import { shell, showShell } from '$lib/shell.svelte';
  import type { Intake } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Notice from '$lib/ui/Notice.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let intake = $state<Intake | null>(null);
  let skipped = $state<string[]>([]);
  let message = $state('Loading intake');
  let busy = $state(false);

  async function load() {
    try {
      intake = await getIntake();
      if (intake.draftPending) message = 'Planner is drafting requirement ids';
      else if (message === 'Loading intake') message = 'Intake';
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    }
  }

  $effect(() => {
    if (shell.projectId === '') return;
    void load();
  });

  $effect(() => {
    if (!intake?.draftPending) return;
    const timer = setInterval(() => {
      void load();
    }, 3000);
    return () => clearInterval(timer);
  });

  async function readDocs() {
    busy = true;
    message = 'Reading docs/prd and docs/rfc';
    try {
      intake = await ingest();
      skipped = intake.skipped ?? [];
      message = 'Intake';
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  async function accept() {
    if (!intake) return;
    busy = true;
    message = 'Saving acceptance';
    try {
      intake = await acceptRequirements(intake.requirements.filter((item) => !item.accepted).map((item) => item.id));
      message = 'Intake';
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  const accepted = $derived(intake?.requirements.filter((item) => item.accepted).length ?? 0);

  showShell(() => ({ title: 'Intake', message }));
</script>

<Page
  title="Intake"
  description="Requirements read from the PRD and RFCs, or added by hand, and the decisions from the RFCs. Cards are built only from accepted requirements."
>
  {#snippet meta()}
    {#if intake && intake.requirements.length > 0}
      <Badge tone={accepted === intake.requirements.length ? 'green' : 'default'}>
        {accepted} of {intake.requirements.length} accepted
      </Badge>
    {/if}
  {/snippet}
  {#snippet actions()}
    <IntakeActions {intake} {busy} oningest={() => void readDocs()} onaccept={() => void accept()} />
  {/snippet}
  <div class="flex flex-col gap-4">
    {#if intake?.planError}<Notice tone="danger">{intake.planError}</Notice>{/if}
    {#if intake?.draftPending}<Notice>The planner is drafting requirement ids. This page refreshes on its own.</Notice>{/if}
    {#if skipped.length > 0}
      <Notice>
        {skipped.join(', ')}
        {skipped.length === 1 ? 'is' : 'are'} also in the repository. The text added by hand was kept.
      </Notice>
    {/if}
    {#if intake && !intake.canAccept && intake.requirements.some((item) => !item.accepted)}
      <Notice>A Lead Developer, Senior Developer, QA Lead, or Product Owner accepts requirements.</Notice>
    {/if}
    {#if intake && intake.requirements.length === 0}
      <EmptyState
        title="No requirements yet"
        description="Ingest reads the PRD and RFCs for this project, or add a requirement by hand."
        href="/intake/new"
        link="Add requirement"
      />
    {/if}
    {#each intake?.requirements ?? [] as requirement (requirement.id)}
      <Panel title={requirement.id} mono>
        {#snippet actions()}
          {#if requirement.sourceKind === 'draft'}<Badge tone="amber">Draft</Badge>{/if}
          {#if requirement.sourceKind === 'manual'}<Badge>Manual</Badge>{/if}
          <Badge tone={requirement.accepted ? 'green' : 'default'}>{requirement.accepted ? 'Accepted' : 'Not accepted'}</Badge>
        {/snippet}
        <p class="text-sm leading-relaxed text-ink">{requirement.statement}</p>
        {#if requirement.acceptance.length > 0}
          <ul class="flex flex-col gap-1.5">
            {#each requirement.acceptance as line}
              <li class="flex items-start gap-2 text-sm text-ink">
                <Icon name="check" class="mt-0.5 size-4 shrink-0 text-muted" />
                {line}
              </li>
            {/each}
          </ul>
        {/if}
        {#snippet footer()}
          {#if requirement.sourceKind === 'manual'}
            <span class="mr-auto text-xs text-muted">Added by {requirement.createdBy ?? 'a former user'}</span>
            <a
              class="inline-flex min-h-11 items-center px-1 text-sm font-medium text-accent hover:underline"
              href={`/intake/${encodeURIComponent(requirement.id)}`}
              aria-label={`Edit ${requirement.id}`}>Edit</a
            >
          {:else}
            <span class="mr-auto font-mono text-xs text-muted">{requirement.sourcePath}</span>
          {/if}
          {#each requirement.linkedCards as id}
            <a class="inline-flex min-h-11 items-center px-1 font-mono text-xs text-accent underline" href={`/cards/${id}`}>{id}</a>
          {/each}
        {/snippet}
      </Panel>
    {/each}
    {#if intake && intake.decisions.length > 0}
      <Panel title="Decisions" flush>
        <ul class="divide-y divide-line">
          {#each intake.decisions as decision (decision.id)}
            <li class="flex flex-col gap-1 px-5 py-3.5 sm:flex-row sm:gap-6">
              <span class="w-28 shrink-0 font-mono text-xs leading-5 text-muted">{decision.id}</span>
              <span class="text-sm text-ink">{decision.statement}</span>
            </li>
          {/each}
        </ul>
      </Panel>
    {/if}
  </div>
</Page>
