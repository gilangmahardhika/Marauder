<script lang="ts">
  import SecretField from '$lib/components/SecretField.svelte';
  import { blankOptions, effortChoices, thinkingChoices, usesBudget, type FamilyDraft } from '$lib/llm';
  import Button from '$lib/ui/Button.svelte';
  import Field from '$lib/ui/Field.svelte';
  import FieldGroup from '$lib/ui/FieldGroup.svelte';
  import Icon from '$lib/ui/Icon.svelte';
  import Row from '$lib/ui/Row.svelte';
  import Select from '$lib/ui/Select.svelte';

  let {
    family = $bindable(),
    idLocked = false,
    fields = {}
  }: { family: FamilyDraft; idLocked?: boolean; fields?: Record<string, string> } = $props();

  const thinking = $derived(thinkingChoices(family.protocol));
  const efforts = $derived(effortChoices(family.protocol));

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
  <Select label="Protocol" id="family-protocol" width="full" bind:value={family.protocol}>
    <option value="anthropic">Anthropic</option>
    <option value="openai">OpenAI</option>
  </Select>
</div>
<Field label="Base URL" id="family-url" width="lg" mono placeholder="https://" error={fields.baseUrl} bind:value={family.baseUrl} />
<FieldGroup label="Models" hint="One model id per row, with the thinking and effort every request to it sends. The history budget is how much earlier tool output an implement attempt sends back, from 4 to 512 KiB.">
  {#if family.models.length > 0}
    <div class="max-w-field-lg overflow-hidden rounded-control border border-line bg-surface">
      {#each family.models as model, index}
        <div class="border-b border-line last:border-b-0">
          <Row id={`model-${index}`} label={`Model ${index + 1}`} bind:value={family.models[index]} onremove={() => removeModel(index)} />
          {#if family.options[index]}
            <div
              role="group"
              aria-label={`Settings for ${model || `model ${index + 1}`}`}
              class="grid gap-3 bg-ground/40 px-3 py-3 sm:grid-cols-3"
            >
              <Select label="Thinking" id={`model-${index}-thinking`} width="full" bind:value={family.options[index].thinking}>
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
                  bind:value={family.options[index].thinkingBudget}
                />
              {/if}
              <Select label="Effort" id={`model-${index}-effort`} width="full" bind:value={family.options[index].effort}>
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
                bind:value={family.options[index].historyBudgetKiB}
              />
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
  <Button type="button" variant="ghost" onclick={addModel}>
    <Icon name="plus" />
    Add model
  </Button>
  {#if fields.models}<p class="error text-xs">{fields.models}</p>{/if}
  {#if fields.modelOptions}<p class="error text-xs">{fields.modelOptions}</p>{/if}
</FieldGroup>
<SecretField label="API key" name="apiKey" saved={family.keySet} error={fields.apiKey} bind:value={family.key} bind:clear={family.clear} />
