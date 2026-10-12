<script lang="ts">
  import { ApiError, setReviewers, setSelfReview, unreachable } from '$lib/api';
  import { reviewerChoices } from '$lib/requirements';
  import type { Intake, Requirement, Reviewer } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Checkbox from '$lib/ui/Checkbox.svelte';
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
  let self = $state(false);
  let error = $state('');
  let busy = $state(false);

  const choices = $derived(reviewerChoices(options, requirement.createdBy, requirement.reviewers));
  const saved = $derived(requirement.reviewers.map((reviewer) => reviewer.id));
  const reviewersDirty = $derived(values.length !== saved.length || values.some((value, index) => value !== saved[index]));
  const selfDirty = $derived(self !== requirement.selfReview);
  const editable = $derived(requirement.canEditReviewers || requirement.canSetSelfReview);

  $effect(() => {
    values = requirement.reviewers.map((reviewer) => reviewer.id);
    self = requirement.selfReview;
  });

  async function save() {
    busy = true;
    error = '';
    onmessage('Saving reviewers');
    try {
      let intake: Intake | null = null;
      if (requirement.canEditReviewers && reviewersDirty) intake = await setReviewers(requirement.id, values);
      if (requirement.canSetSelfReview && selfDirty) intake = await setSelfReview(requirement.id, self);
      if (intake) onsaved(intake);
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
  <Button variant="primary" type="button" disabled={busy || !(reviewersDirty || selfDirty)} onclick={() => void save()}>
    Save reviewers
  </Button>
{/snippet}

<Panel
  title="Reviewers"
  hint={requirement.canEditReviewers
    ? 'GitHub asks these people to review each pull request for this requirement. The list shows tribe members with a verified GitHub login.'
    : 'Only the person who brought this requirement in, or a Lead Developer, can change its reviewers.'}
  footer={editable ? saveBar : undefined}
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
  {#if requirement.canSetSelfReview}
    <Checkbox
      id="requirement-self-review"
      label="Self review"
      hint="The owner of this requirement's cards reviews and approves them alone. The approval is recorded as not independent."
      bind:checked={self}
    />
  {:else if requirement.selfReview}
    <p class="mt-3 text-sm text-ink">Self review: on. The card owner approves alone.</p>
  {/if}
</Panel>
