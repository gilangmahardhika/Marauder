<script lang="ts">
  import { onDestroy } from 'svelte';
  import RefineCompare from '$lib/components/RefineCompare.svelte';
  import { toLines } from '$lib/requirements';
  import type { RefineEvent, RefineField, RefineStream } from '$lib/types';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';

  let {
    field,
    statement,
    acceptance,
    stream,
    onapply
  }: {
    field: RefineField;
    /** The form's statement and acceptance text as typed. */
    statement: string;
    acceptance: string;
    stream: RefineStream;
    /** Gets the new text; for acceptance, the lines joined by newlines. */
    onapply: (text: string) => void;
  } = $props();

  let instructions = $state('');
  let phase = $state<'idle' | 'streaming' | 'done' | 'error'>('idle');
  let text = $state('');
  let modelId = $state('');
  let error = $state('');
  let note = $state('');
  let calling = $state('');
  let asked = $state({ sent: '', before: '' });
  let result = $state({ text: '', modelId: '' });
  let controller: AbortController | null = null;

  const current = $derived(field === 'statement' ? statement : acceptance);
  const stale = $derived(phase === 'done' && current !== asked.sent);
  const name = $derived(field === 'statement' ? 'statement' : 'acceptance');

  onDestroy(() => controller?.abort());

  async function ask() {
    error = '';
    note = '';
    if (statement.trim() === '') {
      phase = 'error';
      error =
        field === 'statement'
          ? 'Write a statement first, then refine it.'
          : 'Write a statement first. Acceptance is refined against it.';
      return;
    }
    controller?.abort();
    const mine = new AbortController();
    controller = mine;
    asked = {
      sent: current,
      before: field === 'statement' ? statement.trim() : toLines(acceptance).join('\n')
    };
    text = '';
    modelId = '';
    calling = '';
    phase = 'streaming';
    let ended = false;
    const onEvent = (event: RefineEvent) => {
      if (controller !== mine) return;
      if (event.kind === 'start') modelId = event.modelId;
      else if (event.kind === 'delta') {
        text += event.text;
        calling = '';
      } else if (event.kind === 'reset') text = '';
      else if (event.kind === 'tool') calling = `${event.server} / ${event.tool}`;
      else if (event.kind === 'done') {
        ended = true;
        result = { text: event.text, modelId: event.modelId };
        phase = 'done';
      } else {
        ended = true;
        error = event.message || 'The refine request failed.';
        phase = 'error';
      }
    };
    try {
      await stream(
        { field, statement: statement.trim(), acceptance: toLines(acceptance), instructions: instructions.trim() },
        onEvent,
        mine.signal
      );
      if (controller === mine && !ended) {
        error = 'The reply ended before it finished.';
        phase = 'error';
      }
    } catch (failure) {
      if (controller !== mine) return;
      error = failure instanceof Error && failure.message ? failure.message : 'The refine request failed.';
      phase = 'error';
    } finally {
      if (controller === mine) controller = null;
    }
  }

  function stop() {
    const running = controller;
    controller = null;
    running?.abort();
    phase = 'idle';
    note = 'Stopped. Nothing changed.';
  }

  function apply() {
    onapply(result.text);
    phase = 'idle';
  }
</script>

<div class="flex flex-col gap-3 rounded-control border border-line bg-ground/40 p-3">
  {#if phase !== 'done'}
    <div class="flex flex-wrap items-end gap-3">
      <div class="min-w-0 flex-1">
        <Field
          label="What to improve (optional)"
          id={`refine-${field}-instructions`}
          width="full"
          placeholder={field === 'statement' ? 'For example: say who needs it and why' : 'For example: one testable check per line'}
          disabled={phase === 'streaming'}
          bind:value={instructions}
        />
      </div>
      {#if phase === 'streaming'}
        <Button onclick={stop}>Stop</Button>
      {:else}
        <Button icon={phase === 'error' ? 'refresh' : 'spark'} onclick={() => void ask()}>{phase === 'error' ? 'Try again' : 'Refine'}</Button>
      {/if}
    </div>
  {/if}

  {#if phase === 'streaming'}
    <p class="text-xs text-muted" role="status">
      {calling ? `Looking up ${calling}` : modelId ? `Writing with ${modelId}` : 'Waiting for the implement model…'}
    </p>
    <div
      class="min-h-16 rounded-control border border-line bg-surface px-3 py-2.5 text-sm whitespace-pre-wrap text-ink"
      aria-label={`Refined ${name} so far`}
      aria-live="polite"
      aria-busy="true"
      role="region"
    >
      {text}<span class="ml-0.5 inline-block h-4 w-1.5 translate-y-0.5 bg-ink motion-safe:animate-pulse" aria-hidden="true"></span>
    </div>
  {:else if phase === 'error'}
    <FormAlert message={error} />
  {:else if phase === 'done'}
    <RefineCompare
      {field}
      before={asked.before}
      after={result.text}
      modelId={result.modelId}
      {stale}
      onapply={apply}
      ondiscard={() => (phase = 'idle')}
    />
  {:else if note}
    <p class="text-xs text-muted" role="status">{note}</p>
  {/if}
</div>
