import { render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { session } from '$lib/session';
import { shell } from '$lib/shell.svelte';

const api = vi.hoisted(() => ({
  createProject: vi.fn(),
  getCompanyRepos: vi.fn(),
  getProjects: vi.fn(),
  getTribes: vi.fn()
}));

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  ...api
}));

import NewProjectPage from './+page.svelte';

beforeEach(() => {
  shell.tribeAdminOf = ['tribe_b', 'tribe_c'];
  api.getCompanyRepos.mockResolvedValue({ repos: [] });
  api.getProjects.mockResolvedValue({ projects: [] });
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
  shell.tribeAdminOf = [];
});

test('the tribe select starts on the active tribe', async () => {
  session.setTribe('tribe_c');
  render(NewProjectPage);
  const select = screen.getByLabelText<HTMLSelectElement>('Tribe');
  await waitFor(() => expect(select.options.length).toBe(2));
  expect(select.value).toBe('tribe_c');
});

test('an active tribe the user cannot manage starts on the first manageable tribe', async () => {
  session.setTribe('tribe_a');
  render(NewProjectPage);
  const select = screen.getByLabelText<HTMLSelectElement>('Tribe');
  await waitFor(() => expect(select.options.length).toBe(2));
  await waitFor(() => expect(select.value).toBe('tribe_b'));
});
