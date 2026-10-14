<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { deleteConvention, getConventions, saveRule, saveStack } from '$lib/api';
  import RuleForm, { ruleFields } from '$lib/components/RuleForm.svelte';
  import StackForm, { stackFields } from '$lib/components/StackForm.svelte';
  import { editedBy, ruleBody, ruleInput, stackDraft, stackInput, type StackDraft } from '$lib/conventions';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { ConventionRule, RuleInput, Stack } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import SaveBar from '$lib/ui/SaveBar.svelte';

  const id = $derived($page.params.id ?? '');
  const kind = $derived(id.startsWith('stk_') ? 'stack' : 'rule');
  const noun = $derived(kind === 'stack' ? 'Stack' : 'Rule');

  let current = $state<ConventionRule | Stack | null>(null);
  let rule = $state<RuleInput | null>(null);
  let stack = $state<StackDraft | null>(null);
  let stacks = $state<Stack[]>([]);
  let saving = $state(false);
  let deleting = $state(false);
  let confirming = $state(false);
  let loading = $state('Loading convention');
  let loadError = $state('');
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();

  const title = $derived(current ? ('name' in current ? current.name : current.title) : 'Convention');
  const back = $derived(kind === 'stack' ? '/admin/conventions?tab=stacks' : '/admin/conventions');

  showShell(() => ({ title }));

  function pick(body: { rules: ConventionRule[]; stacks: Stack[] }) {
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
    if (current === null) loadError = `There is no rule or stack with id ${id}.`;
  }

  function load() {
    loading = 'Loading convention';
    loadError = '';
    void getConventions()
      .then(pick)
      .catch((error: unknown) => {
        loadError = failureText(error);
      })
      .finally(() => {
        loading = '';
      });
  }

  $effect(() => {
    if (id !== '') load();
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    saving = true;
    failure = noFailure;
    try {
      if (kind === 'stack' && stack) {
        current = await saveStack(id, stackInput(stack));
        stack = stackDraft(current as Stack);
      } else if (rule) {
        current = await saveRule(id, ruleBody(rule));
        rule = ruleInput(current as ConventionRule);
      }
      toast.success(`${noun} saved`);
    } catch (error) {
      failure = formFailure(error, kind === 'stack' ? stackFields : rule ? ruleFields(rule) : []);
      void focusInvalid(form);
    } finally {
      saving = false;
    }
  }

  async function remove() {
    deleting = true;
    try {
      await deleteConvention(kind === 'stack' ? 'stacks' : 'rules', id);
      toast.success(`${noun} deleted`);
      await goto(back);
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      deleting = false;
      confirming = false;
    }
  }
</script>

<Page {title} {loading} error={loadError} onretry={load}>
  {#snippet meta()}
    {#if current}
      <Badge>v{current.version}</Badge>
      {#if current.disabled}<Badge tone="amber">Disabled</Badge>{/if}
    {/if}
  {/snippet}
  {#snippet actions()}
    <Button href={back} icon="arrow-left">All {kind === 'stack' ? 'stacks' : 'rules'}</Button>
  {/snippet}
  {#if current}
    <div class="flex flex-col gap-6">
      <form onsubmit={save} bind:this={form} novalidate>
        <Panel title={noun} hint={editedBy(current)}>
          <FormAlert message={failure.message} extra={failure.extra} />
          {#if kind === 'stack' && stack}
            <StackForm bind:stack={stack!} fields={failure.fields} />
          {:else if rule}
            <RuleForm bind:rule={rule!} {stacks} fields={failure.fields} />
          {/if}
        </Panel>
        <SaveBar busy={saving} disabled={deleting} note="Changing the text makes a new version. Earlier versions stay in the revision history." />
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
            <Button type="button" variant="ghost" disabled={deleting} onclick={() => (confirming = false)}>Cancel</Button>
            <Button type="button" variant="danger" icon="trash" loading={deleting} disabled={saving} onclick={() => void remove()}>Delete</Button>
          </div>
        {:else}
          <Button type="button" variant="danger" icon="trash" disabled={saving || deleting} onclick={() => (confirming = true)}>
            {kind === 'stack' ? 'Delete stack' : 'Delete rule'}
          </Button>
        {/if}
      </Panel>
    </div>
  {/if}
</Page>
