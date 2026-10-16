<script lang="ts">
  import {
    ApiError,
    approveCard,
    deployCard,
    getCard,
    mergeCard,
    rejectCard,
    reopenCard,
    requestChanges,
    retryReview,
    runCard,
    unblockCard,
    wontFixCard
  } from '$lib/api';
  import { failureText, focusInvalid, formFailure, noFailure, type FormFailure } from '$lib/forms';
  import { toast } from '$lib/toast.svelte';
  import { verdictLabel, verdictTone } from '$lib/trace';
  import type { Card } from '$lib/types';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Menu from '$lib/ui/Menu.svelte';
  import Notice from '$lib/ui/Notice.svelte';

  type FormName = 'approve' | 'changes' | 'wontfix' | 'run';

  const shownFields: Record<FormName, string[]> = {
    approve: ['sha'],
    changes: ['reason'],
    wontfix: ['reason'],
    run: ['instructions']
  };

  let { card = $bindable() }: { card: Card } = $props();
  let sha = $state(card.headSha ?? '');
  let reset = $state(false);
  /** The action in flight, such as "approve"; its button shows the spinner. */
  let pending = $state('');
  let asking = $state(false);
  let reason = $state('');
  let starting = $state(false);
  let instructions = $state('');
  let closing = $state(false);
  let closeReason = $state('');
  let failures = $state<Record<FormName, FormFailure>>({
    approve: noFailure,
    changes: noFailure,
    wontfix: noFailure,
    run: noFailure
  });
  let forms = $state<Record<FormName, HTMLFormElement | undefined>>({
    approve: undefined,
    changes: undefined,
    wontfix: undefined,
    run: undefined
  });

  let choosing = $state(false);

  const busy = $derived(pending !== '');
  const actions = $derived(card.actions ?? []);
  const menuItems = $derived(
    actions.includes('reject')
      ? [{ label: 'Reject', onclick: () => void act('reject', 'Rejected', () => rejectCard(card.id)) }]
      : []
  );

  function short(value: string) {
    return value.slice(0, 7);
  }

  async function reload() {
    try {
      card = await getCard(card.id);
      toast.info('Card changed. Reloaded.');
    } catch (error) {
      toast.error(failureText(error));
    }
  }

  /**
   * Sends one action. A failure from a form shows in that form; any other failure is an error toast.
   */
  async function act(name: string, done: string, work: () => Promise<Card>, form?: FormName) {
    pending = name;
    if (form) failures[form] = noFailure;
    try {
      card = await work();
      sha = card.headSha ?? sha;
      toast.success(done);
      return true;
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        await reload();
        return false;
      }
      if (form) {
        failures[form] = formFailure(error, shownFields[form]);
        if (form === 'approve' && failures.approve.fields.sha) choosing = true;
        void focusInvalid(forms[form]);
      } else {
        toast.error(failureText(error));
      }
      return false;
    } finally {
      pending = '';
    }
  }

  /** Newest AI review note on the card, or "" when none (notes are newest-first). */
  function latestReviewNote() {
    return card.notes?.find((n) => n.source === 'review')?.note ?? '';
  }

  function openChanges() {
    if (reason === '') reason = latestReviewNote();
    asking = true;
  }

  async function sendChanges() {
    if (await act('request_changes', 'Changes requested', () => requestChanges(card.id, reason), 'changes')) {
      asking = false;
      reason = '';
    }
  }

  function cancelChanges() {
    asking = false;
    reason = '';
    failures.changes = noFailure;
  }

  async function sendWontFix() {
    if (await act('wont_fix', "Closed as won't fix", () => wontFixCard(card.id, closeReason.trim()), 'wontfix')) {
      closing = false;
      closeReason = '';
    }
  }

  function cancelWontFix() {
    closing = false;
    closeReason = '';
    failures.wontfix = noFailure;
  }

  function openRun() {
    instructions = card.instructions ?? '';
    failures.run = noFailure;
    starting = true;
  }

  async function sendRun() {
    const changed = instructions.trim() !== (card.instructions ?? '');
    if (await act('run', 'Queued', () => runCard(card.id, changed ? instructions : undefined), 'run')) {
      starting = false;
    }
  }
</script>

<div class="flex flex-col gap-5">
  {#if actions.includes('run') && starting}
    <form
      class="flex flex-col items-start gap-3"
      bind:this={forms.run}
      novalidate
      onsubmit={(event) => {
        event.preventDefault();
        void sendRun();
      }}
    >
      <FormAlert message={failures.run.message} extra={failures.run.extra} />
      <Field
        label="Instructions for the next attempt"
        id="instructions"
        type="textarea"
        width="full"
        rows={4}
        grow
        hint="Optional. Every later attempt reads these with the newest note, until you change them. Markdown works."
        error={failures.run.fields.instructions}
        bind:value={instructions}
      />
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="primary" type="submit" icon="play" disabled={busy} loading={pending === 'run'}>Run</Button>
        <Button variant="ghost" type="button" disabled={busy} onclick={() => (starting = false)}>Cancel</Button>
      </div>
    </form>
  {:else if actions.includes('run')}
    <div class="flex flex-wrap items-center gap-3">
      <Button variant="primary" type="button" icon="play" disabled={busy} onclick={openRun}>Run</Button>
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
        icon="refresh"
        disabled={busy}
        loading={pending === 'retry_review'}
        onclick={() => void act('retry_review', 'AI review queued', () => retryReview(card.id))}
      >
        Retry AI review
      </Button>
      <p class="text-sm text-muted">The AI review gave no verdict. The worker runs it again on this commit.</p>
    </div>
  {/if}
  {#snippet decisions()}
    {#if actions.includes('deploy')}
      <Button
        variant={actions.includes('approve') ? 'secondary' : 'primary'}
        type="button"
        icon="upload"
        disabled={busy}
        loading={pending === 'deploy'}
        onclick={() => void act('deploy', 'Deploy started', () => deployCard(card.id))}
      >
        Deploy approved SHA
      </Button>
    {/if}
    {#if actions.includes('merge')}
      <Button
        variant={actions.includes('approve') ? 'secondary' : 'primary'}
        type="button"
        icon="git"
        disabled={busy}
        loading={pending === 'merge'}
        onclick={() => void act('merge', 'Merged', () => mergeCard(card.id))}
      >
        Squash and merge
      </Button>
    {/if}
    {#if actions.includes('request_changes') && !asking}
      <Button type="button" icon="message" disabled={busy} onclick={openChanges}>Request changes</Button>
    {/if}
    {#if menuItems.length > 0}
      <Menu label="More actions" block={false} items={menuItems} />
    {/if}
  {/snippet}
  {#if actions.includes('approve')}
    <form
      class="flex flex-col gap-3"
      bind:this={forms.approve}
      novalidate
      onsubmit={(event) => {
        event.preventDefault();
        void act('approve', 'Approved', () => approveCard(card.id, sha), 'approve');
      }}
    >
      <FormAlert message={failures.approve.message} extra={failures.approve.extra} />
      {#if card.reviewVerdict}
        <p class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-muted">
          <Badge tone={verdictTone(card.reviewVerdict)}>AI review: {verdictLabel(card.reviewVerdict)}</Badge>
          {#if card.reviewedSha}
            <span>on <span class="font-mono text-ink" title={card.reviewedSha}>{short(card.reviewedSha)}</span></span>
            {#if card.headSha && card.reviewedSha !== card.headSha}
              <span class="text-amber-800">The head moved after the review.</span>
            {/if}
          {/if}
        </p>
      {/if}
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="primary" type="submit" icon="check" disabled={busy} loading={pending === 'approve'}>
          {sha.trim() ? `Approve ${short(sha.trim())}` : 'Approve SHA'}
        </Button>
        {@render decisions()}
      </div>
      {#if card.headSha}
        <button
          type="button"
          class="inline-flex min-h-11 w-fit items-center gap-1.5 text-sm text-muted hover:text-ink hover:underline"
          aria-expanded={choosing}
          aria-controls="approve-sha"
          onclick={() => (choosing = !choosing)}
        >
          <Icon name="chevron" class="size-4 transition-transform {choosing ? 'rotate-180' : ''}" />
          Approve a different SHA
        </button>
      {/if}
      {#if choosing || !card.headSha || failures.approve.fields.sha}
        <div id="approve-sha">
          <Field
            label="SHA"
            id="sha"
            width="md"
            mono
            hint="The full commit SHA a person approves."
            error={failures.approve.fields.sha}
            bind:value={sha}
          />
        </div>
      {/if}
    </form>
  {:else if actions.includes('deploy') || actions.includes('merge') || actions.includes('request_changes') || menuItems.length > 0}
    <div class="flex flex-wrap items-center gap-2">
      {@render decisions()}
    </div>
  {/if}
  {#if asking && actions.includes('request_changes')}
    <form
      class="flex flex-col items-start gap-3"
      bind:this={forms.changes}
      novalidate
      onsubmit={(event) => {
        event.preventDefault();
        void sendChanges();
      }}
    >
      <FormAlert message={failures.changes.message} extra={failures.changes.extra} />
      <Field
        label="What should change?"
        id="reason"
        type="textarea"
        width="full"
        rows={4}
        grow
        hint="Optional. Prefills from the newest AI review when one exists. The next attempt gets this note and the AI review feedback. Markdown works."
        error={failures.changes.fields.reason}
        bind:value={reason}
      />
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="primary" type="submit" icon="message" disabled={busy} loading={pending === 'request_changes'}>
          Request changes
        </Button>
        <Button variant="ghost" type="button" disabled={busy} onclick={cancelChanges}>Cancel</Button>
      </div>
    </form>
  {/if}
  {#if actions.includes('unblock')}
    <form
      class="flex flex-col items-start gap-2"
      onsubmit={(event) => {
        event.preventDefault();
        void act('unblock', 'Unblocked', () => unblockCard(card.id, reset));
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
      <div class="flex flex-wrap items-center gap-2">
        <Button
          variant={actions.includes('approve') || actions.includes('deploy') ? 'secondary' : 'primary'}
          type="submit"
          icon="unlock"
          disabled={busy}
          loading={pending === 'unblock'}
        >
          Unblock
        </Button>
        {#if actions.includes('wont_fix') && !closing}
          <Button type="button" icon="ban" disabled={busy} onclick={() => (closing = true)}>Won't fix</Button>
        {/if}
      </div>
    </form>
  {:else if actions.includes('wont_fix') && !closing}
    <div class="flex flex-wrap items-center gap-2">
      <Button type="button" icon="ban" disabled={busy} onclick={() => (closing = true)}>Won't fix</Button>
    </div>
  {/if}
  {#if closing && actions.includes('wont_fix')}
    <form
      class="flex flex-col items-start gap-3"
      bind:this={forms.wontfix}
      novalidate
      onsubmit={(event) => {
        event.preventDefault();
        void sendWontFix();
      }}
    >
      <FormAlert message={failures.wontfix.message} extra={failures.wontfix.extra} />
      <Field
        label="Why won't this be fixed?"
        id="wont-fix-reason"
        type="textarea"
        width="full"
        rows={3}
        grow
        hint="Shown in the trace. An open pull request gets this as a comment and is closed."
        error={failures.wontfix.fields.reason}
        bind:value={closeReason}
      />
      <div class="flex flex-wrap items-center gap-2">
        <Button
          variant="danger"
          type="submit"
          icon="ban"
          disabled={busy || closeReason.trim() === ''}
          loading={pending === 'wont_fix'}
        >
          Close as won't fix
        </Button>
        <Button variant="ghost" type="button" disabled={busy} onclick={cancelWontFix}>Cancel</Button>
      </div>
    </form>
  {/if}
  {#if card.status === 'blocked' && card.unblockTo === null}
    <Notice tone="danger">Accept the changed requirements in Intake before this card can be released.</Notice>
  {/if}
  {#if actions.includes('reopen')}
    <div class="flex flex-wrap items-center gap-3">
      <Button
        variant="primary"
        type="button"
        icon="refresh"
        disabled={busy}
        loading={pending === 'reopen'}
        onclick={() => void act('reopen', 'Reopened', () => reopenCard(card.id))}
      >
        Reopen
      </Button>
      <p class="text-sm text-muted">Reopen returns this card to Blocked.</p>
    </div>
  {/if}
</div>
