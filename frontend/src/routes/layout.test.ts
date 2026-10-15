import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { readable } from 'svelte/store';
import { goto } from '$app/navigation';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { session } from '$lib/session';
import { shell } from '$lib/shell.svelte';
import { toast } from '$lib/toast.svelte';

const api = vi.hoisted(() => ({
  getMe: vi.fn(),
  getProjects: vi.fn(),
  getTribes: vi.fn(),
  closeSession: vi.fn(),
  getNotifications: vi.fn(),
  readNotifications: vi.fn()
}));
vi.mock('$lib/stream.svelte', () => ({
  live: { connected: false, start: () => () => {}, wake: () => {}, on: () => () => {} }
}));
const location = vi.hoisted(() => ({ path: '/' }));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: {}, url: new URL(`http://127.0.0.1:4173${location.path}`) }).subscribe(run)
  }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => api);

import Layout from './+layout.svelte';

const children = createRawSnippet(() => ({ render: () => '<p>page</p>' }));

function me(admin: boolean, tribeAdminOf: string[] = []) {
  return { id: 'usr_1', name: 'Ana', email: 'ana@example.com', role: 'developer', admin, tribeAdminOf };
}

beforeEach(() => {
  api.getTribes.mockResolvedValue({ tribes: [] });
  api.getNotifications.mockResolvedValue({ notifications: [], unread: 0 });
});

afterEach(() => {
  vi.clearAllMocks();
  shell.admin = false;
  shell.tribeAdminOf = [];
  location.path = '/';
  sessionStorage.clear();
});

test('Admin links are ordered by use and only admins see them', async () => {
  api.getProjects.mockResolvedValue({ projects: [] });
  api.getMe.mockResolvedValue(me(true));
  const admin = render(Layout, { children });
  await waitFor(() => expect(sections().querySelector('a[href="/admin/usage"]')).not.toBeNull());
  expect(headings(sections()).slice(-2)).toEqual(['Admin', 'Company setup']);
  const adminLinks = [...sections().querySelectorAll('a[href^="/admin"], a[href="/tribes"]')].map((link) =>
    link.getAttribute('href')
  );
  expect(adminLinks).toEqual([
    '/admin/users',
    '/tribes',
    '/admin/runners',
    '/admin/usage',
    '/admin/github',
    '/admin/llm',
    '/admin/router',
    '/admin/agent',
    '/admin/conventions',
    '/admin/skills'
  ]);
  expect(screen.getAllByRole('link', { name: 'Token usage' })[0].getAttribute('href')).toBe('/admin/usage');
  admin.unmount();

  api.getMe.mockResolvedValue(me(false));
  shell.admin = false;
  render(Layout, { children });
  await waitFor(() => expect(api.getMe).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(screen.getAllByRole('link', { name: 'Board' }).length).toBeGreaterThan(0));
  expect(screen.queryByRole('link', { name: 'Token usage' })).toBeNull();
  expect(document.querySelector('a[href="/admin/usage"]')).toBeNull();
  expect(headings(sections())).not.toContain('Company setup');
});

test('the user menu links to My token usage for any member', async () => {
  api.getProjects.mockResolvedValue({ projects: [] });
  api.getMe.mockResolvedValue(me(false));
  render(Layout, { children });
  await waitFor(() => expect(screen.getByRole('button', { name: /Ana/ })).toBeTruthy());
  await fireEvent.click(screen.getByRole('button', { name: /Ana/ }));
  expect(screen.getByRole('link', { name: 'My token usage' }).getAttribute('href')).toBe('/account/usage');
});

const tribes = [
  { id: 'tribe_a', name: 'Alpha' },
  { id: 'tribe_b', name: 'Beta' }
];
const projects = [
  { id: 'proj_a1', tribeId: 'tribe_a', name: 'Shop', repo: 'acme/shop', baseBranch: 'main' },
  { id: 'proj_a2', tribeId: 'tribe_a', name: 'Admin', repo: 'acme/admin', baseBranch: 'main' },
  { id: 'proj_b1', tribeId: 'tribe_b', name: 'Billing', repo: 'acme/billing', baseBranch: 'main' }
];

test('the switcher lists only the selected tribe, and a project from another tribe is replaced', async () => {
  session.setTribe('tribe_a');
  session.setProject('proj_b1');
  api.getMe.mockResolvedValue(me(false));
  api.getTribes.mockResolvedValue({ tribes });
  api.getProjects.mockResolvedValue({ projects });
  render(Layout, { children });
  await waitFor(() => expect(session.project()).toBe('proj_a1'));
  expect(shell.projectId).toBe('proj_a1');
  await waitFor(() => expect(screen.getAllByText('Alpha').length).toBeGreaterThan(0));
  expect(screen.getAllByRole('link', { name: 'Switch tribe' })[0].getAttribute('href')).toBe('/choose-tribe');

  await fireEvent.click(document.getElementById('project-switch') as HTMLElement);
  const options = screen.getAllByRole('option').map((option) => option.textContent?.trim() ?? '');
  expect(options.some((text) => text.startsWith('Shop'))).toBe(true);
  expect(options.some((text) => text.startsWith('Admin'))).toBe(true);
  expect(options.some((text) => text.startsWith('Billing'))).toBe(false);
});

test('a stored tribe the API no longer returns sends the browser to the tribe selector', async () => {
  session.setTribe('tribe_gone');
  session.setProject('proj_b1');
  api.getMe.mockResolvedValue(me(false));
  api.getTribes.mockResolvedValue({ tribes });
  api.getProjects.mockResolvedValue({ projects });
  render(Layout, { children });
  await waitFor(() => expect(goto).toHaveBeenCalledWith('/choose-tribe'));
  expect(session.tribe()).toBe('');
  expect(session.project()).toBe('');
});

function sections() {
  return screen.getAllByRole('navigation', { name: 'Sections' })[0];
}

function headings(nav: HTMLElement) {
  return [...nav.querySelectorAll('p')].map((item) => item.textContent?.trim() ?? '');
}

test('the sidebar names the tribe and project sections and shows tribe pages to a tribe manager only', async () => {
  session.setTribe('tribe_a');
  session.setProject('proj_a1');
  api.getMe.mockResolvedValue(me(false));
  api.getTribes.mockResolvedValue({ tribes });
  api.getProjects.mockResolvedValue({ projects });
  const member = render(Layout, { children });
  await waitFor(() => expect(headings(sections())).toContain('Tribe · Alpha'));
  expect(headings(sections())).toEqual(['Workspace', 'Project · Shop', 'Tribe · Alpha']);
  expect(sections().querySelector('a[href="/projects"]')).not.toBeNull();
  expect(sections().querySelector('a[href="/tribes/tribe_a/conventions"]')).not.toBeNull();
  expect(sections().querySelector('a[href="/settings/conventions"]')).not.toBeNull();
  const projectLinks = [...sections().querySelectorAll('a[href^="/settings/"]')].map((link) => link.textContent?.trim());
  expect(projectLinks).toEqual(['Deploy', 'Workflow', 'Repository', 'Conventions', 'Linked projects']);
  expect(sections().querySelector('a[href="/tribes/tribe_a/mcp"]')).toBeNull();
  expect(sections().querySelector('a[href="/tribes/tribe_a"]')).toBeNull();
  expect(sections().querySelector('a[href="/tribes"]')).toBeNull();
  member.unmount();

  api.getMe.mockResolvedValue(me(false, ['tribe_a']));
  render(Layout, { children });
  await waitFor(() => expect(sections().querySelector('a[href="/tribes/tribe_a/mcp"]')).not.toBeNull());
  const tribeLinks = [...sections().querySelectorAll('a')].map((link) => link.textContent?.trim());
  expect(tribeLinks).toEqual([
    'Board',
    'Intake',
    'Deploy',
    'Workflow',
    'Repository',
    'Conventions',
    'Linked projects',
    'Projects',
    'Members',
    'MCP servers',
    'Conventions'
  ]);
  expect(sections().querySelector('a[href="/tribes/tribe_a"]')?.textContent?.trim()).toBe('Members');
});

test('Members is current only on the tribe page itself', async () => {
  session.setTribe('tribe_a');
  session.setProject('proj_a1');
  location.path = '/tribes/tribe_a/mcp';
  api.getMe.mockResolvedValue(me(true));
  api.getTribes.mockResolvedValue({ tribes });
  api.getProjects.mockResolvedValue({ projects });
  render(Layout, { children });
  await waitFor(() => expect(sections().querySelector('a[href="/tribes/tribe_a/mcp"]')).not.toBeNull());
  const current = [...sections().querySelectorAll('a[aria-current="page"]')].map((link) => link.getAttribute('href'));
  expect(current).toEqual(['/tribes/tribe_a/mcp']);
});

test('the Members link is current on the tribe page and the admin Tribes link is not', async () => {
  session.setTribe('tribe_a');
  session.setProject('proj_a1');
  location.path = '/tribes/tribe_a';
  api.getMe.mockResolvedValue(me(true));
  api.getTribes.mockResolvedValue({ tribes });
  api.getProjects.mockResolvedValue({ projects });
  render(Layout, { children });
  await waitFor(() => expect(sections().querySelector('a[href="/tribes/tribe_a"]')).not.toBeNull());
  const current = [...sections().querySelectorAll('a[aria-current="page"]')].map((link) => link.getAttribute('href'));
  expect(current).toEqual(['/tribes/tribe_a']);
});

test('with no project the Project section is gone', async () => {
  session.setTribe('tribe_c');
  session.setProject('');
  api.getMe.mockResolvedValue(me(false));
  api.getTribes.mockResolvedValue({ tribes: [...tribes, { id: 'tribe_c', name: 'Gamma' }] });
  api.getProjects.mockResolvedValue({ projects });
  render(Layout, { children });
  await waitFor(() => expect(headings(sections())).toContain('Tribe · Gamma'));
  expect(headings(sections())).toEqual(['Workspace', 'Tribe · Gamma']);
  expect(sections().querySelector('a[href="/settings/repository"]')).toBeNull();
});

test('the mobile menu shows the section headings', async () => {
  session.setTribe('tribe_a');
  session.setProject('proj_a1');
  api.getMe.mockResolvedValue(me(false));
  api.getTribes.mockResolvedValue({ tribes });
  api.getProjects.mockResolvedValue({ projects });
  render(Layout, { children });
  await waitFor(() => expect(headings(sections())).toContain('Project · Shop'));
  await fireEvent.click(screen.getByRole('button', { name: 'Open menu' }));
  const navs = screen.getAllByRole('navigation', { name: 'Sections' });
  expect(navs).toHaveLength(2);
  expect(headings(navs[1])).toEqual(['Workspace', 'Project · Shop', 'Tribe · Alpha', 'Account']);
});

test('toasts show on bare pages and the top bar has no status line', async () => {
  location.path = '/login';
  render(Layout, { children });
  toast.success('Signed out');
  expect(await screen.findByText('Signed out')).toBeTruthy();
  expect(document.querySelector('header')).toBeNull();
  toast.clear();
});

test('the top bar holds the bell and no status text', async () => {
  api.getProjects.mockResolvedValue({ projects: [] });
  api.getMe.mockResolvedValue(me(false));
  render(Layout, { children });
  const header = document.querySelector('header');
  expect(header?.querySelector('[role="status"]')).toBeNull();
  expect(await screen.findByRole('button', { name: /Notifications/ })).toBeTruthy();
});
