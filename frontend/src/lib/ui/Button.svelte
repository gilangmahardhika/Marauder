<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import Spinner from './Spinner.svelte';
  import type { IconName } from './icons';

  let {
    variant = 'secondary',
    type = 'button',
    disabled = false,
    loading = false,
    full = false,
    href,
    form,
    icon,
    trailing,
    title,
    'aria-label': ariaLabel,
    onclick,
    children
  }: {
    variant?: 'primary' | 'secondary' | 'ghost' | 'danger';
    type?: 'button' | 'submit';
    disabled?: boolean;
    /** Shows a spinner in place of the icon and disables the button; the label stays. */
    loading?: boolean;
    full?: boolean;
    href?: string;
    form?: string;
    icon?: IconName;
    trailing?: IconName;
    title?: string;
    'aria-label'?: string;
    onclick?: (event: MouseEvent) => void;
    children: Snippet;
  } = $props();

  const look = $derived(
    variant === 'primary'
      ? 'border border-accent bg-accent text-surface shadow-xs hover:border-accent-strong hover:bg-accent-strong'
      : variant === 'danger'
        ? 'border border-red-200 bg-surface text-red-800 shadow-xs hover:border-red-300 hover:bg-red-50'
        : variant === 'ghost'
          ? 'border border-transparent bg-transparent text-muted hover:bg-ground hover:text-ink'
          : 'border border-line bg-surface text-ink shadow-xs hover:border-line-strong hover:bg-ground/60'
  );

  const base = $derived(
    `inline-flex min-h-11 items-center justify-center gap-2 whitespace-nowrap rounded-control px-4 text-sm font-medium transition-colors duration-150 active:translate-y-px ${full ? 'w-full' : 'w-fit shrink-0'}`
  );
</script>

{#snippet body()}
  {#if loading}
    <Spinner />
  {:else if icon}
    <Icon name={icon} />
  {/if}
  {@render children()}
  {#if trailing}<Icon name={trailing} />{/if}
{/snippet}

{#if href && !disabled && !loading}
  <a {href} {title} aria-label={ariaLabel} class="{base} {look}">
    {@render body()}
  </a>
{:else}
  <button
    {type}
    {form}
    {onclick}
    {title}
    aria-label={ariaLabel}
    aria-busy={loading || undefined}
    disabled={disabled || loading}
    class="{base} disabled:cursor-not-allowed disabled:opacity-50 disabled:active:translate-y-0 {look}"
  >
    {@render body()}
  </button>
{/if}
