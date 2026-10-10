<script lang="ts">
  import { ApiError, acceptRequirements, getIntake, ingest, unreachable } from '$lib/api';
  import Live from '$lib/components/Live.svelte';
  import type { Intake } from '$lib/types';

  let intake = $state<Intake | null>(null);
  let message = $state('Loading intake');
  let busy = $state(false);

  async function load() {
    try {
      intake = await getIntake();
      if (intake.draftPending) message = 'Planner is drafting requirement ids';
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    }
  }

  $effect(() => {
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
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<h1>Intake</h1>
<Live {message} />
{#if intake?.planError}<p>{intake.planError}</p>{/if}
<button type="button" disabled={busy} onclick={() => void readDocs()}>Ingest</button>
<button class="primary" type="button" disabled={busy || intake?.draftPending} onclick={() => void accept()}>
  Accept unaccepted ids
</button>
{#if intake}
  {#each intake.requirements as requirement}
    <article>
      <h2>{requirement.id}</h2>
      <p>{requirement.accepted ? 'Accepted' : 'Not accepted'}</p>
      {#if requirement.sourceKind === 'draft'}<p>Drafted by the planner</p>{/if}
      <p>{requirement.statement}</p>
      <ul>
        {#each requirement.acceptance as line}
          <li>{line}</li>
        {/each}
      </ul>
      <p>{requirement.sourcePath}</p>
      {#each requirement.linkedCards as id}
        <a href={`/cards/${id}`}>{id}</a>
      {/each}
    </article>
  {/each}
  <h2>Decisions</h2>
  {#each intake.decisions as decision}
    <p>{decision.id}</p>
    <p>{decision.statement}</p>
  {/each}
{/if}
