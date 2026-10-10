<script lang="ts">
  import { categories } from '$lib/conventions';
  import type { RuleInput, Stack } from '$lib/types';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import Field from '$lib/ui/Field.svelte';
  import Select from '$lib/ui/Select.svelte';

  let {
    rule = $bindable(),
    stacks,
    fields = {}
  }: { rule: RuleInput; stacks: Stack[]; fields?: Record<string, string> } = $props();
</script>

<div class="flex flex-col gap-5">
  <div class="grid gap-5 sm:grid-cols-2">
    <Select label="Applies to" id="rule-scope" bind:value={rule.scope} error={fields.scope ?? ''} width="full">
      <option value="global">Every project</option>
      <option value="stack">Projects with one stack</option>
    </Select>
    <Select label="Category" id="rule-category" bind:value={rule.category} error={fields.category ?? ''} width="full">
      {#each categories as item (item.value)}
        <option value={item.value}>{item.label}</option>
      {/each}
    </Select>
  </div>
  {#if rule.scope === 'stack'}
    <Select label="Stack" id="rule-stack" bind:value={rule.stackId} error={fields.stackId ?? ''}>
      <option value="" disabled>Choose a stack</option>
      {#each stacks as stack (stack.id)}
        <option value={stack.id}>{stack.name}{stack.disabled ? ' (disabled)' : ''}</option>
      {/each}
    </Select>
  {/if}
  <Field label="Title" id="rule-title" width="lg" bind:value={rule.title} error={fields.title ?? ''} />
  <Field
    label="Rule"
    id="rule-body"
    type="textarea"
    mono
    rows={14}
    width="full"
    hint="Plain text, at most 8000 characters. The implementer and reviewer read it as data. It cannot add a tool or a command."
    bind:value={rule.body}
    error={fields.body ?? ''}
  />
  <Checkbox id="rule-disabled" label="Disabled" hint="A disabled rule stays in the catalog and leaves every pack." bind:checked={rule.disabled} />
</div>
