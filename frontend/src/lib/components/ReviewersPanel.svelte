<script lang="ts">
  import { ApiError, setReviewers, unreachable } from '$lib/api';
  import { reviewerChoices } from '$lib/requirements';
  import type { Intake, Requirement, Reviewer } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import MultiSearchSelect from '$lib/ui/MultiSearchSelect.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let {
    requirement,
    options,
    onsaved,
    onmessage
  }: {
    requirement: Requirement;
    options: Reviewer[];
    onsaved: (intake: Intake) => void;
    onmessage: (message: string) => void;
  } = $props();

  let values = $state<string[]>([]);
  let error = $state('');
  let busy = $state(false);

  const choices = $derived(reviewerChoices(options, requirement.createdBy, requirement.reviewers));
  const saved = $derived(requirement.reviewers.map((reviewer) => reviewer.id));
  const dirty = $derived(values.length !== saved.length || values.some((value, index) => value !== saved[index]));

  $effect(() => {
    values = requirement.reviewers.map((reviewer) => reviewer.id);
  });

  async function save() {
    busy = true;
    error = '';
    onmessage('Saving reviewers');
    try {
      onsaved(await setReviewers(requirement.id, values));
      onmessage('Reviewers saved');
    } catch (failure) {
      error = failure instanceof ApiError ? (failure.fields.userIds ?? failure.message) : unreachable();
      onmessage(failure instanceof ApiError ? failure.message : unreachable());
    } finally {
      busy = false;
    }
  }
</script>

{#snippet saveBar()}
  <Button variant="primary" type="button" disabled={busy || !dirty} onclick={() => void save()}>Save reviewers</Button>
{/snippet}

<Panel
  title="Reviewers"
  hint={requirement.canEditReviewers
    ? 'GitHub asks these people to review each pull request for this requirement. The list shows tribe members with a verified GitHub login.'
    : 'Only the person who brought this requirement in, or a Lead Developer, can change its reviewers.'}
  footer={requirement.canEditReviewers ? saveBar : undefined}
>
  {#if requirement.canEditReviewers}
    <MultiSearchSelect
      label="Reviewers"
      id="requirement-reviewers"
      width="lg"
      options={choices}
      placeholder={choices.length === 0 ? 'No verified tribe members yet' : 'Search by name, login, or email'}
      max={15}
      {error}
      bind:values
    />
  {:else if requirement.reviewers.length > 0}
    <MultiSearchSelect label="Reviewers" id="requirement-reviewers" options={choices} values={saved} disabled />
  {:else}
    <p class="text-sm text-muted">No reviewers named.</p>
  {/if}
</Panel>
