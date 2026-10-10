<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { ApiError, deleteConvention, getConventions, saveRule, saveStack, unreachable } from '$lib/api';
  import RuleForm from '$lib/components/RuleForm.svelte';
  import StackForm from '$lib/components/StackForm.svelte';
  import { editedBy, ruleBody, ruleInput, stackDraft, stackInput, type StackDraft } from '$lib/conventions';
  import { showShell } from '$lib/shell.svelte';
  import type { ConventionRule, RuleInput, Stack } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';

  const id = $derived($page.params.id ?? '');
  const kind = $derived(id.startsWith('stk_') ? 'stack' : 'rule');

  let current = $state<ConventionRule | Stack | null>(null);
  let rule = $state<RuleInput | null>(null);
  let stack = $state<StackDraft | null>(null);
  let stacks = $state<Stack[]>([]);
  let missing = $state(false);
  let fields = $state<Record<string, string>>({});
  let busy = $state(false);
  let confirming = $state(false);
  let message = $state('Loading');

  const title = $derived(current ? ('name' in current ? current.name : current.title) : 'Convention');
  const back = $derived(kind === 'stack' ? '/admin/conventions?tab=stacks' : '/admin/conventions');

  showShell(() => ({ title, message }));

  function load(body: { rules: ConventionRule[]; stacks: Stack[] }) {
    stacks = body.stacks;
    if (kind === 'stack') {
      const found = body.stacks.find((item) => item.id === id) ?? null;
      current = found;
      stack = found ? stackDraft(found) : null;
    } else {
      const found = body.rules.find((item) => item.id === id) ?? null;
      current = found;
      rule = found ? ruleInput(found) : null;
    }
    missing = current === null;
  }

  $effect(() => {
    if (id === '') return;
    void getConventions()
      .then((body) => {
        load(body);
        message = missing ? 'Not found' : title;
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    fields = {};
    message = 'Saving';
    try {
      if (kind === 'stack' && stack) {
        current = await saveStack(id, stackInput(stack));
        stack = stackDraft(current as Stack);
      } else if (rule) {
        current = await saveRule(id, ruleBody(rule));
        rule = ruleInput(current as ConventionRule);
      }
      message = 'Saved';
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  async function remove() {
    busy = true;
    message = 'Deleting';
    try {
      await deleteConvention(kind === 'stack' ? 'stacks' : 'rules', id);
      message = 'Deleted';
      await goto(back);
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
      confirming = false;
    }
  }
</script>

{#if missing}
  <Page title="Not found">
    <EmptyState title={`There is no rule or stack with id ${id}.`} href="/admin/conventions" link="All conventions" />
  </Page>
{:else if current}
  <Page title={title}>
    {#snippet meta()}
      <Badge>v{current!.version}</Badge>
      {#if current!.disabled}<Badge tone="amber">Disabled</Badge>{/if}
    {/snippet}
    {#snippet actions()}
      <Button href={back}>All {kind === 'stack' ? 'stacks' : 'rules'}</Button>
    {/snippet}
    <div class="flex flex-col gap-6">
      <form onsubmit={save}>
        <Panel title={kind === 'stack' ? 'Stack' : 'Rule'} hint={editedBy(current)}>
          {#if kind === 'stack' && stack}
            <StackForm bind:stack={stack!} {fields} />
          {:else if rule}
            <RuleForm bind:rule={rule!} {stacks} {fields} />
          {/if}
        </Panel>
        <SaveBar {busy} note="Changing the text makes a new version. Earlier versions stay in the revision history." />
      </form>
      <Panel
        title={kind === 'stack' ? 'Delete stack' : 'Delete rule'}
        hint={kind === 'stack'
          ? 'A stack that a rule or project still names is disabled instead.'
          : 'The rule leaves every pack. Its revisions stay.'}
      >
        {#if confirming}
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm text-ink">Delete {title}?</span>
            <Button type="button" variant="ghost" disabled={busy} onclick={() => (confirming = false)}>Cancel</Button>
            <Button type="button" variant="danger" disabled={busy} onclick={() => void remove()}>Delete</Button>
          </div>
        {:else}
          <Button type="button" variant="danger" disabled={busy} onclick={() => (confirming = true)}>
            {kind === 'stack' ? 'Delete stack' : 'Delete rule'}
          </Button>
        {/if}
      </Panel>
    </div>
  </Page>
{/if}
