import { render, screen, waitFor } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { readable } from 'svelte/store';
import { afterEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';

const api = vi.hoisted(() => ({ getMe: vi.fn(), getProjects: vi.fn(), closeSession: vi.fn() }));

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

afterEach(() => {
  vi.clearAllMocks();
  shell.admin = false;
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
