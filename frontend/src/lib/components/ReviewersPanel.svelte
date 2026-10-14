<script lang="ts">
  import { setAIReviewMode, setReviewers, setSelfReview } from '$lib/api';
  import AIReviewModeField from '$lib/components/AIReviewModeField.svelte';
  import { failureText, formFailure } from '$lib/forms';
  import { reviewerChoices } from '$lib/requirements';
  import { toast } from '$lib/toast.svelte';
  import type { AIReviewMode, Intake, Requirement, Reviewer } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import MultiSearchSelect from '$lib/ui/MultiSearchSelect.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let {
    requirement,
    options,
    onsaved
  }: {
    requirement: Requirement;
    options: Reviewer[];
    onsaved: (intake: Intake) => void;
  } = $props();

  let values = $state<string[]>([]);
  let self = $state(false);
  let mode = $state<AIReviewMode>('continue');
  let fields = $state<Record<string, string>>({});
  let busy = $state(false);

  const choices = $derived(reviewerChoices(options, requirement.createdBy, requirement.reviewers));
  const saved = $derived(requirement.reviewers.map((reviewer) => reviewer.id));
  const reviewersDirty = $derived(values.length !== saved.length || values.some((value, index) => value !== saved[index]));
  const selfDirty = $derived(self !== requirement.selfReview);
  const modeDirty = $derived(mode !== requirement.aiReviewMode);
  const editable = $derived(requirement.canEditReviewers || requirement.canSetSelfReview);

  $effect(() => {
    values = requirement.reviewers.map((reviewer) => reviewer.id);
    self = requirement.selfReview;
    mode = requirement.aiReviewMode;
  });

  async function save() {
    busy = true;
    fields = {};
    try {
      let intake: Intake | null = null;
      if (requirement.canEditReviewers && reviewersDirty) intake = await setReviewers(requirement.id, values);
      if (requirement.canSetSelfReview && selfDirty) intake = await setSelfReview(requirement.id, self);
      if (requirement.canSetSelfReview && modeDirty) intake = await setAIReviewMode(requirement.id, mode);
      if (intake) onsaved(intake);
      toast.success('Reviewers saved');
    } catch (error) {
      fields = formFailure(error).fields;
      toast.error(failureText(error));
    } finally {
      busy = false;
    }
  }
</script>

{#snippet saveBar()}
  <Button
    variant="primary"
    type="button"
    icon="save"
    loading={busy}
    disabled={!(reviewersDirty || selfDirty || modeDirty)}
    onclick={() => void save()}
  >
    Save reviewers
  </Button>
{/snippet}

<Panel
  title="Reviewers"
  hint={requirement.canEditReviewers
    ? 'GitHub asks these people to review each pull request for this requirement. Someone without a verified GitHub login is asked to review on GitHub after they verify.'
    : 'Only the person who brought this requirement in, or a Lead Developer, can change its reviewers.'}
  footer={editable ? saveBar : undefined}
>
  {#if requirement.canEditReviewers}
    <MultiSearchSelect
      label="Reviewers"
      id="requirement-reviewers"
      width="lg"
      options={choices}
      placeholder={choices.length === 0 ? 'No other tribe members yet' : 'Search by name, login, or email'}
      max={15}
      error={fields.userIds}
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
      error={fields.selfReview}
      bind:checked={self}
    />
    <AIReviewModeField error={fields.aiReviewMode} bind:value={mode} />
  {:else}
    {#if requirement.selfReview}
      <p class="text-sm text-ink">Self review: on. The card owner approves alone.</p>
    {/if}
    {#if requirement.aiReviewMode === 'manual'}
      <p class="text-sm text-ink">When the AI review asks for changes, a person reviews before another attempt.</p>
    {/if}
  {/if}
</Panel>
