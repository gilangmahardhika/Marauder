import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import type { Runner } from '$lib/types';

const api = vi.hoisted(() => ({
  ApiError: class ApiError extends Error {},
  getRunners: vi.fn(),
  putJobs: vi.fn(),
  runnerAction: vi.fn()
}));
const route = vi.hoisted(() => ({ search: '' }));
const nav = vi.hoisted(() => ({ goto: vi.fn() }));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: {}, url: new URL(`http://127.0.0.1:4173/admin/runners${route.search}`) }).subscribe(run)
  }
}));
vi.mock('$app/navigation', () => ({ goto: nav.goto }));
vi.mock('$lib/api', () => ({
  ApiError: api.ApiError,
  unreachable: () => 'API unreachable',
  getRunners: api.getRunners,
  putJobs: api.putJobs,
  runnerAction: api.runnerAction
}));

import RunnersPage from './+page.svelte';

function runner(over: Partial<Runner>): Runner {
  return {
    id: 'rnr_a',
    name: 'agent-1',
    labels: ['podman'],
    slots: 2,
    busy: 1,
    status: 'busy',
    version: '0.1.0',
    protocol: 1,
    updateRequired: false,
    heartbeatAt: new Date().toISOString(),
    leases: [{ id: 'lse_1', cardId: 'card_one', projectId: 'proj_default', since: '2026-10-14T05:00:00Z' }],
    actions: ['drain', 'remove'],
    ...over
  };
}

const runners = [
  runner({}),
  runner({ id: 'rnr_b', name: 'old-box', busy: 0, status: 'offline', updateRequired: true, leases: [], actions: ['resume', 'remove'] })
];

beforeEach(() => {
  route.search = '';
  api.getRunners.mockResolvedValue({ executor: 'runners', protocol: 1, runners });
  api.putJobs.mockResolvedValue({ executor: 'local' });
  api.runnerAction.mockResolvedValue({});
});

afterEach(() => {
  vi.clearAllMocks();
});

test('lists runners with status, slots, cards, and an update badge', async () => {
  render(RunnersPage);
  const table = await screen.findByRole('table', { name: 'Runners' });
  const rows = within(table).getAllByRole('row').slice(1);
  expect(rows).toHaveLength(2);
  expect(within(rows[0]).getByText('Busy')).toBeTruthy();
  expect(within(rows[0]).getByText('1/2')).toBeTruthy();
  expect(within(rows[0]).getByRole('link', { name: 'card_one' }).getAttribute('href')).toBe('/cards/card_one');
  expect(within(rows[1]).getByText('Offline')).toBeTruthy();
  expect(within(rows[1]).getByText('Update required')).toBeTruthy();
  expect(screen.queryByText(/No runner is online/)).toBeNull();
});

test('each row shows only the actions from the API', async () => {
  render(RunnersPage);
  const table = await screen.findByRole('table', { name: 'Runners' });
  const rows = within(table).getAllByRole('row').slice(1);
  expect(within(rows[1]).getAllByRole('button').map((b) => b.textContent)).toEqual(['Resume', 'Remove']);
  await fireEvent.click(within(rows[1]).getByRole('button', { name: 'Resume old-box' }));
  expect(api.runnerAction).toHaveBeenCalledWith('rnr_b', 'resume');
});

test('remove asks to confirm first', async () => {
  render(RunnersPage);
  const table = await screen.findByRole('table', { name: 'Runners' });
  const row = within(table).getAllByRole('row')[1];
  await fireEvent.click(within(row).getByRole('button', { name: 'Remove agent-1' }));
  expect(api.runnerAction).not.toHaveBeenCalled();
  expect(screen.getByText(/Remove agent-1\?/)).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
  expect(api.runnerAction).toHaveBeenCalledWith('rnr_a', 'remove');
});

test('switching where jobs run saves the executor', async () => {
  render(RunnersPage);
  await screen.findByRole('table', { name: 'Runners' });
  const group = screen.getByRole('group', { name: 'Implement jobs run on' });
  expect(within(group).getByRole('button', { name: 'Runners' }).getAttribute('aria-pressed')).toBe('true');
  await fireEvent.click(within(group).getByRole('button', { name: 'This server' }));
  await waitFor(() => expect(api.putJobs).toHaveBeenCalledWith('local'));
});

test('warns when runners take jobs and none is online', async () => {
  api.getRunners.mockResolvedValue({ executor: 'runners', protocol: 1, runners: [runners[1]] });
  render(RunnersPage);
  expect(await screen.findByText(/No runner is online/)).toBeTruthy();
});

test('filters by status from the URL', async () => {
  route.search = '?status=offline';
  render(RunnersPage);
  const table = await screen.findByRole('table', { name: 'Runners' });
  const rows = within(table).getAllByRole('row').slice(1);
  expect(rows).toHaveLength(1);
  expect(within(rows[0]).getAllByRole('cell')[0].textContent).toContain('old-box');
});
