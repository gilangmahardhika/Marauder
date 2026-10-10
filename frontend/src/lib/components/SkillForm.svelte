<script lang="ts">
  import type { SkillInput } from '$lib/types';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import Field from '$lib/ui/Field.svelte';

  let { skill = $bindable(), fields = {} }: { skill: SkillInput; fields?: Record<string, string> } = $props();
</script>

<div class="flex flex-col gap-5">
  <div class="grid gap-5 sm:grid-cols-2">
    <Field
      label="Slug"
      id="skill-slug"
      mono
      width="full"
      placeholder="oauth2-integration"
      hint="Lowercase letters, digits, and dashes. The planner names the skill by it."
      bind:value={skill.slug}
      error={fields.slug ?? ''}
    />
    <Field label="Title" id="skill-title" width="full" bind:value={skill.title} error={fields.title ?? ''} />
  </div>
  <Field
    label="Use when"
    id="skill-use-when"
    type="textarea"
    rows={3}
    width="full"
    hint="One or two sentences the planner reads to decide whether a card needs this skill. At most 500 characters."
    bind:value={skill.useWhen}
    error={fields.useWhen ?? ''}
  />
  <Field
    label="Procedure"
    id="skill-body"
    type="textarea"
    mono
    rows={18}
    width="full"
    hint="Steps, configuration, security rules, and a test checklist. Plain text, at most 8000 characters."
    bind:value={skill.body}
    error={fields.body ?? ''}
  />
  <Checkbox
    id="skill-disabled"
    label="Disabled"
    hint="A disabled skill cannot be enabled or attached and leaves every pack."
    bind:checked={skill.disabled}
  />
</div>
