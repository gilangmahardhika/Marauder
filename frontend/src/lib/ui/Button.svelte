<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    variant = 'secondary',
    type = 'button',
    disabled = false,
    full = false,
    href,
    form,
    onclick,
    children
  }: {
    variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
    type?: 'button' | 'submit';
    disabled?: boolean;
    full?: boolean;
    href?: string;
    form?: string;
    onclick?: (event: MouseEvent) => void;
    children: Snippet;
  } = $props();

  const look = $derived(
    variant === 'primary'
      ? 'border border-accent bg-accent text-surface hover:bg-[#183d2e]'
      : variant === 'danger'
        ? 'border border-red-200 bg-surface text-red-800 hover:bg-red-50'
        : variant === 'ghost'
          ? 'border border-transparent bg-transparent text-muted hover:bg-ground hover:text-ink'
          : 'border border-line bg-surface text-ink hover:border-line-strong hover:bg-ground/60'
  );

  const base = $derived(
    `inline-flex min-h-11 items-center justify-center gap-2 whitespace-nowrap rounded-control px-4 text-sm font-medium ${full ? 'w-full' : 'w-fit shrink-0'}`
  );
</script>

{#if href && !disabled}
  <a {href} class="{base} {look}">
    {@render children()}
  </a>
{:else}
  <button {type} {disabled} {form} {onclick} class="{base} disabled:opacity-50 {look}">
    {@render children()}
  </button>
{/if}
