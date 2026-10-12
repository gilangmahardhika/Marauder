<script lang="ts">
  import {
    ApiError,
    approveCard,
    deployCard,
    getCard,
    rejectCard,
    requestChanges,
    retryReview,
    runCard,
    unblockCard
  } from '$lib/api';
  import { shell } from '$lib/shell.svelte';
  import type { Card } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import Field from '$lib/ui/Field.svelte';
  import Menu from '$lib/ui/Menu.svelte';
  import Notice from '$lib/ui/Notice.svelte';

  let { card = $bindable() }: { card: Card } = $props();
  let sha = $state(card.headSha ?? '');
  let reset = $state(false);
  let shaError = $state('');
  let busy = $state(false);
  let asking = $state(false);
  let reason = $state('');
  let reasonError = $state('');
  let starting = $state(false);
  let instructions = $state('');
  let instructionsError = $state('');

  const actions = $derived(card.actions ?? []);
  const menuItems = $derived([
    ...(actions.includes('request_changes') ? [{ label: 'Request changes', onclick: () => (asking = true) }] : []),
    ...(actions.includes('reject') ? [{ label: 'Reject', onclick: () => void run('Reject', () => rejectCard(card.id)) }] : [])
  ]);

  async function run(action: string, work: () => Promise<Card>) {
    busy = true;
    shaError = '';
    reasonError = '';
    instructionsError = '';
    shell.message = action;
    try {
      card = await work();
      sha = card.headSha ?? sha;
      return true;
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        card = await getCard(card.id);
        shell.message = 'Card changed. Reloaded.';
        return false;
      }
      if (error instanceof ApiError && error.fields.sha) shaError = error.fields.sha;
      if (error instanceof ApiError && error.fields.reason) reasonError = error.fields.reason;
      if (error instanceof ApiError && error.fields.instructions) instructionsError = error.fields.instructions;
      shell.message = error instanceof Error ? error.message : 'API unreachable at http://127.0.0.1:8787';
      return false;
    } finally {
      busy = false;
    }
  }

  async function sendChanges() {
    if (await run('Request changes', () => requestChanges(card.id, reason))) {
      asking = false;
      reason = '';
    }
  }

  function cancelChanges() {
    asking = false;
    reason = '';
    reasonError = '';
  }

  function openRun() {
    instructions = card.instructions ?? '';
    instructionsError = '';
    starting = true;
  }

  async function sendRun() {
    const changed = instructions.trim() !== (card.instructions ?? '');
    if (await run('Queue run', () => runCard(card.id, changed ? instructions : undefined))) {
      starting = false;
    }
  }
</script>

<div class="flex flex-col gap-5">
  {#if actions.includes('run') && starting}
    <form
      class="flex flex-col items-start gap-3"
      onsubmit={(event) => {
        event.preventDefault();
        void sendRun();
      }}
    >
      <Field
        label="Instructions for the next attempt"
        id="instructions"
        type="textarea"
        width="full"
        rows={4}
        grow
        hint="Optional. Every later attempt reads these with the newest note, until you change them. Markdown works."
        error={instructionsError}
        bind:value={instructions}
      />
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="primary" type="submit" disabled={busy}>Run</Button>
        <Button variant="ghost" type="button" disabled={busy} onclick={() => (starting = false)}>Cancel</Button>
      </div>
    </form>
  {:else if actions.includes('run')}
    <div class="flex flex-wrap items-center gap-3">
      <Button variant="primary" type="button" disabled={busy} onclick={openRun}>Run</Button>
      <p class="text-sm text-muted">Queues this card for the worker. You can add instructions first.</p>
    </div>
  {:else if card.queued}
    <p class="text-sm text-muted">Queued. The card starts when the worker picks it up.</p>
  {/if}
  {#if actions.includes('retry_review')}
    <div class="flex flex-wrap items-center gap-3">
      <Button
        variant="primary"
        type="button"
        disabled={busy}
        onclick={() => void run('Retry AI review', () => retryReview(card.id))}
      >
        Retry AI review
      </Button>
      <p class="text-sm text-muted">The AI review gave no verdict. The worker runs it again on this commit.</p>
    </div>
  {/if}
  {#if actions.includes('approve')}
    <form
      class="flex flex-col gap-1.5"
      onsubmit={(event) => {
        event.preventDefault();
        void run('Approve SHA', () => approveCard(card.id, sha));
      }}
    >
      <div class="flex flex-wrap items-end gap-3">
        <Field label="SHA" id="sha" width="md" mono bind:value={sha} />
        <Button variant="primary" type="submit" disabled={busy}>Approve SHA</Button>
      </div>
      {#if shaError}<p class="error text-xs">{shaError}</p>{/if}
    </form>
  {/if}
  {#if actions.includes('deploy') || menuItems.length > 0}
    <div class="flex flex-wrap items-center gap-2">
      {#if actions.includes('deploy')}
        <Button
          variant={actions.includes('approve') ? 'secondary' : 'primary'}
          type="button"
          disabled={busy}
          onclick={() => void run('Deploy approved SHA', () => deployCard(card.id))}
        >
          Deploy approved SHA
        </Button>
      {/if}
      {#if menuItems.length > 0}
        <Menu label="Actions" block={false} items={menuItems} />
      {/if}
    </div>
  {/if}
  {#if asking && actions.includes('request_changes')}
    <form
      class="flex flex-col items-start gap-3"
      onsubmit={(event) => {
        event.preventDefault();
        void sendChanges();
      }}
    >
      <Field
        label="What should change?"
        id="reason"
        type="textarea"
        width="full"
        rows={4}
        grow
        hint="Optional. The next attempt gets this note. Markdown works."
        error={reasonError}
        bind:value={reason}
      />
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="primary" type="submit" disabled={busy}>Request changes</Button>
        <Button variant="ghost" type="button" disabled={busy} onclick={cancelChanges}>Cancel</Button>
      </div>
    </form>
  {/if}
  {#if actions.includes('unblock')}
    <form
      class="flex flex-col items-start gap-2"
      onsubmit={(event) => {
        event.preventDefault();
        void run('Unblock', () => unblockCard(card.id, reset));
      }}
    >
      <p class="text-sm text-ink">
        {#if card.unblockTo === 'human_review'}
          Unblock returns this card to human review. Approve the head again to redeploy.
        {:else}
          Unblock returns this card to Ready.
        {/if}
      </p>
      <Checkbox id="reset" label="Reset attempt counter" bind:checked={reset} />
      <Button variant={actions.includes('approve') || actions.includes('deploy') ? 'secondary' : 'primary'} type="submit" disabled={busy}>
        Unblock
      </Button>
    </form>
  {/if}
  {#if card.status === 'blocked' && card.unblockTo === null}
    <Notice tone="danger">Accept the changed requirements in Intake before this card can be released.</Notice>
  {/if}
</div>
