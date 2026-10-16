import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import type { UsageRow } from '$lib/types';

const api = vi.hoisted(() => ({ getMyUsage: vi.fn() }));
const route = vi.hoisted(() => ({ search: '' }));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: {}, url: new URL(`http://127.0.0.1:4173/account/usage${route.search}`) }).subscribe(run)
  }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  getMyUsage: api.getMyUsage
}));

import MyUsagePage from './+page.svelte';

function row(over: Partial<UsageRow>): UsageRow {
  return {
    day: '2026-10-01',
    projectId: 'proj_default',
    projectName: 'Default',
    familyId: 'anthropic-main',
    protocol: 'anthropic',
    modelId: 'claude-a',
    task: 'implement',
    userId: 'usr_ana',
    userName: 'Ana',
    userEmail: 'ana@example.com',
    source: 'card',
    cardId: 'card_ana',
    cardTitle: 'Ana card',
    calls: 1,
    inputTokens: 100,
    outputTokens: 10,
    cachedInputTokens: 0,
    ...over
  };
}

beforeEach(() => {
  route.search = '';
  api.getMyUsage.mockResolvedValue({
    from: '2026-10-01',
    to: '2026-10-03',
    rows: [row({}), row({ modelId: 'gpt-b', familyId: 'openai-main', task: 'refine_requirement', source: 'intake', cardId: '', cardTitle: '', inputTokens: 40, outputTokens: 4 })]
  });
});

afterEach(() => {
  vi.clearAllMocks();
});

test('reads my usage and offers Source and Card but no User filter or grouping', async () => {
  render(MyUsagePage);
  expect(await screen.findByRole('table', { name: 'Token usage by model' })).toBeTruthy();
  expect(api.getMyUsage).toHaveBeenCalledTimes(1);
  expect(screen.getByRole('heading', { name: 'My token usage' })).toBeTruthy();
  expect(screen.queryByRole('combobox', { name: 'User' })).toBeNull();
  await fireEvent.click(screen.getByRole('combobox', { name: 'Group by' }));
  expect(
    within(screen.getByRole('listbox', { name: 'Group by' }))
      .getAllByRole('option')
      .map((option) => option.textContent?.trim())
  ).toEqual(['LLM', 'Model', 'Project', 'Task', 'Source', 'Card']);
  await fireEvent.click(screen.getByRole('combobox', { name: 'Add filter' }));
  expect(screen.queryByRole('option', { name: 'User' })).toBeNull();
  expect(
    within(screen.getByRole('listbox', { name: 'Add filter' }))
      .getAllByRole('option')
      .map((option) => option.textContent?.trim())
  ).toEqual(['LLM', 'Model', 'Project', 'Task', 'Source', 'Card']);
});

test('a User grouping or filter in the URL is ignored', async () => {
  route.search = '?by=user&user=usr_other';
  render(MyUsagePage);
  expect(await screen.findByRole('table', { name: 'Token usage by model' })).toBeTruthy();
  const totals = screen.getByLabelText('Totals');
  expect(within(totals).getByText('Total tokens').nextElementSibling?.textContent).toBe('154');
});
