<script lang="ts">
  import Markdown from '$lib/components/Markdown.svelte';
  import Icon from '$lib/ui/Icon.svelte';

  let { lines, label }: { lines: string[]; label: string } = $props();
</script>

<ul class="flex flex-col gap-1.5" aria-label={label}>
  {#each lines as line, index (index)}
    <li class="flex items-start gap-2 text-sm text-ink">
      <Icon name="check" class="mt-0.5 size-4 shrink-0 text-muted" />
      <Markdown inline text={line} />
    </li>
  {/each}
</ul>
