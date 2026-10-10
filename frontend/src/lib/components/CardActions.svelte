<script lang="ts">
  import { ApiError, approveCard, getCard, rejectCard, requestChanges, unblockCard } from '$lib/api';
  import { live } from '$lib/live';
  import type { Card } from '$lib/types';

  let { card = $bindable() }: { card: Card } = $props();
  let sha = $state(card.headSha ?? '');
  let reset = $state(false);
  let shaError = $state('');
  let busy = $state(false);

  const actions = $derived(card.actions ?? []);

  async function run(action: string, work: () => Promise<Card>) {
    busy = true;
    shaError = '';
    live.set(action);
    try {
      card = await work();
      sha = card.headSha ?? sha;
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        card = await getCard(card.id);
        live.set('Card changed. Reloaded.');
        return;
      }
      if (error instanceof ApiError && error.fields.sha) shaError = error.fields.sha;
      live.set(error instanceof Error ? error.message : 'API unreachable at http://127.0.0.1:8787');
    } finally {
      busy = false;
    }
  }
</script>

{#if actions.includes('approve')}
  <form
    class="form"
    onsubmit={(event) => {
      event.preventDefault();
      void run('Approve SHA', () => approveCard(card.id, sha));
    }}
  >
    <label for="sha">
      SHA
      <input id="sha" name="sha" bind:value={sha} />
    </label>
    {#if shaError}<p class="error">{shaError}</p>{/if}
    <button class="primary" type="submit" disabled={busy}>Approve SHA</button>
  </form>
{/if}
{#if actions.includes('request_changes')}
  <button type="button" disabled={busy} onclick={() => void run('Request changes', () => requestChanges(card.id))}>
    Request changes
  </button>
{/if}
{#if actions.includes('reject')}
  <button type="button" disabled={busy} onclick={() => void run('Reject', () => rejectCard(card.id))}>Reject</button>
{/if}
{#if actions.includes('unblock')}
  <form
    class="form"
    onsubmit={(event) => {
      event.preventDefault();
      void run('Unblock', () => unblockCard(card.id, reset));
    }}
  >
    <p>
      {#if card.unblockTo === 'human_review'}
        Unblock returns this card to human review. Approve the head again to redeploy.
      {:else}
        Unblock returns this card to Ready.
      {/if}
    </p>
    <label for="reset">
      <input id="reset" type="checkbox" bind:checked={reset} />
      Reset attempt counter
    </label>
    <button class="primary" type="submit" disabled={busy}>Unblock</button>
  </form>
{/if}
{#if card.status === 'blocked' && card.unblockTo === null}
  <p>Accept the changed requirements in Intake before this card can be released.</p>
{/if}
