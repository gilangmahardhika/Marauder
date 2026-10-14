import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';
import { toast } from '$lib/toast.svelte';

const api = vi.hoisted(() => ({
  addMember: vi.fn(),
  getMembers: vi.fn(),
  getTribeCandidates: vi.fn(),
  getTribes: vi.fn(),
  removeMember: vi.fn(),
  setTribeAdmin: vi.fn()
}));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: { id: 'tribe_a' }, url: new URL('http://127.0.0.1:4173/tribes/tribe_a') }).subscribe(run)
  }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  ...api
}));

import TribePage from './+page.svelte';

const ana = { id: 'usr_ana', name: 'Ana', email: 'ana@example.com', role: 'developer', admin: false, tribeAdmin: true };

beforeEach(() => {
  shell.admin = true;
  api.getMembers.mockResolvedValue({ members: [ana] });
  api.getTribeCandidates.mockResolvedValue({ users: [] });
  api.getTribes.mockResolvedValue({ tribes: [{ id: 'tribe_a', name: 'Alpha' }] });
});

afterEach(() => {
  vi.clearAllMocks();
  toast.clear();
  shell.admin = false;
});

test('a refused tribe admin change shows its field error in a toast', async () => {
  api.setTribeAdmin.mockRejectedValue(
    Object.assign(new Error('validation failed'), { fields: { tribeAdmin: 'a tribe admin cannot clear their own flag' } })
  );
  render(TribePage);
  await fireEvent.click(await screen.findByRole('button', { name: 'Remove tribe admin' }));
  await waitFor(() =>
    expect(toast.items.map((item) => [item.tone, item.text])).toEqual([['error', 'a tribe admin cannot clear their own flag']])
  );
});

test('removing a member asks on the row, then confirms with a toast', async () => {
  api.removeMember.mockResolvedValue(undefined);
  render(TribePage);
  await fireEvent.click(await screen.findByRole('button', { name: 'Remove' }));
  expect(screen.getByText('Remove from Alpha?')).toBeTruthy();
  api.getMembers.mockResolvedValue({ members: [] });
  await fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
  await waitFor(() => expect(api.removeMember).toHaveBeenCalledWith('tribe_a', 'usr_ana'));
  await waitFor(() => expect(toast.items.map((item) => item.text)).toEqual(['Member removed']));
  expect(await screen.findByText('No members yet')).toBeTruthy();
});

test('a failed first load shows the error with Try again', async () => {
  api.getMembers.mockRejectedValueOnce(new Error('tribe not found'));
  render(TribePage);
  expect(await screen.findByText('tribe not found')).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
  expect(await screen.findByText('Ana')).toBeTruthy();
});
