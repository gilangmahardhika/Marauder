<script lang="ts">
  import { answerQuestions, ApiError, getCard } from '$lib/api';
  import { failureText, focusInvalid, formFailure, noFailure } from '$lib/forms';
  import { displayName } from '$lib/people';
  import { toast } from '$lib/toast.svelte';
  import type { Card, CardQuestion, GivenAnswer, QuestionAnswer } from '$lib/types';
  import { autogrow } from '$lib/ui/autogrow';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { control, controlBase } from '$lib/ui/control';
  import FieldError from '$lib/ui/FieldError.svelte';
  import FormAlert from '$lib/ui/FormAlert.svelte';

  type Draft = { selected: string[]; other: boolean; text: string };

  const blank: Draft = { selected: [], other: false, text: '' };
  const missingText = 'Answer this question.';

  let { card = $bindable() }: { card: Card } = $props();

  let drafts = $state<Record<string, Draft>>({});
  let missing = $state<Record<string, string>>({});
  let failure = $state(noFailure);
  let pending = $state<'' | 'submit' | 'suggest'>('');
  let showEarlier = $state(false);
  let form = $state<HTMLFormElement>();

  const questions = $derived(card.questions ?? []);
  const open = $derived(questions.filter((q) => q.answer === null));
  const earlier = $derived(questions.filter((q) => q.answer !== null));
  const canAnswer = $derived((card.actions ?? []).includes('answer') && open.length > 0);
  const shown = $derived(open.map((_, index) => `answers.${index}`));

  function draftOf(id: string): Draft {
    return drafts[id] ?? blank;
  }

  function update(id: string, change: Partial<Draft>) {
    drafts[id] = { ...draftOf(id), ...change };
    if (missing[id]) missing[id] = '';
  }

  function pickSingle(id: string, option: string) {
    update(id, { selected: [option], other: false });
  }

  function toggleMultiple(q: CardQuestion, option: string, on: boolean) {
    const current = draftOf(q.id).selected;
    update(q.id, { selected: q.options.filter((o) => (o === option ? on : current.includes(o))) });
  }

  function answerOf(q: CardQuestion): QuestionAnswer {
    const d = draftOf(q.id);
    if (q.kind === 'text') return { selected: [], text: d.text };
    if (q.kind === 'single') return d.other ? { selected: [], text: d.text } : { selected: d.selected, text: '' };
    return { selected: d.selected, text: d.other ? d.text : '' };
  }

  function answered(answer: QuestionAnswer) {
    return answer.selected.length > 0 || answer.text.trim() !== '';
  }

  /** The suggestion or answer as one line: the options, then the text. */
  function answerLine(answer: QuestionAnswer) {
    return [...answer.selected, answer.text.trim()].filter(Boolean).join('; ');
  }

  function sourceBadge(q: CardQuestion) {
    if (q.answerSource === 'auto') return 'Auto';
    if (q.answerSource === 'suggestion') return "Agent's suggestion";
    return 'You';
  }

  async function send(body: { answers: GivenAnswer[] } | { useSuggestions: true }, which: 'submit' | 'suggest') {
    pending = which;
    failure = noFailure;
    try {
      card = await answerQuestions(card.id, body);
      drafts = {};
      missing = {};
      toast.success('Answers sent. The next attempt is queued.');
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        try {
          card = await getCard(card.id);
          toast.info('Card changed. Reloaded.');
        } catch (reloadError) {
          toast.error(failureText(reloadError));
        }
        return;
      }
      failure = formFailure(error, shown);
      void focusInvalid(form);
    } finally {
      pending = '';
    }
  }

  function submit(event: SubmitEvent) {
    event.preventDefault();
    const next: Record<string, string> = {};
    const answers = open.map((q) => {
      const answer = answerOf(q);
      if (!answered(answer)) next[q.id] = missingText;
      return { questionId: q.id, ...answer };
    });
    missing = next;
    failure = noFailure;
    if (Object.keys(next).length > 0) {
      void focusInvalid(form);
      return;
    }
    void send({ answers }, 'submit');
  }
</script>

{#snippet otherInput(q: CardQuestion, d: Draft, invalid: true | undefined, described: string | undefined)}
  <label for="question-{q.id}-other" class="sr-only">Your answer</label>
  <input
    id="question-{q.id}-other"
    type="text"
    placeholder="Your answer"
    class="{control} max-w-field-lg"
    aria-invalid={invalid}
    aria-describedby={described}
    value={d.text}
    oninput={(event) => update(q.id, { text: event.currentTarget.value })}
  />
{/snippet}

<section id="questions" class="flex flex-col gap-5">
  {#if canAnswer}
    <div>
      <p class="font-medium text-ink">
        {open.length === 1 ? 'The agent has a question' : `The agent has ${open.length} questions`}
      </p>
      <p class="mt-0.5 text-sm text-muted">Answer every question to queue the next attempt. It reads your answers.</p>
    </div>
    <form class="flex flex-col gap-6" onsubmit={submit} bind:this={form} novalidate>
      <FormAlert message={failure.message} extra={failure.extra} />
      {#each open as q, index (q.id)}
        {@const d = draftOf(q.id)}
        {@const error = missing[q.id] || failure.fields[`answers.${index}`] || ''}
        {@const errorId = `question-${q.id}`}
        {@const invalid = error ? true : undefined}
        {@const described = error ? `${errorId}-error` : undefined}
        <fieldset class="flex min-w-0 flex-col gap-2">
          <legend class="mb-1 text-sm font-medium wrap-break-word text-ink">{q.question}</legend>
          {#if q.kind === 'text'}
            <label for="{errorId}-text" class="sr-only">Your answer</label>
            <textarea
              id="{errorId}-text"
              rows="2"
              placeholder="Your answer"
              class="{controlBase} resize-none overflow-hidden py-2.5"
              aria-invalid={invalid}
              aria-describedby={described}
              value={d.text}
              oninput={(event) => update(q.id, { text: event.currentTarget.value })}
              use:autogrow={{ on: true, value: d.text }}
            ></textarea>
          {:else if q.kind === 'single'}
            <div
              role="radiogroup"
              aria-label={q.question}
              aria-invalid={invalid}
              aria-describedby={described}
              class="flex flex-col"
            >
              {#each q.options as option (option)}
                <label class="flex min-h-11 w-fit items-center gap-3">
                  <input
                    type="radio"
                    name={errorId}
                    class="size-4 shrink-0 accent-accent"
                    checked={!d.other && d.selected.includes(option)}
                    onchange={() => pickSingle(q.id, option)}
                  />
                  <span class="text-sm wrap-break-word text-ink">{option}</span>
                </label>
              {/each}
              {#if q.allowOther}
                <label class="flex min-h-11 w-fit items-center gap-3">
                  <input
                    type="radio"
                    name={errorId}
                    class="size-4 shrink-0 accent-accent"
                    checked={d.other}
                    onchange={() => update(q.id, { selected: [], other: true })}
                  />
                  <span class="text-sm text-ink">Other</span>
                </label>
              {/if}
            </div>
            {#if q.allowOther && d.other}{@render otherInput(q, d, invalid, described)}{/if}
          {:else}
            {#each q.options as option (option)}
              <label class="flex min-h-11 w-fit items-center gap-3">
                <input
                  type="checkbox"
                  class="size-4 shrink-0 accent-accent"
                  aria-invalid={invalid}
                  aria-describedby={described}
                  checked={d.selected.includes(option)}
                  onchange={(event) => toggleMultiple(q, option, event.currentTarget.checked)}
                />
                <span class="text-sm wrap-break-word text-ink">{option}</span>
              </label>
            {/each}
            {#if q.allowOther}
              <label class="flex min-h-11 w-fit items-center gap-3">
                <input
                  type="checkbox"
                  class="size-4 shrink-0 accent-accent"
                  aria-invalid={invalid}
                  aria-describedby={described}
                  checked={d.other}
                  onchange={(event) => update(q.id, { other: event.currentTarget.checked })}
                />
                <span class="text-sm text-ink">Other</span>
              </label>
              {#if d.other}{@render otherInput(q, d, invalid, described)}{/if}
            {/if}
          {/if}
          <p class="text-xs text-muted wrap-break-word">Agent suggests: {answerLine(q.recommended)}</p>
          {#if q.why}<p class="text-xs text-muted wrap-break-word">Why: {q.why}</p>{/if}
          <FieldError id={errorId} {error} />
        </fieldset>
      {/each}
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="primary" type="submit" icon="send" loading={pending === 'submit'} disabled={pending !== ''}>
          Submit answers
        </Button>
        <Button
          variant="secondary"
          loading={pending === 'suggest'}
          disabled={pending !== ''}
          onclick={() => void send({ useSuggestions: true }, 'suggest')}
        >
          Let the agent decide
        </Button>
      </div>
    </form>
  {:else if open.length > 0}
    <ul class="flex flex-col gap-3">
      {#each open as q (q.id)}
        <li class="flex flex-col gap-1">
          <p class="text-sm font-medium wrap-break-word text-ink">{q.question}</p>
          <p class="text-xs text-muted">Waiting for an answer</p>
        </li>
      {/each}
    </ul>
  {/if}

  {#if earlier.length > 0}
    <div class="flex flex-col gap-3">
      {#if canAnswer || open.length > 0}
        <h3 class="text-[13px] font-medium text-ink">Earlier questions</h3>
      {/if}
      {#if showEarlier}
        <ul class="flex flex-col divide-y divide-line">
          {#each earlier as q (q.id)}
            <li class="flex flex-col gap-1 py-3 first:pt-0 last:pb-0">
              <p class="text-sm font-medium wrap-break-word text-ink">{q.question}</p>
              <p class="text-sm wrap-break-word text-ink">{q.answer ? answerLine(q.answer) : ''}</p>
              <p class="flex flex-wrap items-center gap-2 text-xs text-muted">
                <Badge tone={q.answerSource === 'person' ? 'green' : 'default'}>{sourceBadge(q)}</Badge>
                {#if q.answerSource !== 'auto' && q.answeredBy}<span>{displayName(q.answeredBy)}</span>{/if}
              </p>
            </li>
          {/each}
        </ul>
      {:else}
        <Button variant="ghost" icon="chevron" onclick={() => (showEarlier = true)}>
          {earlier.length === 1 ? 'Show 1 earlier question' : `Show ${earlier.length} earlier questions`}
        </Button>
      {/if}
    </div>
  {/if}
</section>
