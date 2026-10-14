import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import type { UsageRow } from '$lib/types';

const api = vi.hoisted(() => ({ ApiError: class ApiError extends Error {}, getUsage: vi.fn() }));
const route = vi.hoisted(() => ({ search: '' }));
const nav = vi.hoisted(() => ({ goto: vi.fn() }));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: {}, url: new URL(`http://127.0.0.1:4173/admin/usage${route.search}`) }).subscribe(run)
  }
}));
vi.mock('$app/navigation', () => ({ goto: nav.goto }));
vi.mock('$lib/api', () => ({
  ApiError: api.ApiError,
  unreachable: () => 'API unreachable',
  getUsage: api.getUsage
}));

import UsagePage from './+page.svelte';

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

const rows: UsageRow[] = [
  row({ calls: 3, inputTokens: 300, outputTokens: 30, cachedInputTokens: 100 }),
  row({ task: 'review', familyId: 'openai-main', protocol: 'openai', modelId: 'gpt-b', inputTokens: 50, outputTokens: 5 }),
  row({ day: '2026-10-03', projectId: 'proj_gadget', projectName: 'Gadget', userId: 'usr_ben', userName: '', userEmail: 'ben@example.com', task: 'refine_requirement', source: 'intake', cardId: '', cardTitle: '', inputTokens: 40, outputTokens: 4 }),
  row({ day: '2026-10-03', familyId: '', protocol: '', modelId: 'retired', userId: '', userName: '', userEmail: '', cardId: 'card_loose', cardTitle: 'Loose card', inputTokens: 7, outputTokens: 1 })
];

function header(name: string) {
  return within(screen.getByRole('table', { name: /^Token usage by/ })).getByRole('columnheader', { name });
}

function lastParams() {
  const url = nav.goto.mock.calls.at(-1)?.[0] as URL;
  return url.searchParams;
}

async function groupNames(name: string) {
  const table = await screen.findByRole('table', { name });
  return within(table)
    .getAllByRole('row')
    .slice(1)
    .map((tr) => within(tr).getAllByRole('cell')[0].textContent?.trim());
}

beforeEach(() => {
  route.search = '';
  api.getUsage.mockResolvedValue({ from: '2026-10-01', to: '2026-10-03', rows });
});

afterEach(() => {
  vi.clearAllMocks();
});

test('renders the totals, a bar per day, and models by total', async () => {
  render(UsagePage);
  expect(await groupNames('Token usage by model')).toEqual(['claude-a', 'gpt-b', 'retired']);
  const [from, to] = api.getUsage.mock.calls[0] as [string, string];
  expect((Date.parse(to) - Date.parse(from)) / 86_400_000).toBe(29);
  const totals = screen.getByLabelText('Totals');
  expect(within(totals).getByText('Total tokens').nextElementSibling?.textContent).toBe('437');
  expect(within(totals).getByText('Calls').nextElementSibling?.textContent).toBe('6');
  expect(within(totals).getByText('Cache share').nextElementSibling?.textContent).toBe('20%');
  const days = screen.getByRole('table', { name: 'Tokens per day' });
  expect(within(days).getAllByRole('row')).toHaveLength(4);
  expect(header('Total').getAttribute('aria-sort')).toBe('descending');
  expect(header('Calls').getAttribute('aria-sort')).toBe('none');
});

test('a filter from the URL changes every total, and picking one sets the param', async () => {
  route.search = '?llm=-';
  render(UsagePage);
  expect(await groupNames('Token usage by model')).toEqual(['retired']);
  expect(within(screen.getByLabelText('Totals')).getByText('Total tokens').nextElementSibling?.textContent).toBe('8');
  const model = screen.getByRole('combobox', { name: 'Model' });
  expect(within(model).getAllByRole('option').map((option) => option.textContent)).toEqual(['All models', 'retired · 8']);
  await fireEvent.change(model, { target: { value: 'retired' } });
  expect(lastParams().get('model')).toBe('retired');
  await fireEvent.click(screen.getAllByRole('button', { name: 'Clear filters' })[0]);
  expect(lastParams().get('llm')).toBeNull();
});

test('groups by another dimension through the URL', async () => {
  route.search = '?by=user';
  render(UsagePage);
  expect(await groupNames('Token usage by user')).toEqual(['Ana', 'ben@example.com', 'No owner']);
  await fireEvent.click(within(screen.getByRole('group', { name: 'Group by' })).getByRole('button', { name: 'Task' }));
  expect(lastParams().get('by')).toBe('task');
  await fireEvent.click(within(screen.getByRole('group', { name: 'Group by' })).getByRole('button', { name: 'Model' }));
  expect(lastParams().has('by')).toBe(false);
});

test('sorts by a column header in either direction', async () => {
  route.search = '?sort=name&dir=desc';
  render(UsagePage);
  expect(await groupNames('Token usage by model')).toEqual(['retired', 'gpt-b', 'claude-a']);
  expect(header('Model').getAttribute('aria-sort')).toBe('descending');
  await fireEvent.click(within(header('Model')).getByRole('button'));
  expect(lastParams().get('sort')).toBe('name');
  expect(lastParams().has('dir')).toBe(false);
  await fireEvent.click(within(header('Calls')).getByRole('button'));
  expect(lastParams().get('sort')).toBe('calls');
  expect(lastParams().has('dir')).toBe(false);
});

test('changing the range loads that many days', async () => {
  render(UsagePage);
  await screen.findByRole('table', { name: 'Token usage by model' });
  await fireEvent.click(screen.getByRole('button', { name: '7 days' }));
  expect(lastParams().get('range')).toBe('7');
});

test('shows the empty state when the range has no calls', async () => {
  api.getUsage.mockResolvedValue({ from: '2026-10-01', to: '2026-10-03', rows: [] });
  render(UsagePage);
  expect(await screen.findByText('No model calls in this range')).toBeTruthy();
  expect(screen.queryByRole('table')).toBeNull();
});

test('a member who is not an admin sees the refusal', async () => {
  api.getUsage.mockRejectedValue(new api.ApiError('forbidden'));
  render(UsagePage);
  expect((await screen.findByRole('alert')).textContent).toContain('forbidden');
  expect(screen.getByRole('button', { name: 'Try again' })).toBeTruthy();
  expect(screen.queryByLabelText('Totals')).toBeNull();
});
test('groups by card and filters by source', async () => {
  route.search = '?by=card';
  const { unmount } = render(UsagePage);
  expect(await groupNames('Token usage by card')).toEqual(['Ana card', 'Intake', 'Loose card']);
  const table = screen.getByRole('table', { name: 'Token usage by card' });
  const cells = within(table).getAllByRole('row').slice(1).map((tr) => within(tr).getAllByRole('cell')[0]);
  expect(cells.map((cell) => cell.getAttribute('title'))).toEqual(['card_ana', null, 'card_loose']);
  const groupBy = within(screen.getByRole('group', { name: 'Group by' }));
  expect(groupBy.getByRole('button', { name: 'Source' })).toBeTruthy();
  expect(groupBy.getByRole('button', { name: 'Card' })).toBeTruthy();
  unmount();

  route.search = '?source=intake';
  render(UsagePage);
  expect(await groupNames('Token usage by model')).toEqual(['claude-a']);
  expect(within(screen.getByLabelText('Totals')).getByText('Total tokens').nextElementSibling?.textContent).toBe('44');
  await fireEvent.click(screen.getAllByRole('button', { name: 'Clear filters' })[0]);
  expect(lastParams().has('source')).toBe(false);
});
