<script lang="ts">
  import { page } from '$app/stores';
  import type { Snippet } from 'svelte';

  let { children }: { children: Snippet } = $props();

  const links = [
    { href: '/settings/repository', label: 'Repository' },
    { href: '/settings/links', label: 'Linked projects' },
    { href: '/settings/deploy', label: 'Deploy' },
    { href: '/settings/conventions', label: 'Conventions' }
  ];

  function active(href: string) {
    return $page.url.pathname === href || $page.url.pathname.startsWith(`${href}/`);
  }
</script>

<div class="grid gap-6 lg:grid-cols-[11rem_minmax(0,1fr)] lg:gap-10">
  <nav class="self-start lg:sticky lg:top-20" aria-label="Project settings">
    <p class="hidden px-3 pb-2 text-[11px] font-semibold uppercase tracking-wider text-muted lg:block">Project settings</p>
    <div class="flex gap-1 overflow-x-auto border-b border-line lg:flex-col lg:border-b-0">
      {#each links as link (link.href)}
        <a
          href={link.href}
          aria-current={active(link.href) ? 'page' : undefined}
          class="flex min-h-11 shrink-0 items-center border-b-2 px-3 text-sm lg:border-b-0 lg:border-l-2 {active(link.href)
            ? 'border-accent font-medium text-ink lg:bg-surface'
            : 'border-transparent text-muted hover:text-ink lg:hover:bg-surface'}"
        >
          {link.label}
        </a>
      {/each}
    </div>
  </nav>
  <div class="min-w-0">{@render children()}</div>
</div>
