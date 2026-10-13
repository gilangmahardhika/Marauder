import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { readable } from 'svelte/store';
import { goto } from '$app/navigation';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { session } from '$lib/session';
import { shell } from '$lib/shell.svelte';

const api = vi.hoisted(() => ({ getMe: vi.fn(), getProjects: vi.fn(), getTribes: vi.fn(), closeSession: vi.fn() }));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) => readable({ params: {}, url: new URL('http://127.0.0.1:4173/') }).subscribe(run)
  }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => api);

import Layout from './+layout.svelte';

const children = createRawSnippet(() => ({ render: () => '<p>page</p>' }));

function me(admin: boolean) {
  return { id: 'usr_1', name: 'Ana', email: 'ana@example.com', role: 'developer', admin, tribeAdminOf: [] };
}

beforeEach(() => {
  api.getTribes.mockResolvedValue({ tribes: [] });
});

afterEach(() => {
  vi.clearAllMocks();
  shell.admin = false;
  sessionStorage.clear();
});

test('Token usage is the first admin item and only admins see it', async () => {
  api.getProjects.mockResolvedValue({ projects: [] });
  api.getMe.mockResolvedValue(me(true));
  const admin = render(Layout, { children });
  const sections = () => screen.getAllByRole('navigation', { name: 'Sections' })[0];
  await waitFor(() => expect(sections().querySelector('a[href="/admin/usage"]')).not.toBeNull());
  const adminLinks = [...sections().querySelectorAll('a[href^="/admin"]')].map((link) => link.getAttribute('href'));
  expect(adminLinks[0]).toBe('/admin/usage');
  expect(screen.getAllByRole('link', { name: 'Token usage' })[0].getAttribute('href')).toBe('/admin/usage');
  admin.unmount();

  api.getMe.mockResolvedValue(me(false));
  shell.admin = false;
  render(Layout, { children });
  await waitFor(() => expect(api.getMe).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(screen.getAllByRole('link', { name: 'Board' }).length).toBeGreaterThan(0));
  expect(screen.queryByRole('link', { name: 'Token usage' })).toBeNull();
  expect(document.querySelector('a[href="/admin/usage"]')).toBeNull();
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
