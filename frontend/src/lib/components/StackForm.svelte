<script lang="ts">
  import type { StackDraft } from '$lib/conventions';
  import Checkbox from '$lib/ui/Checkbox.svelte';
  import Field from '$lib/ui/Field.svelte';

  let { stack = $bindable(), fields = {} }: { stack: StackDraft; fields?: Record<string, string> } = $props();
</script>

<div class="flex flex-col gap-5">
  <Field label="Name" id="stack-name" width="md" bind:value={stack.name} error={fields.name ?? ''} />
  <Field
    label="Marker files"
    id="stack-files"
    mono
    width="lg"
    placeholder="go.mod, tsconfig.json"
    hint="File names or globs, comma separated, no folders. A match anywhere in the top three levels of the repository detects the stack."
    bind:value={stack.files}
    error={fields['detect.files'] ?? ''}
  />
  <Field
    label="npm dependencies"
    id="stack-npm"
    mono
    width="lg"
    placeholder="@sveltejs/kit"
    hint="Package names from any package.json, comma separated."
    bind:value={stack.npmDeps}
    error={fields['detect.npmDeps'] ?? ''}
  />
  <Field
    label="Python dependencies"
    id="stack-py"
    mono
    width="lg"
    placeholder="django"
    hint="Package names from pyproject.toml or requirements files, comma separated."
    bind:value={stack.pyDeps}
    error={fields['detect.pyDeps'] ?? ''}
  />
  <Checkbox
    id="stack-disabled"
    label="Disabled"
    hint="A disabled stack is not detected and its rules leave every pack."
    bind:checked={stack.disabled}
  />
</div>
