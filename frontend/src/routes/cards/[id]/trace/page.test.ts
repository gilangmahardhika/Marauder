import { render, screen } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';
import type { TraceAttempt, TraceEvent, TraceResponse } from '$lib/types';

const api = vi.hoisted(() => ({ getTrace: vi.fn(), getCard: vi.fn() }));

vi.mock('$app/stores', () => ({
  page: readable({ params: { id: 'card_1' }, url: new URL('http://127.0.0.1:4173/cards/card_1/trace') })
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  getTrace: api.getTrace,
  getCard: api.getCard
}));

import TracePage from './+page.svelte';

const attempt: TraceAttempt = {
  id: 'att_1',
  n: 1,
  taskClass: 'implement',
  modelId: 'model-a',
  fallbackModelId: '',
  inputTokens: 900,
  outputTokens: 100,
  maxTurns: 12,
  commitSha: '',
  reviewModelId: '',
  reviewVerdict: '',
  reviewIndependent: null,
  startedAt: '2026-10-12T10:00:00Z',
  endedAt: '',
  outcome: ''
};

const first: TraceEvent = {
  id: 'e1',
  seq: 1,
  ts: '2026-10-12T10:00:01Z',
  attemptId: 'att_1',
  kind: 'model_call',
  payload: { role: 'implementer', modelId: 'model-a', text: 'I will read the login page.', inputTokens: 900, outputTokens: 100 }
};

const second: TraceEvent = {
  id: 'e2',
  seq: 2,
  ts: '2026-10-12T10:00:04Z',
  attemptId: 'att_1',
  kind: 'tool_call',
  payload: { name: 'read_file', ok: true, summary: 'a\nb', args: { path: 'src/login.ts' } }
};

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'setTimeout', 'clearTimeout', 'Date'] });
  vi.setSystemTime(new Date('2026-10-12T10:00:10Z'));
  Element.prototype.scrollIntoView = vi.fn();
  shell.projectId = 'proj_default';
  api.getCard.mockResolvedValue({ id: 'card_1', title: 'Fix login', status: 'in_progress' });
});

afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
  shell.projectId = '';
});

test('polls with since while live, merges new rows, and stops when the card is done', async () => {
  const responses: TraceResponse[] = [
    { attempts: [attempt], events: [first], live: true },
    { attempts: [attempt], events: [first, second], live: true },
    { attempts: [{ ...attempt, endedAt: '2026-10-12T10:00:20Z', outcome: 'checks_passed' }], events: [second], live: false }
  ];
  api.getTrace.mockImplementation(() => Promise.resolve(responses.shift()));
  render(TracePage);

  await vi.advanceTimersByTimeAsync(0);
  expect(api.getTrace).toHaveBeenCalledTimes(1);
  expect(api.getTrace).toHaveBeenLastCalledWith('card_1', undefined);
  expect(screen.getByText('Live')).toBeTruthy();
  expect(screen.getByRole('region', { name: 'Now running' }).textContent).toContain('Turn 1 of 12');

  await vi.advanceTimersByTimeAsync(3000);
  expect(api.getTrace).toHaveBeenLastCalledWith('card_1', first.ts);
  expect(screen.getAllByText('src/login.ts · 2 lines').length).toBeGreaterThan(0);

  await vi.advanceTimersByTimeAsync(3000);
  expect(api.getTrace).toHaveBeenLastCalledWith('card_1', second.ts);
  expect(api.getTrace).toHaveBeenCalledTimes(3);
  expect(screen.queryByText('Live')).toBeNull();
  expect(screen.getByText('I will read the login page.')).toBeTruthy();
  expect(shell.message).toBe('Attempt 1 ended: Checks passed');

  await vi.advanceTimersByTimeAsync(9000);
  expect(api.getTrace).toHaveBeenCalledTimes(3);
});

test('does not poll a card that is not live', async () => {
  api.getTrace.mockResolvedValue({ attempts: [{ ...attempt, endedAt: '2026-10-12T10:00:20Z', outcome: 'checks_passed' }], events: [first], live: false });
  render(TracePage);
  await vi.advanceTimersByTimeAsync(10000);
  expect(api.getTrace).toHaveBeenCalledTimes(1);
  expect(screen.queryByRole('region', { name: 'Now running' })).toBeNull();
});
