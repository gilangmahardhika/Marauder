<script lang="ts">
  import { iconPaths, type IconName } from './icons';

  let { name, class: className = 'size-4' }: { name: IconName; class?: string } = $props();
</script>

<svg class={className} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" aria-hidden="true">
  {#each iconPaths[name] as d (d)}
    <path {d} stroke-linejoin="round" stroke-linecap="round" />
  {/each}
</svg>
