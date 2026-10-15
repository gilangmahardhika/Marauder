<script lang="ts">
  import { setQuestionMode } from '$lib/api';
  import { failureText } from '$lib/forms';
  import { toast } from '$lib/toast.svelte';
  import type { Card, QuestionMode } from '$lib/types';
  import ChoiceCards, { type Choice } from '$lib/ui/ChoiceCards.svelte';
  import { labelClass } from '$lib/ui/control';

  const label = 'When the agent has a question';
  const modes: Choice[] = [
    { value: 'manual', label: 'A person answers', description: 'The card waits and the owner is notified.' },
    { value: 'auto', label: 'Agent decides', description: 'The agent picks its own suggested answer and keeps going.' }
  ];

  let { card = $bindable() }: { card: Card } = $props();

  let saving = $state(false);

  const mode = $derived<QuestionMode>(card.questionMode === 'auto' ? 'auto' : 'manual');
  const canChange = $derived((card.actions ?? []).includes('question_mode'));

  async function change(next: string) {
    if (next === mode || saving) return;
    saving = true;
    try {
      card = await setQuestionMode(card.id, next as QuestionMode);
      toast.success(next === 'auto' ? 'The agent decides its questions' : 'A person answers the questions');
    } catch (error) {
      toast.error(failureText(error));
    } finally {
      saving = false;
    }
  }
</script>

{#if canChange}
  <div class="flex flex-col gap-2">
    <span class={labelClass}>{label}</span>
    <ChoiceCards {label} id="question-mode" options={modes} value={mode} disabled={saving} onchange={(next) => void change(next)} />
  </div>
{:else}
  <p class="text-sm text-ink">Questions: {modes.find((m) => m.value === mode)?.label}</p>
{/if}
