<script lang="ts">
  import { reviewModeHints, reviewModes } from '$lib/requirements';
  import type { AIReviewMode } from '$lib/types';
  import { labelClass } from '$lib/ui/control';
  import Segmented from '$lib/ui/Segmented.svelte';

  let { value = $bindable('continue') }: { value?: AIReviewMode } = $props();

  const label = 'When the AI review asks for changes';
</script>

<div class="flex flex-col gap-1.5">
  <span class={labelClass}>{label}</span>
  <div>
    <Segmented {label} options={reviewModes} {value} onchange={(next) => (value = next as AIReviewMode)} />
  </div>
  <p class="text-xs text-muted">{reviewModeHints[value]}</p>
</div>
