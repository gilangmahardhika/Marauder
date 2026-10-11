<script lang="ts">
  import { pageSize } from '$lib/filter';
  import Button from './Button.svelte';

  let {
    total,
    limit,
    noun,
    onmore
  }: { total: number; limit: number; noun: [string, string]; onmore: () => void } = $props();
</script>

<div class="flex flex-wrap items-center justify-between gap-3">
  <p class="text-sm text-muted">Showing {Math.min(limit, total)} of {total} {total === 1 ? noun[0] : noun[1]}</p>
  {#if total > limit}
    <Button type="button" variant="secondary" onclick={onmore}>Show {Math.min(pageSize, total - limit)} more</Button>
  {/if}
</div>
