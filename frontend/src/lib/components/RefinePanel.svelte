<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import RefineCompare from '$lib/components/RefineCompare.svelte';
  import { effortChoices, requestChoices, thinkingChoices } from '$lib/llm';
  import { toLines } from '$lib/requirements';
  import type { ModelChoice, RefineEvent, RefineField, RefineStream } from '$lib/types';
  import { autogrow } from '$lib/ui/autogrow';
  import Button from '$lib/ui/Button.svelte';
  import ChipSelect, { type ChipOption } from '$lib/ui/ChipSelect.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';
  import Icon from '$lib/ui/Icon.svelte';

  let {
    field,
    statement,
    acceptance,
    stream,
    models = [],
    implementModel = '',
    model = $bindable(''),
    thinking = $bindable(''),
    effort = $bindable(''),
    autostart = false,
    onapply,
    onclose
  }: {
    field: RefineField;
    /** The form's statement and acceptance text as typed. */
    statement: string;
    acceptance: string;
    stream: RefineStream;
    /** Models the member may choose (RFC-124). Without any, the composer shows no model choices. */
    models?: ModelChoice[];
    implementModel?: string;
    /** The model, thinking, and effort of the request, shared by both fields' panels. Empty keeps the defaults. */
    model?: string;
    thinking?: string;
    effort?: string;
    /** Sends an empty first message as soon as the panel opens. */
    autostart?: boolean;
    /** Gets the new text; for acceptance, the lines joined by newlines. */
    onapply: (text: string) => void;
    onclose?: () => void;
  } = $props();

  type Turn = {
    message: string;
    reply: string;
    modelId: string;
    calling: string;
    status: 'streaming' | 'done' | 'error' | 'stopped';
    error: string;
  };

  let turns = $state<Turn[]>([]);
  let draft = $state('');
  let notice = $state('');
  let thread = $state({ sent: '', before: '' });
  let controller: AbortController | null = null;

  const name = $derived(field === 'statement' ? 'statement' : 'acceptance');
  const current = $derived(field === 'statement' ? statement : acceptance);
  const streaming = $derived(turns.at(-1)?.status === 'streaming');
  const latest = $derived(turns.findLastIndex((turn) => turn.status === 'done'));
  const stale = $derived(latest >= 0 && current !== thread.sent);
  const emptyAsk = $derived(field === 'statement' ? 'Refine this statement' : 'Refine these acceptance lines');

  const reads = $derived.by(() => {
    if (field === 'statement') {
      const lines = toLines(acceptance).length;
      return lines === 0 ? 'No acceptance yet' : `Reads your acceptance: ${lines} ${lines === 1 ? 'line' : 'lines'}`;
    }
    const words = statement.trim() === '' ? 0 : statement.trim().split(/\s+/).length;
    return `Reads your statement: ${words} ${words === 1 ? 'word' : 'words'}`;
  });

  const modelOptions = $derived<ChipOption[]>([
    ...(models.some((choice) => choice.id === model) ? [] : [{ value: model, label: model || 'Implement model' }]),
    ...models.map((choice) => ({
      value: choice.id,
      label: choice.id,
      group: choice.family,
      tag: choice.id === implementModel ? 'Implement' : undefined
    }))
  ]);
  const protocol = $derived(models.find((choice) => choice.id === model)?.protocol ?? 'anthropic');
  const thinkings = $derived(requestChoices(thinkingChoices(protocol)));
  const efforts = $derived(
    requestChoices(effortChoices(protocol)).map((choice) => ({
      ...choice,
      label: choice.label.charAt(0).toUpperCase() + choice.label.slice(1)
    }))
  );

  function shown(choices: ChipOption[], value: string) {
    return value === '' ? 'Default' : (choices.find((choice) => choice.value === value)?.label ?? value);
  }

  function pick(next: string) {
    model = next;
    const nextProtocol = models.find((choice) => choice.id === next)?.protocol ?? 'anthropic';
    if (!thinkingChoices(nextProtocol).some((choice) => choice.value === thinking)) thinking = '';
    if (!effortChoices(nextProtocol).some((choice) => choice.value === effort)) effort = '';
  }

  onMount(() => {
    if (autostart) void send('');
  });

  onDestroy(() => controller?.abort());

  async function send(message: string) {
    notice = '';
    if (statement.trim() === '') {
      notice =
        field === 'statement'
          ? 'Write a statement first, then refine it.'
          : 'Write a statement first. Acceptance is refined against it.';
      return;
    }
    controller?.abort();
    const mine = new AbortController();
    controller = mine;
    const base = latest >= 0 ? turns[latest].reply : null;
    if (turns.length === 0) {
      thread = {
        sent: current,
        before: field === 'statement' ? statement.trim() : toLines(acceptance).join('\n')
      };
    }
    const body = {
      field,
      statement: field === 'statement' && base !== null ? base : statement.trim(),
      acceptance: field === 'acceptance' && base !== null ? (base === '' ? [] : base.split('\n')) : toLines(acceptance),
      instructions: message.trim(),
      model,
      thinking,
      effort
    };
    turns.push({ message: message.trim(), reply: '', modelId: '', calling: '', status: 'streaming', error: '' });
    const turn = turns[turns.length - 1];
    let ended = false;
    const onEvent = (event: RefineEvent) => {
      if (controller !== mine) return;
      if (event.kind === 'start') turn.modelId = event.modelId;
      else if (event.kind === 'delta') {
        turn.reply += event.text;
        turn.calling = '';
      } else if (event.kind === 'reset') turn.reply = '';
      else if (event.kind === 'tool') turn.calling = `${event.server} / ${event.tool}`;
      else if (event.kind === 'done') {
        ended = true;
        turn.reply = event.text;
        turn.modelId = event.modelId;
        turn.status = 'done';
      } else {
        ended = true;
        turn.error = event.message || 'The refine request failed.';
        turn.status = 'error';
      }
    };
    try {
      await stream(body, onEvent, mine.signal);
      if (controller === mine && !ended) {
        turn.error = 'The reply ended before it finished.';
        turn.status = 'error';
      }
    } catch (failure) {
      if (controller !== mine) return;
      turn.error = failure instanceof Error && failure.message ? failure.message : 'The refine request failed.';
      turn.status = 'error';
    } finally {
      if (controller === mine) controller = null;
    }
  }

  function submit() {
    if (streaming) return;
    const message = draft;
    if (statement.trim() !== '') draft = '';
    void send(message);
  }

  function retry(index: number) {
    const { message } = turns[index];
    turns.splice(index, 1);
    void send(message);
  }

  function stop() {
    const running = controller;
    controller = null;
    running?.abort();
    const turn = turns.at(-1);
    if (turn?.status === 'streaming') turn.status = 'stopped';
  }

  function clear() {
    stop();
    turns = [];
    notice = '';
  }

  function apply() {
    const text = turns[latest].reply;
    clear();
    onapply(text);
  }

  function onkeydown(event: KeyboardEvent) {
    if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return;
    event.preventDefault();
    submit();
  }
</script>

<section class="flex flex-col rounded-panel border border-line bg-surface shadow-xs" aria-label={`Refine ${name} chat`}>
  <header class="flex items-center gap-3 border-b border-line py-1 pr-1 pl-4">
    <span class="inline-flex size-8 shrink-0 items-center justify-center rounded-full bg-accent/10 text-accent">
      <Icon name="sparkles" class="size-4" />
    </span>
    <div class="flex min-w-0 flex-1 flex-col">
      <h3 class="text-sm font-semibold text-ink">Refine {name}</h3>
      <p class="truncate text-xs text-muted">{reads}</p>
    </div>
    {#if turns.length > 0}
      <Button variant="ghost" icon="plus" onclick={clear}>New chat</Button>
    {/if}
    {#if onclose}
      <button
        type="button"
        class="inline-flex size-11 shrink-0 items-center justify-center rounded-control text-muted hover:bg-ground hover:text-ink"
        aria-label="Close refine"
        onclick={onclose}
      >
        <Icon name="close" class="size-4" />
      </button>
    {/if}
  </header>

  {#if turns.length > 0}
    <ol class="flex flex-col gap-4 px-4 py-4">
      {#each turns as turn, index (index)}
        <li class="flex flex-col gap-3">
          <div class="flex justify-end">
            <p
              class="max-w-[80%] rounded-2xl rounded-br-md bg-accent/10 px-3.5 py-2 text-sm whitespace-pre-wrap text-ink"
            >
              {turn.message || emptyAsk}
            </p>
          </div>
          <div class="flex gap-3">
            <span
              class="mt-0.5 inline-flex size-7 shrink-0 items-center justify-center rounded-full bg-accent/10 text-accent"
              aria-hidden="true"
            >
              <Icon name="sparkles" class="size-3.5" />
            </span>
            <div class="flex min-w-0 flex-1 flex-col gap-2">
              {#if turn.status === 'done' && index === latest && !streaming}
                <RefineCompare
                  {field}
                  before={thread.before}
                  after={turn.reply}
                  modelId={turn.modelId}
                  {stale}
                  onapply={apply}
                  ondiscard={clear}
                />
              {:else}
                <p class="text-xs font-medium text-muted">{turn.modelId || model || 'Implement model'}</p>
                {#if turn.status === 'streaming'}
                  <p class="text-xs text-muted" role="status">
                    {turn.calling
                      ? `Looking up ${turn.calling}`
                      : turn.modelId
                        ? `Writing with ${turn.modelId}`
                        : model
                          ? `Waiting for ${model}…`
                          : 'Waiting for the implement model…'}
                  </p>
                  <div
                    class="min-h-11 rounded-2xl rounded-tl-md bg-ground/70 px-3.5 py-2.5 text-sm whitespace-pre-wrap text-ink"
                    aria-label={`Refined ${name} so far`}
                    aria-live="polite"
                    aria-busy="true"
                    role="region"
                  >
                    {turn.reply}<span
                      class="ml-0.5 inline-block h-4 w-1.5 translate-y-0.5 bg-ink motion-safe:animate-pulse"
                      aria-hidden="true"
                    ></span>
                  </div>
                {:else if turn.status === 'done'}
                  <p class="rounded-2xl rounded-tl-md bg-ground/70 px-3.5 py-2.5 text-sm whitespace-pre-wrap text-muted">
                    {turn.reply}
                  </p>
                {:else if turn.status === 'error'}
                  <FormAlert message={turn.error} />
                  <Button icon="refresh" onclick={() => retry(index)}>Try again</Button>
                {:else}
                  <p class="text-sm text-muted" role="status">Stopped. Nothing changed.</p>
                {/if}
              {/if}
            </div>
          </div>
        </li>
      {/each}
    </ol>
  {/if}

  <div class="flex flex-col gap-2 p-4 {turns.length > 0 ? 'border-t border-line' : ''}">
    <FormAlert message={notice} />
    <div class="rounded-2xl border border-line bg-surface shadow-xs transition-colors duration-150 focus-within:border-muted">
      <label for={`refine-${field}-instructions`} class="sr-only">What to improve (optional)</label>
      <textarea
        id={`refine-${field}-instructions`}
        rows="3"
        maxlength="1000"
        class="block max-h-72 min-h-24 w-full resize-none overflow-y-auto rounded-t-2xl bg-transparent px-4 pt-3.5 pb-2 text-sm leading-6 text-ink placeholder:text-muted/70 focus-visible:outline-hidden"
        placeholder="Ask for a change, or press Enter to refine as is"
        use:autogrow={{ on: true, value: draft }}
        bind:value={draft}
        {onkeydown}
      ></textarea>
      <div class="flex flex-wrap items-center gap-1 px-1.5 pb-1.5">
        {#if models.length > 0}
          <ChipSelect
            label="Model"
            id={`refine-${field}-model`}
            icon="cpu"
            display={model || 'Implement model'}
            disabled={streaming}
            options={modelOptions}
            value={model}
            onchange={pick}
          />
          <ChipSelect
            label="Thinking"
            prefix="Thinking"
            id={`refine-${field}-thinking`}
            display={shown(thinkings, thinking)}
            disabled={streaming}
            options={thinkings}
            bind:value={thinking}
          />
          <ChipSelect
            label="Effort"
            prefix="Effort"
            id={`refine-${field}-effort`}
            display={shown(efforts, effort)}
            disabled={streaming}
            options={efforts}
            bind:value={effort}
          />
        {/if}
        {#if streaming}
          <button
            type="button"
            class="ml-auto inline-flex size-11 shrink-0 items-center justify-center rounded-full bg-ink text-surface transition-colors duration-150 hover:bg-ink/85"
            aria-label="Stop"
            onclick={stop}
          >
            <Icon name="stop" class="size-4" />
          </button>
        {:else}
          <button
            type="button"
            class="ml-auto inline-flex size-11 shrink-0 items-center justify-center rounded-full bg-ink text-surface transition-colors duration-150 hover:bg-ink/85"
            aria-label="Refine"
            onclick={submit}
          >
            <Icon name="arrow-up" class="size-4" />
          </button>
        {/if}
      </div>
    </div>
  </div>
</section>
