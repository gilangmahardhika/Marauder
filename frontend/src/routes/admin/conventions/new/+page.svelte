<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { getConventions, saveRule, saveStack } from '$lib/api';
  import RuleForm, { ruleFields } from '$lib/components/RuleForm.svelte';
  import StackForm, { stackFields } from '$lib/components/StackForm.svelte';
  import { ruleBody, ruleInput, stackDraft, stackInput } from '$lib/conventions';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { Stack } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';

  const kind = $derived($page.url.searchParams.get('kind') === 'stack' ? 'stack' : 'rule');
  let rule = $state(ruleInput(undefined, $page.url.searchParams.get('stack') ?? ''));
  let stack = $state(stackDraft());
  let stacks = $state<Stack[]>([]);
  let busy = $state(false);
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();

  showShell(() => ({ title: kind === 'stack' ? 'New stack' : 'New rule' }));

  $effect(() => {
    void getConventions()
      .then((body) => {
        stacks = body.stacks;
      })
      .catch((error: unknown) => {
        toast.error(failureText(error));
      });
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failure = noFailure;
    try {
      if (kind === 'stack') {
        await saveStack('', stackInput(stack));
        toast.success('Stack saved');
        await goto('/admin/conventions?tab=stacks');
      } else {
        const saved = await saveRule('', ruleBody(rule));
        toast.success('Rule saved');
        await goto(`/admin/conventions/${encodeURIComponent(saved.id)}`);
      }
    } catch (error) {
      failure = formFailure(error, kind === 'stack' ? stackFields : ruleFields(rule));
      void focusInvalid(form);
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
  <form onsubmit={save} bind:this={form} novalidate>
    <Panel title={kind === 'stack' ? 'Stack' : 'Rule'}>
      <FormAlert message={failure.message} extra={failure.extra} />
      {#if kind === 'stack'}
        <StackForm bind:stack fields={failure.fields} />
      {:else}
        <RuleForm bind:rule {stacks} fields={failure.fields} />
      {/if}
    </Panel>
    <SaveBar {busy} label={kind === 'stack' ? 'Add stack' : 'Add rule'} icon="plus" note="Only an admin can change the catalog. Each save is versioned." />
  </form>
</Page>
