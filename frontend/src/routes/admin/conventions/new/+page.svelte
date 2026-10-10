<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { ApiError, getConventions, saveRule, saveStack, unreachable } from '$lib/api';
  import RuleForm from '$lib/components/RuleForm.svelte';
  import StackForm from '$lib/components/StackForm.svelte';
  import { ruleBody, ruleInput, stackDraft, stackInput } from '$lib/conventions';
  import { showShell } from '$lib/shell.svelte';
  import type { Stack } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';

  const kind = $derived($page.url.searchParams.get('kind') === 'stack' ? 'stack' : 'rule');
  let rule = $state(ruleInput(undefined, $page.url.searchParams.get('stack') ?? ''));
  let stack = $state(stackDraft());
  let stacks = $state<Stack[]>([]);
  let fields = $state<Record<string, string>>({});
  let busy = $state(false);
  let message = $state('New convention');

  showShell(() => ({ title: kind === 'stack' ? 'New stack' : 'New rule', message }));

  $effect(() => {
    void getConventions()
      .then((body) => {
        stacks = body.stacks;
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    fields = {};
    message = kind === 'stack' ? 'Saving stack' : 'Saving rule';
    try {
      if (kind === 'stack') {
        await saveStack('', stackInput(stack));
        message = 'Stack saved';
        await goto('/admin/conventions?tab=stacks');
      } else {
        const saved = await saveRule('', ruleBody(rule));
        message = 'Rule saved';
        await goto(`/admin/conventions/${encodeURIComponent(saved.id)}`);
      }
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }
</script>

<Page
  title={kind === 'stack' ? 'New stack' : 'New rule'}
  description={kind === 'stack'
    ? 'A language or framework. Sync detects it from marker files and dependencies in the repository.'
    : 'A rule the implementer follows and the reviewer checks, for every project or for one stack.'}
>
  {#snippet actions()}
    <Button href={kind === 'stack' ? '/admin/conventions?tab=stacks' : '/admin/conventions'}>Cancel</Button>
  {/snippet}
  <form onsubmit={save}>
    <Panel title={kind === 'stack' ? 'Stack' : 'Rule'}>
      {#if kind === 'stack'}
        <StackForm bind:stack {fields} />
      {:else}
        <RuleForm bind:rule {stacks} {fields} />
      {/if}
    </Panel>
    <SaveBar {busy} label={kind === 'stack' ? 'Add stack' : 'Add rule'} note="Only an admin can change the catalog. Each save is versioned." />
  </form>
</Page>
