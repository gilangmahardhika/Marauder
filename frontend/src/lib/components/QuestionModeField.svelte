<script lang="ts">
  import { setQuestionMode } from '$lib/api';
  import { failureText } from '$lib/forms';
  import { toast } from '$lib/toast.svelte';
  import type { Card, QuestionMode } from '$lib/types';
  import { labelClass } from '$lib/ui/control';
  import Segmented from '$lib/ui/Segmented.svelte';

  const label = 'When the agent has a question';
  const modes = [
    { value: 'manual', label: 'A person answers' },
    { value: 'auto', label: 'Agent decides' }
  ];
  const hints: Record<QuestionMode, string> = {
    manual: 'The card waits and the owner is notified.',
    auto: 'The agent picks its own suggested answer and keeps going.'
  };

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
  <div class="flex flex-col gap-1.5">
    <span class={labelClass}>{label}</span>
    <Segmented {label} options={modes} value={mode} disabled={saving} onchange={(next) => void change(next)} />
    <p class="text-xs text-muted">{hints[mode]}</p>
  </div>
{:else}
  <p class="text-sm text-ink">Questions: {modes.find((m) => m.value === mode)?.label}</p>
{/if}
