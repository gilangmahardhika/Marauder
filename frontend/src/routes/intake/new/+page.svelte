<script lang="ts">
  import { goto } from '$app/navigation';
  import { ApiError, addRequirement, getIntake, setReviewers, unreachable } from '$lib/api';
  import RequirementForm from '$lib/components/RequirementForm.svelte';
  import { reviewerChoices, toLines } from '$lib/requirements';
  import { shell, showShell } from '$lib/shell.svelte';
  import type { Reviewer } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import MultiSearchSelect from '$lib/ui/MultiSearchSelect.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let id = $state('');
  let statement = $state('');
  let acceptance = $state('');
  let reviewers = $state<string[]>([]);
  let options = $state<Reviewer[]>([]);
  let known = $state<string[]>([]);
  let fields = $state<Record<string, string>>({});
  let message = $state('New requirement');
  let busy = $state(false);

  const choices = $derived(reviewerChoices(options, shell.email));

  showShell(() => ({ title: 'New requirement', message }));

  $effect(() => {
    if (shell.projectId === '') return;
    void getIntake()
      .then((body) => {
        options = body.reviewerOptions;
        known = body.requirements.map((requirement) => requirement.id);
      })
      .catch(() => {
        options = [];
      });
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    fields = {};
    if (statement.trim() === '') {
      fields = { statement: 'Enter a statement.' };
      return;
    }
    busy = true;
    message = 'Adding requirement';
    let added = '';
    try {
      const body = await addRequirement({ id: id.trim(), statement, acceptance: toLines(acceptance) });
      added = id.trim() || (body.requirements.find((requirement) => !known.includes(requirement.id))?.id ?? '');
      if (reviewers.length > 0 && added !== '') {
        message = 'Saving reviewers';
        await setReviewers(added, reviewers);
      }
      message = 'Requirement added';
      await goto('/intake');
    } catch (error) {
      if (added !== '') {
        await goto(`/intake/${encodeURIComponent(added)}`);
        return;
      }
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title="New requirement"
  description="Added to this project's intake. It is not accepted until a senior member accepts it."
>
  <form onsubmit={submit}>
    <Panel title="Requirement">
      <RequirementForm bind:id bind:statement bind:acceptance {fields} />
      <MultiSearchSelect
        label="Reviewers"
        id="requirement-reviewers"
        width="lg"
        options={choices}
        placeholder={choices.length === 0 ? 'No verified tribe members yet' : 'Search by login or email'}
        max={15}
        error={fields.userIds}
        bind:values={reviewers}
      />
      {#snippet footer()}
        <Button variant="ghost" href="/intake">Cancel</Button>
        <Button variant="primary" type="submit" disabled={busy}>Add requirement</Button>
      {/snippet}
    </Panel>
  </form>
</Page>
