<script lang="ts">
  import { page } from '$app/stores';
  import type { Snippet } from 'svelte';
  import { closeSession, getMe, getProjects } from '$lib/api';
  import Live from '$lib/components/Live.svelte';
  import { initials, roleLabel } from '$lib/roles';
  import { session } from '$lib/session';
  import { shell } from '$lib/shell.svelte';
  import type { ProjectRecord } from '$lib/types';
  import Icon from '$lib/ui/Icon.svelte';
  import Menu from '$lib/ui/Menu.svelte';
  import SearchSelect from '$lib/ui/SearchSelect.svelte';
  import type { IconName } from '$lib/ui/icons';
  import '../app.css';

  let { children }: { children: Snippet } = $props();
  let projects = $state<ProjectRecord[]>([]);
  let selected = $state('');
  let mobile = $state(false);

  const groups: { label: string; admin?: boolean; items: { href: string; label: string; icon: IconName }[] }[] = [
    {
      label: 'Workspace',
      items: [
        { href: '/', label: 'Board', icon: 'board' },
        { href: '/intake', label: 'Intake', icon: 'intake' }
      ]
    },
    {
      label: 'Project',
      items: [
        { href: '/projects', label: 'Projects', icon: 'projects' },
        { href: '/settings/repository', label: 'Repository', icon: 'git' },
        { href: '/settings/links', label: 'Linked projects', icon: 'link' },
        { href: '/settings/deploy', label: 'Deploy', icon: 'terminal' },
        { href: '/settings/conventions', label: 'Conventions', icon: 'rules' }
      ]
    },
    {
      label: 'Admin',
      admin: true,
      items: [
        { href: '/admin/users', label: 'Users', icon: 'users' },
        { href: '/tribes', label: 'Tribes', icon: 'shield' },
        { href: '/admin/github', label: 'Company GitHub', icon: 'building' },
        { href: '/admin/llm', label: 'LLM APIs', icon: 'spark' },
        { href: '/admin/router', label: 'Router', icon: 'router' },
        { href: '/admin/conventions', label: 'Conventions', icon: 'rules' },
        { href: '/admin/skills', label: 'Skills', icon: 'skill' }
      ]
    }
  ];

  const login = $derived($page.url.pathname === '/login');
  const projectName = $derived(projects.find((project) => project.id === selected)?.name ?? 'No project');
  const projectOptions = $derived(
    projects.map((project) => ({ value: project.id, label: project.name, detail: project.repo }))
  );

  function active(href: string) {
    const path = $page.url.pathname;
    if (href === '/') return path === '/';
    return path === href || path.startsWith(`${href}/`);
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
    if ($page.url.pathname === '/login') return;
    selected = session.project();
    void getMe()
      .then((me) => {
        shell.admin = me.admin;
        shell.email = me.email;
        shell.role = me.role;
      })
      .catch(() => {
        shell.admin = false;
      });
    void getProjects()
      .then((body) => {
        projects = body.projects;
        if (session.project() === '' && body.projects[0]) {
          session.setProject(body.projects[0].id);
          selected = body.projects[0].id;
        }
        shell.projectId = session.project();
      })
      .catch(() => {
        projects = [];
      });
  });
</script>

<svelte:head>
  <title>{shell.title === 'Marauder' ? 'Marauder' : `${shell.title} · Marauder`}</title>
</svelte:head>

<a class="skip" href="#main">Skip to content</a>
{#if login}
  <main id="main">{@render children()}</main>
{:else}
  <div class="min-h-screen bg-ground text-ink lg:pl-60">
    <aside class="fixed inset-y-0 left-0 z-20 hidden w-60 flex-col border-r border-line bg-sidebar lg:flex">
      <div class="flex h-16 items-center gap-2 px-4">
        <span class="flex size-7 items-center justify-center rounded-control bg-accent text-xs font-semibold text-surface">M</span>
        <a href="/" class="font-semibold">Marauder</a>
      </div>
      <div class="px-3">
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
        {#each groups as group (group.label)}
          {#if !group.admin || shell.admin}
            <p class="px-2.5 pb-2 pt-4 text-[10px] font-semibold uppercase tracking-widest text-muted">{group.label}</p>
            {#each group.items as item (item.href)}
              <a
                href={item.href}
                class="mb-1 flex h-10 items-center gap-3 border-l-2 px-2.5 text-sm {active(item.href)
                  ? 'border-accent bg-surface font-medium text-ink'
                  : 'border-transparent text-muted hover:bg-surface hover:text-ink'}"
              >
                <Icon name={item.icon} />
                {item.label}
              </a>
            {/each}
          {/if}
        {/each}
      </nav>
      <div class="border-t border-line p-3">
        <Menu
          label={shell.email || 'Account'}
          detail={roleLabel(shell.role)}
          initials={initials(shell.email || 'M')}
          placement="up"
          items={[
            { label: 'Account', href: '/account' },
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
            {#each groups as group (group.label)}
              {#if !group.admin || shell.admin}
                {#each group.items as item (item.href)}
                  <a
                    href={item.href}
                    class="flex h-11 items-center gap-3 rounded-control px-3 text-sm hover:bg-surface"
                    onclick={() => (mobile = false)}
                  >
                    <Icon name={item.icon} />
                    {item.label}
                  </a>
                {/each}
              {/if}
            {/each}
            <a href="/account" class="flex h-11 items-center rounded-control px-3 text-sm hover:bg-surface" onclick={() => (mobile = false)}>Account</a>
            <button type="button" class="flex h-11 items-center rounded-control px-3 text-left text-sm hover:bg-surface" onclick={() => void signOut()}>Sign out</button>
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
          <span class="hidden text-muted lg:inline">Project</span>
          <span class="hidden text-line-strong lg:inline" aria-hidden="true">/</span>
          <span class="hidden truncate font-medium text-ink lg:inline">{projectName}</span>
        </p>
        <div class="ml-auto min-w-0 pr-2 text-right lg:pr-0">
          <Live class="truncate text-[13px]" message={shell.message === shell.title ? '' : shell.message} />
        </div>
      </header>
      <main id="main" class="flex-1 px-6 py-6 lg:px-8 lg:py-8">{@render children()}</main>
    </div>
  </div>
{/if}
