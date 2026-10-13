import { render, screen } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { session } from '$lib/session';
import { shell } from '$lib/shell.svelte';

const api = vi.hoisted(() => ({ getProjects: vi.fn(), getTribes: vi.fn() }));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: {}, url: new URL('http://127.0.0.1:4173/projects') }).subscribe(run)
  }
}));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  ...api
}));

import ProjectsPage from './+page.svelte';

beforeEach(() => {
  shell.admin = true;
  api.getProjects.mockResolvedValue({
    projects: [
      { id: 'proj_a1', tribeId: 'tribe_a', name: 'Shop', repo: 'acme/shop', baseBranch: 'main' },
      { id: 'proj_b1', tribeId: 'tribe_b', name: 'Billing', repo: 'acme/billing', baseBranch: 'main' }
    ]
  });
  api.getTribes.mockResolvedValue({
    tribes: [
      { id: 'tribe_a', name: 'Alpha' },
      { id: 'tribe_b', name: 'Beta' },
      { id: 'tribe_c', name: 'Gamma' }
    ]
  });
});

afterEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  shell.admin = false;
});

test('an admin sees only the active tribe projects and no tribe filter', async () => {
  session.setTribe('tribe_a');
  render(ProjectsPage);
  expect(await screen.findByText('Shop')).toBeTruthy();
  expect(screen.queryByText('Billing')).toBeNull();
  expect(screen.getByText("Projects in Alpha. Switch tribe in the sidebar to see another tribe's projects.")).toBeTruthy();
  expect(screen.queryByLabelText('Tribe')).toBeNull();
  expect(screen.queryByRole('columnheader', { name: 'Tribe' })).toBeNull();
  expect(screen.getByRole('link', { name: 'New project' })).toBeTruthy();
});

test('with no active tribe the page links to the tribe selector', async () => {
  render(ProjectsPage);
  const link = await screen.findByRole('link', { name: 'Choose a tribe' });
  expect(link.getAttribute('href')).toBe('/choose-tribe');
  expect(screen.getByText('Choose a tribe to see its projects.')).toBeTruthy();
  expect(screen.queryByText('Shop')).toBeNull();
});

test('a tribe with no projects says so', async () => {
  session.setTribe('tribe_c');
  render(ProjectsPage);
  expect(await screen.findByText('No projects in this tribe yet.')).toBeTruthy();
  expect(screen.queryByText('Shop')).toBeNull();
  expect(screen.queryByText('Billing')).toBeNull();
});
