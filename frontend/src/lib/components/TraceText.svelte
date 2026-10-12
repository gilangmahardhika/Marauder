<script lang="ts">
  import { bytes, count } from '$lib/format';
  import { cleanText } from '$lib/trace';
  import Button from '$lib/ui/Button.svelte';

  let { text, cap = 4000 }: { text: string; cap?: number } = $props();

  let all = $state(false);
  let reveal = $state(false);

  const clean = $derived(cleanText(text, cap));
  const shown = $derived(all ? clean.full : clean.text);
</script>

{#if clean.binary && !reveal}
  <div class="flex flex-wrap items-center gap-3 text-sm text-muted">
    <span>Binary output ({bytes(clean.bytes)}) is hidden.</span>
    <Button type="button" variant="ghost" onclick={() => (reveal = true)}>Show anyway</Button>
  </div>
{:else}
  <pre class="max-h-[32rem] overflow-auto rounded-control border border-line bg-ground/60 px-3 py-2 font-mono text-xs leading-relaxed whitespace-pre-wrap break-words text-ink">{shown}</pre>
  {#if clean.cut && !all}
    <div class="flex flex-wrap items-center gap-3 text-xs text-muted">
      <span>Showing the first {count(cap)} characters of {bytes(clean.bytes)}.</span>
      <Button type="button" variant="ghost" onclick={() => (all = true)}>Show all ({bytes(clean.bytes)})</Button>
    </div>
  {/if}
{/if}
