<script lang="ts" module>
  import type { FamilyDraft } from '$lib/llm';

  /** The API field keys an input on this form shows; model errors need a model row. */
  export function familyFields(family: FamilyDraft) {
    const base = ['id', 'protocol', 'baseUrl', 'apiKey'];
    return family.models.length > 0 ? [...base, 'models', 'modelOptions'] : base;
  }
</script>

<script lang="ts">
  import SecretField from '$lib/components/SecretField.svelte';
  import { blankOptions, effortChoices, thinkingChoices, usesBudget, type OptionsDraft } from '$lib/llm';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FieldGroup from '$lib/ui/FieldGroup.svelte';
  import Select from '$lib/ui/Select.svelte';

  type Control = 'thinking' | 'budget' | 'effort' | 'history' | 'context';

  let {
    family = $bindable(),
    idLocked = false,
    fields = {}
  }: { family: FamilyDraft; idLocked?: boolean; fields?: Record<string, string> } = $props();

  const thinking = $derived(thinkingChoices(family.protocol));
  const efforts = $derived(effortChoices(family.protocol));

  // The API names no row, so a models error goes to the first empty or repeated id.
  const modelRow = $derived.by(() => {
    const ids = family.models.map((model) => model.trim());
    const bad = ids.findIndex((id, index) => id === '' || ids.indexOf(id) !== index);
    return bad === -1 ? 0 : bad;
  });

  // A modelOptions error goes to the control it is about, on the row its message
  // names, else the first row where that control is set.
  const optionsError = $derived.by(() => {
    const message = fields.modelOptions ?? '';
    if (!message) return null;
    const control: Control = /context window/i.test(message)
      ? 'context'
      : /history/i.test(message)
        ? 'history'
        : /budget/i.test(message)
          ? 'budget'
          : /effort/i.test(message)
            ? 'effort'
            : 'thinking';
    const named = family.models.findIndex((model) => model.trim() !== '' && message.includes(model.trim()));
    const set = family.options.findIndex((options) => isSet(control, options));
    const row = named !== -1 ? named : set !== -1 ? set : 0;
    const shown = control !== 'budget' || usesBudget(family.protocol, family.options[row]?.thinking ?? '');
    return { row, control: shown ? control : 'thinking', message };
  });

  function isSet(control: Control, options: OptionsDraft) {
    if (control === 'budget') return usesBudget(family.protocol, options.thinking) && options.thinkingBudget !== '';
    if (control === 'history') return options.historyBudgetKiB !== '';
    if (control === 'context') return options.contextTokens !== '';
    return options[control] !== '';
  }

  function optionError(index: number, control: Control) {
    return optionsError?.row === index && optionsError.control === control ? optionsError.message : '';
  }

  function addModel() {
    family.models = [...family.models, ''];
    family.options = [...family.options, blankOptions()];
  }

  function removeModel(index: number) {
    family.models = family.models.filter((_, item) => item !== index);
    family.options = family.options.filter((_, item) => item !== index);
  }
</script>

<div class="grid gap-5 sm:grid-cols-2">
  <Field
    label="Id"
    id="family-id"
    width="full"
    mono
    disabled={idLocked}
    hint={idLocked ? 'The id keys the saved API key, so it cannot change.' : 'For example anthropic or openai-eu.'}
    error={fields.id}
    bind:value={family.id}
  />
  <Select label="Protocol" id="family-protocol" width="full" error={fields.protocol} bind:value={family.protocol}>
    <option value="anthropic">Anthropic</option>
    <option value="openai">OpenAI</option>
  </Select>
</div>
<Field label="Base URL" id="family-url" width="lg" mono placeholder="https://" error={fields.baseUrl} bind:value={family.baseUrl} />
<FieldGroup label="Models" hint="One model id per row, with the thinking and effort every request to it sends. The history budget is how much earlier tool output an implement attempt sends back, from 4 to 512 KiB.">
  {#if family.models.length > 0}
    <div class="max-w-field-lg overflow-hidden rounded-control border border-line bg-surface">
      {#each family.models as model, index (index)}
        <div class="border-b border-line last:border-b-0">
          <div class="p-3">
            <Field
              label={`Model ${index + 1}`}
              id={`model-${index}`}
              width="full"
              mono
              error={index === modelRow ? fields.models : ''}
              bind:value={family.models[index]}
            >
              {#snippet labelAction()}
                <Button
                  type="button"
                  variant="ghost"
                  icon="trash"
                  aria-label={`Remove ${model || `Model ${index + 1}`}`}
                  onclick={() => removeModel(index)}>Remove</Button
                >
              {/snippet}
            </Field>
          </div>
          {#if family.options[index]}
            <div
              role="group"
              aria-label={`Settings for ${model || `model ${index + 1}`}`}
              class="grid gap-3 border-t border-line bg-ground/40 px-3 py-3 sm:grid-cols-3"
            >
              <Select
                label="Thinking"
                id={`model-${index}-thinking`}
                width="full"
                error={optionError(index, 'thinking')}
                bind:value={family.options[index].thinking}
              >
                {#each thinking as choice (choice.value)}
                  <option value={choice.value}>{choice.label}</option>
                {/each}
              </Select>
              {#if usesBudget(family.protocol, family.options[index].thinking)}
                <Field
                  label="Budget"
                  id={`model-${index}-budget`}
                  type="number"
                  width="full"
                  mono
                  placeholder="8192"
                  error={optionError(index, 'budget')}
                  bind:value={family.options[index].thinkingBudget}
                />
              {/if}
              <Select
                label="Effort"
                id={`model-${index}-effort`}
                width="full"
                error={optionError(index, 'effort')}
                bind:value={family.options[index].effort}
              >
                {#each efforts as choice (choice.value)}
                  <option value={choice.value}>{choice.label}</option>
                {/each}
              </Select>
              <Field
                label="History budget (KiB)"
                id={`model-${index}-history`}
                type="number"
                width="full"
                mono
                placeholder="16"
                error={optionError(index, 'history')}
                bind:value={family.options[index].historyBudgetKiB}
              />
              <Field
                label="Context window (tokens)"
                id={`model-${index}-context`}
                type="number"
                width="full"
                mono
                placeholder="128000"
                hint="The agent compacts its work at 75% of this."
                error={optionError(index, 'context')}
                bind:value={family.options[index].contextTokens}
              />
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
  <Button type="button" variant="ghost" icon="plus" onclick={addModel}>Add model</Button>
</FieldGroup>
<SecretField label="API key" name="apiKey" saved={family.keySet} error={fields.apiKey} bind:value={family.key} bind:clear={family.clear} />
