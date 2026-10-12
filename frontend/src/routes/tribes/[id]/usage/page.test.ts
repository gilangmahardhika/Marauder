import { render, screen, waitFor } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';
import type { UsageRow } from '$lib/types';

const api = vi.hoisted(() => ({ getTribeUsage: vi.fn(), getTribes: vi.fn() }));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: { id: 'tribe_gadget' }, url: new URL('http://127.0.0.1:4173/tribes/tribe_gadget/usage') }).subscribe(run)
  }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  getTribeUsage: api.getTribeUsage,
  getTribes: api.getTribes
}));

import TribeUsagePage from './+page.svelte';

const row: UsageRow = {
  day: '2026-10-01',
  projectId: 'proj_gadget',
  projectName: 'Gadget',
  familyId: 'anthropic-main',
  protocol: 'anthropic',
  modelId: 'claude-a',
  task: 'implement',
  userId: 'usr_ben',
  userName: 'Ben',
  userEmail: 'ben@example.com',
  source: 'card',
  cardId: 'card_ben',
  cardTitle: 'Ben card',
  calls: 2,
  inputTokens: 200,
  outputTokens: 20,
  cachedInputTokens: 0
};

beforeEach(() => {
  api.getTribes.mockResolvedValue({ tribes: [{ id: 'tribe_gadget', name: 'Gadget tribe' }] });
  api.getTribeUsage.mockResolvedValue({ from: '2026-10-01', to: '2026-10-03', rows: [row] });
});

afterEach(() => {
  vi.clearAllMocks();
});

test('reads the route tribe and titles the page with its name', async () => {
  render(TribeUsagePage);
  expect(await screen.findByRole('heading', { name: 'Token usage: Gadget tribe' })).toBeTruthy();
  const [id, from, to] = api.getTribeUsage.mock.calls[0] as [string, string, string];
  expect(id).toBe('tribe_gadget');
  expect((Date.parse(to) - Date.parse(from)) / 86_400_000).toBe(29);
  await waitFor(() => expect(shell.title).toBe('Token usage: Gadget tribe'));
  expect(screen.getByRole('link', { name: 'Back to tribe' }).getAttribute('href')).toBe('/tribes/tribe_gadget');
  expect(screen.getByRole('combobox', { name: 'User' })).toBeTruthy();
  expect(await screen.findByRole('table', { name: 'Token usage by model' })).toBeTruthy();
});
