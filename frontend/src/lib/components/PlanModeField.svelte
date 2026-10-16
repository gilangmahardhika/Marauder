<script lang="ts">
  import { setPlanMode } from '$lib/api';
  import { failureText } from '$lib/forms';
  import { toast } from '$lib/toast.svelte';
  import type { Card, PlanMode } from '$lib/types';
  import ChoiceCards, { type Choice } from '$lib/ui/ChoiceCards.svelte';
  import { labelClass } from '$lib/ui/control';

  const label = 'Before the agent codes';
  const modes: Choice[] = [
    {
      value: 'required',
      label: 'Plan before coding',
      description: 'The agent submits a plan; the card waits on Needs answer until you accept it.'
    },
    {
      value: 'off',
      label: 'Skip planning',
      description: 'The agent may edit files on the first attempt without a plan gate.'
    }
  ];

  let { card = $bindable() }: { card: Card } = $props();

  let saving = $state(false);

  const mode = $derived<PlanMode>(card.planMode === 'off' ? 'off' : 'required');
  const canChange = $derived((card.actions ?? []).includes('plan_mode'));

  async function change(next: string) {
    if (next === mode || saving) return;
    saving = true;
    try {
      card = await setPlanMode(card.id, next as PlanMode);
      toast.success(next === 'off' ? 'Planning skipped for this card' : 'Plan required before coding');
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
    <ChoiceCards {label} id="plan-mode" options={modes} value={mode} disabled={saving} onchange={(next) => void change(next)} />
  </div>
{:else}
  <p class="text-sm text-ink">Plan: {modes.find((m) => m.value === mode)?.label}</p>
{/if}
