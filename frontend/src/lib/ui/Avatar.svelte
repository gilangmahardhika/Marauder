<script lang="ts">
  import { displayName, type Person } from '$lib/people';
  import { initials } from '$lib/roles';

  let { person, size = 'md' }: { person: Person | null; size?: 'sm' | 'md' } = $props();

  const box = $derived(size === 'sm' ? 'size-5 text-[9px]' : 'size-8 text-xs');
</script>

{#if person}
  <span
    class="flex shrink-0 items-center justify-center rounded-full bg-sidebar font-medium text-ink {box}"
    aria-hidden="true">{initials(displayName(person))}</span
  >
{:else}
  <span class="shrink-0 rounded-full border border-dashed border-line-strong {box}" aria-hidden="true"></span>
{/if}
