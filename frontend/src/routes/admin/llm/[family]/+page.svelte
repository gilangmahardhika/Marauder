<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { getLLM, putLLM, testLLM } from '$lib/api';
  import FamilyForm, { familyFields } from '$lib/components/FamilyForm.svelte';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { fromDraft, keepKey, protocolLabel, toDraft, type FamilyDraft } from '$lib/llm';
  import { showShell } from '$lib/shell.svelte';
  import { toast } from '$lib/toast.svelte';
  import type { Family } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Page from '$lib/ui/Page.svelte';
  import Panel from '$lib/ui/Panel.svelte';

  let family = $state<FamilyDraft | null>(null);
  let failure = $state(noFailure);
  let form = $state<HTMLFormElement>();
  let result = $state('');
  let confirming = $state(false);
  let loading = $state('Loading family');
  let loadError = $state('');
  let saving = $state(false);
  let testing = $state(false);
  let deleting = $state(false);
  const busy = $derived(saving || testing || deleting);

  const familyId = $derived($page.params.family ?? '');

  showShell(() => ({ title: familyId || 'LLM family' }));

  function pick(list: Family[]) {
    const found = list.find((item) => item.id === familyId);
    family = found ? toDraft(found) : null;
    if (!found) loadError = `There is no LLM family named ${familyId}.`;
  }

  function load(id: string) {
    loading = 'Loading family';
    loadError = '';
    void getLLM()
      .then((body) => {
        if (id === familyId) pick(body.families);
      })
      .catch((error: unknown) => {
        loadError = failureText(error);
      })
      .finally(() => {
        loading = '';
      });
  }

  $effect(() => {
    const id = familyId;
    if (id !== '') load(id);
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    if (!family) return;
    saving = true;
    failure = noFailure;
    try {
      const current = await getLLM();
      const draft = family;
      const saved = await putLLM({
        families: current.families.map((item) => (item.id === familyId ? fromDraft(draft) : keepKey(item)))
      });
      pick(saved.families);
      toast.success('Family saved');
    } catch (error) {
      failure = formFailure(error, familyFields(family));
      void focusInvalid(form);
    } finally {
      saving = false;
    }
  }

  async function test() {
    testing = true;
    try {
      const outcome = await testLLM(familyId);
      result = outcome.ok ? 'Connected' : (outcome.message ?? 'failed');
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      testing = false;
    }
  }

  async function remove() {
    deleting = true;
    try {
      const current = await getLLM();
      await putLLM({ families: current.families.filter((item) => item.id !== familyId).map(keepKey) });
      toast.success('Family deleted');
      await goto('/admin/llm');
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      deleting = false;
      confirming = false;
    }
  }
</script>

<Page title={familyId || 'LLM family'} {loading} error={loadError} onretry={() => familyId && load(familyId)}>
  {#snippet meta()}
    {#if family}<Badge>{protocolLabel(family.protocol)}</Badge>{/if}
  {/snippet}
  {#snippet actions()}
    <Button href="/admin/llm" icon="arrow-left">All families</Button>
  {/snippet}
  {#if family}
    <div class="flex flex-col gap-6">
      <form onsubmit={save} bind:this={form} novalidate>
        <Panel title="Family">
          <FormAlert message={failure.message} extra={failure.extra} />
          <FamilyForm bind:family={family!} idLocked fields={failure.fields} />
          {#snippet footer()}
            {#if result}<span class="mr-auto text-sm text-muted">{result}</span>{/if}
            <Button
              type="button"
              variant="secondary"
              icon="zap"
              loading={testing}
              disabled={busy}
              onclick={() => void test()}>Test connection</Button
            >
            <Button variant="primary" type="submit" icon="save" loading={saving} disabled={busy}>Save</Button>
          {/snippet}
        </Panel>
      </form>
      <Panel title="Delete family" hint="Routes that use its models stop resolving.">
        {#if confirming}
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm text-ink">Delete {familyId}?</span>
            <Button type="button" variant="ghost" disabled={busy} onclick={() => (confirming = false)}>Cancel</Button>
            <Button type="button" variant="danger" icon="trash" loading={deleting} disabled={busy} onclick={() => void remove()}>Delete</Button>
          </div>
        {:else}
          <Button type="button" variant="danger" icon="trash" disabled={busy} onclick={() => (confirming = true)}>Delete family</Button>
        {/if}
      </Panel>
    </div>
  {/if}
</Page>
