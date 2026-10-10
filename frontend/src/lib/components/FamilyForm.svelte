<script lang="ts">
  import SecretField from '$lib/components/SecretField.svelte';
  import type { FamilyDraft } from '$lib/llm';
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
<FieldGroup label="Models" hint="One model id per row.">
  {#if family.models.length > 0}
    <div class="max-w-field-lg overflow-hidden rounded-control border border-line bg-surface">
      {#each family.models as _, index}
        <Row
          id={`model-${index}`}
          label={`Model ${index + 1}`}
          bind:value={family.models[index]}
          onremove={() => (family.models = family.models.filter((_, item) => item !== index))}
        />
      {/each}
    </div>
  {/if}
  <Button type="button" variant="ghost" onclick={() => (family.models = [...family.models, ''])}>
    <Icon name="plus" />
    Add model
  </Button>
  {#if fields.models}<p class="error text-xs">{fields.models}</p>{/if}
</FieldGroup>
<SecretField label="API key" name="apiKey" saved={family.keySet} error={fields.apiKey} bind:value={family.key} bind:clear={family.clear} />
