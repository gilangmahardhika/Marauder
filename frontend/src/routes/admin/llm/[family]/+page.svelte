<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { ApiError, getLLM, putLLM, testLLM, unreachable } from '$lib/api';
  import FamilyForm from '$lib/components/FamilyForm.svelte';
  import { fromDraft, keepKey, protocolLabel, toDraft, type FamilyDraft } from '$lib/llm';
  import { showShell } from '$lib/shell.svelte';
  import type { Family } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let family = $state<FamilyDraft | null>(null);
  let missing = $state(false);
  let fields = $state<Record<string, string>>({});
  let result = $state('');
  let confirming = $state(false);
  let message = $state('Loading family');
  let busy = $state(false);

  const familyId = $derived($page.params.family ?? '');

  showShell(() => ({ title: familyId || 'LLM family', message }));

  function pick(list: Family[]) {
    const found = list.find((item) => item.id === familyId);
    family = found ? toDraft(found) : null;
    missing = !found;
  }

  $effect(() => {
    const id = familyId;
    if (id === '') return;
    void getLLM()
      .then((body) => {
        pick(body.families);
        message = missing ? 'Family not found' : id;
      })
      .catch((error: unknown) => {
        message = error instanceof ApiError ? error.message : unreachable();
      });
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    if (!family) return;
    busy = true;
    fields = {};
    message = 'Saving family';
    try {
      const current = await getLLM();
      const draft = family;
      const saved = await putLLM({
        families: current.families.map((item) => (item.id === familyId ? fromDraft(draft) : keepKey(item)))
      });
      pick(saved.families);
      message = 'Family saved';
    } catch (error) {
      fields = error instanceof ApiError ? error.fields : {};
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  async function test() {
    busy = true;
    try {
      const outcome = await testLLM(familyId);
      result = outcome.ok ? 'Connected' : (outcome.message ?? 'failed');
      message = result;
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
    }
  }

  async function remove() {
    busy = true;
    message = 'Deleting family';
    try {
      const current = await getLLM();
      await putLLM({ families: current.families.filter((item) => item.id !== familyId).map(keepKey) });
      message = 'Family deleted';
      await goto('/admin/llm');
    } catch (error) {
      message = error instanceof ApiError ? error.message : unreachable();
    } finally {
      busy = false;
      confirming = false;
    }
  }
</script>

{#if missing}
  <Page title="Family not found">
    <EmptyState title={`There is no LLM family named ${familyId}.`} href="/admin/llm" link="All families" />
  </Page>
{:else if family}
  <Page title={familyId}>
    {#snippet meta()}
      <Badge>{protocolLabel(family!.protocol)}</Badge>
    {/snippet}
    {#snippet actions()}
      <Button href="/admin/llm">All families</Button>
    {/snippet}
    <div class="flex flex-col gap-6">
      <form onsubmit={save}>
        <Panel title="Family">
          <FamilyForm bind:family={family!} idLocked {fields} />
          {#snippet footer()}
            {#if result}<span class="mr-auto text-sm text-muted">{result}</span>{/if}
            <Button type="button" variant="secondary" disabled={busy} onclick={() => void test()}>Test connection</Button>
            <Button variant="primary" type="submit" disabled={busy}>Save</Button>
          {/snippet}
        </Panel>
      </form>
      <Panel title="Delete family" hint="Routes that use its models stop resolving.">
        {#if confirming}
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm text-ink">Delete {familyId}?</span>
            <Button type="button" variant="ghost" disabled={busy} onclick={() => (confirming = false)}>Cancel</Button>
            <Button type="button" variant="danger" disabled={busy} onclick={() => void remove()}>Delete</Button>
          </div>
        {:else}
          <Button type="button" variant="danger" disabled={busy} onclick={() => (confirming = true)}>Delete family</Button>
        {/if}
      </Panel>
    </div>
  </Page>
{/if}
