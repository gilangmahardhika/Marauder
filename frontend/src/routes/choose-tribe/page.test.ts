import { fireEvent, render, screen } from '@testing-library/svelte';
import { goto } from '$app/navigation';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { session } from '$lib/session';

const api = vi.hoisted(() => ({ getTribes: vi.fn(), getProjects: vi.fn(), getMe: vi.fn(), closeSession: vi.fn() }));

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  ...api
}));

import ChooseTribePage from './+page.svelte';

const projects = [
  { id: 'proj_a1', tribeId: 'tribe_a', name: 'Shop', repo: 'acme/shop', baseBranch: 'main' },
  { id: 'proj_a2', tribeId: 'tribe_a', name: 'Admin', repo: 'acme/admin', baseBranch: 'main' }
];

beforeEach(() => {
  api.getMe.mockResolvedValue({ admin: false });
  api.getProjects.mockResolvedValue({ projects });
  api.getTribes.mockResolvedValue({
    tribes: [
      { id: 'tribe_a', name: 'Alpha' },
      { id: 'tribe_b', name: 'Beta' }
    ]
  });
});

afterEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
});

test('lists tribes with project counts, and picking one stores it with its first project', async () => {
  session.setTribe('tribe_b');
  render(ChooseTribePage);
  const alpha = await screen.findByRole('button', { name: /Alpha/ });
  expect(alpha.textContent).toContain('2 projects');
  const beta = screen.getByRole('button', { name: /Beta/ });
  expect(beta.textContent).toContain('0 projects');
  expect(beta.getAttribute('aria-current')).toBe('true');

  await fireEvent.click(alpha);
  expect(session.tribe()).toBe('tribe_a');
  expect(session.project()).toBe('proj_a1');
  expect(goto).toHaveBeenCalledWith('/');
});

test('a tribe with no project stores an empty project', async () => {
  session.setProject('proj_a1');
  render(ChooseTribePage);
  await fireEvent.click(await screen.findByRole('button', { name: /Beta/ }));
  expect(session.tribe()).toBe('tribe_b');
  expect(session.project()).toBe('');
});

test('with no tribe a member is told to ask an admin, and an admin can create one', async () => {
  api.getTribes.mockResolvedValue({ tribes: [] });
  const member = render(ChooseTribePage);
  expect(await screen.findByText('You are not in a tribe yet. Ask an admin to add you.')).toBeTruthy();
  expect(screen.queryByRole('link', { name: 'Create a tribe' })).toBeNull();
  member.unmount();

  api.getMe.mockResolvedValue({ admin: true });
  render(ChooseTribePage);
  expect((await screen.findByRole('link', { name: 'Create a tribe' })).getAttribute('href')).toBe('/tribes/new');
});
