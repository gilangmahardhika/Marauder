<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import type { Snippet } from 'svelte';
  import { closeSession, getMe, getProjects, getTribes } from '$lib/api';
  import NotificationBell from '$lib/components/NotificationBell.svelte';
  import { displayName } from '$lib/people';
  import { initials, roleLabel } from '$lib/roles';
  import { session } from '$lib/session';
  import { canManageTribe, shell } from '$lib/shell.svelte';
  import { live } from '$lib/stream.svelte';
  import type { ProjectRecord, TribeRecord } from '$lib/types';
  import Icon from '$lib/ui/Icon.svelte';
  import Menu from '$lib/ui/Menu.svelte';
  import SearchSelect from '$lib/ui/SearchSelect.svelte';
  import Toaster from '$lib/ui/Toaster.svelte';
  import type { IconName } from '$lib/ui/icons';
  import '../app.css';

  let { children }: { children: Snippet } = $props();
  let projects = $state<ProjectRecord[]>([]);
  let tribes = $state<TribeRecord[]>([]);
  let tribeId = $state('');
  let selected = $state('');
  let mobile = $state(false);

  type NavItem = { href: string; label: string; icon: IconName; exact?: boolean };
  type NavGroup = { label: string; items: NavItem[] };

  const workspace: NavGroup = {
    label: 'Workspace',
    items: [
      { href: '/', label: 'Board', icon: 'board' },
      { href: '/intake', label: 'Intake', icon: 'intake' }
    ]
  };

  const admin: NavGroup = {
    label: 'Admin',
    items: [
      { href: '/admin/users', label: 'Users', icon: 'users' },
      { href: '/tribes', label: 'Tribes', icon: 'shield' },
      { href: '/admin/runners', label: 'Runners', icon: 'server' },
      { href: '/admin/usage', label: 'Token usage', icon: 'chart' }
    ]
  };

  const company: NavGroup = {
    label: 'Company setup',
    items: [
      { href: '/admin/github', label: 'Company GitHub', icon: 'building' },
      { href: '/admin/llm', label: 'LLM APIs', icon: 'spark' },
      { href: '/admin/router', label: 'Router', icon: 'router' },
      { href: '/admin/conventions', label: 'Conventions', icon: 'rules' },
      { href: '/admin/skills', label: 'Skills', icon: 'skill' }
    ]
  };

  const tribeGroup = $derived.by((): NavGroup => {
    const tribe = tribes.find((item) => item.id === tribeId);
    if (!tribe) return { label: 'Tribe', items: [{ href: '/projects', label: 'Projects', icon: 'projects' }] };
    const manages = canManageTribe(tribe.id);
    const items: NavItem[] = [{ href: '/projects', label: 'Projects', icon: 'projects' }];
    if (manages) {
      items.push({ href: `/tribes/${tribe.id}`, label: 'Members', icon: 'users', exact: true });
      items.push({ href: `/tribes/${tribe.id}/mcp`, label: 'MCP servers', icon: 'mcp' });
    }
    items.push({ href: `/tribes/${tribe.id}/conventions`, label: 'Conventions', icon: 'rules' });
    return { label: `Tribe · ${tribe.name}`, items };
  });

  const projectGroup = $derived.by((): NavGroup | null => {
    const project = tribeProjects.find((item) => item.id === selected);
    if (!project) return null;
    return {
      label: `Project · ${project.name}`,
      items: [
        { href: '/settings/deploy', label: 'Deploy', icon: 'terminal' },
        { href: '/settings/repository', label: 'Repository', icon: 'git' },
        { href: '/settings/conventions', label: 'Conventions', icon: 'rules' },
        { href: '/settings/links', label: 'Linked projects', icon: 'link' }
      ]
    };
  });

  const navGroups = $derived(
    [workspace, projectGroup, tribeGroup, shell.admin ? admin : null, shell.admin ? company : null].filter(
      (group): group is NavGroup => group !== null
    )
  );
  const navItems = $derived(navGroups.flatMap((group) => group.items));

  const bare = $derived($page.url.pathname === '/login' || $page.url.pathname === '/choose-tribe');
  const tribeProjects = $derived(projects.filter((project) => project.tribeId === tribeId));
  const tribeName = $derived(tribes.find((tribe) => tribe.id === tribeId)?.name ?? 'No tribe');
  const projectName = $derived(tribeProjects.find((project) => project.id === selected)?.name ?? 'No project');
  const projectOptions = $derived(
    tribeProjects.map((project) => ({ value: project.id, label: project.name, detail: project.repo }))
  );

  function matches(item: NavItem, path: string) {
    if (item.href === '/' || item.exact) return path === item.href;
    return path === item.href || path.startsWith(`${item.href}/`);
  }

  function active(item: NavItem) {
    const path = $page.url.pathname;
    if (!matches(item, path)) return false;
    return !navItems.some((other) => other.href.length > item.href.length && matches(other, path));
  }

  function choose(id: string) {
    session.setProject(id);
    window.location.assign('/');
  }

  async function signOut() {
    try {
      await closeSession();
    } catch {
      /* The local session still ends. */
    }
    session.clear();
    window.location.assign('/login');
  }

  $effect(() => {
    shell.projectId = session.project();
    if (bare) return;
    selected = session.project();
    tribeId = session.tribe();
    void getMe()
      .then((me) => {
        shell.admin = me.admin;
        shell.name = me.name;
        shell.email = me.email;
        shell.role = me.role;
        shell.tribeAdminOf = me.tribeAdminOf ?? [];
      })
      .catch(() => {
        shell.admin = false;
        shell.tribeAdminOf = [];
      });
    void Promise.all([getTribes(), getProjects()])
      .then(([tribeBody, projectBody]) => {
        tribes = tribeBody.tribes;
        projects = projectBody.projects;
        if (tribeId !== '' && !tribes.some((tribe) => tribe.id === tribeId)) {
          session.setTribe('');
          session.setProject('');
          shell.projectId = '';
          void goto('/choose-tribe');
          return;
        }
        const own = projects.filter((project) => project.tribeId === tribeId);
        if (!own.some((project) => project.id === session.project())) {
          session.setProject(own[0]?.id ?? '');
          selected = session.project();
        }
        shell.projectId = session.project();
      })
      .catch(() => {
        projects = [];
      });
  });

  $effect(() => {
    if (bare || session.bearer() === '') return;
    return live.start(shell.projectId);
  });
</script>

<svelte:document
  onvisibilitychange={() => {
    if (document.visibilityState === 'visible') live.wake();
  }}
/>

<svelte:head>
  <title>{shell.title === 'Marauder' ? 'Marauder' : `${shell.title} · Marauder`}</title>
</svelte:head>

<a class="skip" href="#main">Skip to content</a>
{#if bare}
  <main id="main">{@render children()}</main>
{:else}
  <div class="min-h-screen bg-ground text-ink lg:pl-60">
    <aside class="fixed inset-y-0 left-0 z-20 hidden w-60 flex-col border-r border-line bg-sidebar lg:flex">
      <div class="flex h-16 items-center gap-2 px-4">
        <span class="flex size-7 items-center justify-center rounded-control bg-accent text-xs font-semibold text-surface">M</span>
        <a href="/" class="font-semibold">Marauder</a>
      </div>
      <div class="px-3">
        {@render tribeRow()}
        <SearchSelect
          label="Project"
          id="project-switch"
          width="full"
          hideLabel
          placeholder={projects.length === 0 ? 'No project' : 'Search projects'}
          options={projectOptions}
          value={selected}
          onchange={choose}
        />
      </div>
      <nav class="mt-4 flex-1 overflow-auto px-3" aria-label="Sections">
        {#each navGroups as group (group.label)}
          <p class="truncate px-2.5 pb-2 pt-4 text-[10px] font-semibold uppercase tracking-widest text-muted" title={group.label}>{group.label}</p>
          {#each group.items as item (item.href)}
            <a
              href={item.href}
              aria-current={active(item) ? 'page' : undefined}
              class="mb-1 flex h-10 items-center gap-3 border-l-2 px-2.5 text-sm {active(item)
                ? 'border-accent bg-surface font-medium text-ink'
                : 'border-transparent text-muted hover:bg-surface hover:text-ink'}"
            >
              <Icon name={item.icon} />
              {item.label}
            </a>
          {/each}
        {/each}
      </nav>
      <div class="border-t border-line p-3">
        <Menu
          label={shell.email ? displayName(shell) : 'Account'}
          detail={roleLabel(shell.role)}
          initials={initials(shell.email ? displayName(shell) : 'M')}
          placement="up"
          items={[
            { label: 'Account', href: '/account' },
            { label: 'My token usage', href: '/account/usage' },
            { label: 'Sign out', onclick: () => void signOut() }
          ]}
        />
      </div>
    </aside>

    {#if mobile}
      <div class="fixed inset-0 z-30 bg-ink/20 lg:hidden">
        <button type="button" class="absolute inset-0 h-full w-full cursor-default border-0 bg-transparent" aria-label="Close menu" onclick={() => (mobile = false)}></button>
        <div class="relative flex h-full w-72 flex-col border-r border-line bg-sidebar p-4">
          <div class="mb-4 flex items-center justify-between">
            <span class="font-semibold">Marauder</span>
            <button type="button" class="inline-flex size-11 items-center justify-center" aria-label="Close" onclick={() => (mobile = false)}>
              <Icon name="close" class="size-5" />
            </button>
          </div>
          {@render tribeRow()}
          <SearchSelect
            label="Project"
            id="project-switch-mobile"
            width="full"
            hideLabel
            placeholder={projects.length === 0 ? 'No project' : 'Search projects'}
            options={projectOptions}
            value={selected}
            onchange={choose}
          />
          <nav class="mt-4 flex flex-1 flex-col gap-1" aria-label="Sections">
            {#each navGroups as group (group.label)}
              <p class="truncate px-3 pb-1 pt-3 text-[10px] font-semibold uppercase tracking-widest text-muted" title={group.label}>{group.label}</p>
              {#each group.items as item (item.href)}
                <a
                  href={item.href}
                  aria-current={active(item) ? 'page' : undefined}
                  class="flex h-11 items-center gap-3 rounded-control px-3 text-sm hover:bg-surface {active(item) ? 'bg-surface font-medium' : ''}"
                  onclick={() => (mobile = false)}
                >
                  <Icon name={item.icon} />
                  {item.label}
                </a>
              {/each}
            {/each}
            <p class="px-3 pb-1 pt-3 text-[10px] font-semibold uppercase tracking-widest text-muted">Account</p>
            <a href="/account" class="flex h-11 items-center rounded-control px-3 text-sm hover:bg-surface" onclick={() => (mobile = false)}>Account</a>
            <a href="/account/usage" class="flex h-11 items-center rounded-control px-3 text-sm hover:bg-surface" onclick={() => (mobile = false)}>My token usage</a>
            <button type="button" class="flex h-11 items-center gap-3 rounded-control px-3 text-left text-sm hover:bg-surface" onclick={() => void signOut()}>
              <Icon name="log-out" />
              Sign out
            </button>
          </nav>
        </div>
      </div>
    {/if}

    <div class="flex min-h-screen min-w-0 flex-col">
      <header class="sticky top-0 z-10 flex h-12 items-center gap-3 border-b border-line bg-ground px-2 lg:px-8">
        <button
          type="button"
          class="inline-flex size-11 items-center justify-center lg:hidden"
          aria-label="Open menu"
          onclick={() => (mobile = true)}
        >
          <Icon name="menu" class="size-5" />
        </button>
        <p class="flex min-w-0 items-center gap-2 text-[13px]">
          <span class="font-semibold text-ink lg:hidden">Marauder</span>
          <span class="hidden truncate text-muted lg:inline">{tribeName}</span>
          <span class="hidden text-line-strong lg:inline" aria-hidden="true">/</span>
          <span class="hidden truncate font-medium text-ink lg:inline">{projectName}</span>
        </p>
        <div class="ml-auto">
          <NotificationBell />
        </div>
      </header>
      <main id="main" class="flex-1 px-6 py-6 lg:px-8 lg:py-8">{@render children()}</main>
    </div>
  </div>
{/if}
<Toaster />

{#snippet tribeRow()}
  <div class="mb-1 flex items-center justify-between gap-2">
    <span class="min-w-0 truncate pl-1 text-xs font-medium text-muted" title={tribeName}>{tribeName}</span>
    <a href="/choose-tribe" class="inline-flex min-h-11 shrink-0 items-center px-1 text-xs font-medium text-accent hover:underline">Switch tribe</a>
  </div>
{/snippet}
