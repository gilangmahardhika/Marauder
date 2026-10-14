<script lang="ts">
  import { reviewModeHints, reviewModes } from '$lib/requirements';
  import type { AIReviewMode } from '$lib/types';
  import { labelClass } from '$lib/ui/control';
  import FieldError from '$lib/ui/FieldError.svelte';
  import Segmented from '$lib/ui/Segmented.svelte';

  let {
    value = $bindable('continue'),
    error = ''
  }: { value?: AIReviewMode; error?: string } = $props();

  const id = 'ai-review-mode';
  const label = 'When the AI review asks for changes';
</script>

<div class="flex flex-col gap-1.5">
  <span class={labelClass}>{label}</span>
  <p class="text-xs text-muted">{reviewModeHints[value]}</p>
  <div aria-invalid={error ? true : undefined} aria-describedby={error ? `${id}-error` : undefined}>
    <Segmented {label} options={reviewModes} {value} onchange={(next) => (value = next as AIReviewMode)} />
  </div>
  <FieldError {id} {error} />
</div>
