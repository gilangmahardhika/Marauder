import { fireEvent, render, screen } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { ApiError } from '$lib/api';
import { shell } from '$lib/shell.svelte';
import { toast } from '$lib/toast.svelte';
import type { Board, Card } from '$lib/types';

const api = vi.hoisted(() => ({ getCards: vi.fn(), runCard: vi.fn(), startRun: vi.fn() }));
const stream = vi.hoisted(() => ({ connected: true, handlers: [] as ((event: unknown) => void)[] }));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: {}, url: new URL('http://127.0.0.1:4173/') }).subscribe(run)
  }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/stream.svelte', () => ({
  live: {
    get connected() {
      return stream.connected;
    },
    on: (handler: (event: unknown) => void) => {
      stream.handlers.push(handler);
      return () => {
        stream.handlers = stream.handlers.filter((item) => item !== handler);
      };
    }
  }
}));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {
    constructor(
      public status: number,
      public code: string,
      message: string
    ) {
      super(message);
    }
  },
  getCards: api.getCards,
  runCard: api.runCard,
  startRun: api.startRun
}));

import BoardPage from './+page.svelte';

function card(id: string, title: string, status: string, group: string): Card {
  return {
    id,
    title,
    status,
    group,
    actions: [],
    unblockTo: null,
    owner: null,
    reviewers: [],
    skills: [],
    requirementIds: [],
    decisionIds: [],
    dependsOn: [],
    queued: false,
    waitingOn: [],
    risk: 'low',
    service: 'default',
    manual: false,
    localCheck: null,
    devCheck: null,
    attemptsUsed: 0,
    maxAttempts: 3,
    branch: null,
    prUrl: null,
    headSha: null,
    reviewedSha: null,
    reviewIndependent: null,
    reviewVerdict: null,
    selfReview: false,
    aiReviewMode: 'continue',
    approvedSha: null,
    deployedSha: null,
    environment: 'dev',
    failureReason: null,
    blockedReason: null,
    updatedAt: '2026-10-14T05:00:00Z'
  } as Card;
}

const board: Board = {
  canRun: true,
  readyCount: 0,
  running: true,
  cards: [card('card_1', 'Export CSV', 'in_progress', 'running')]
};

function push(event: unknown) {
  for (const handler of stream.handlers) handler(event);
}

beforeEach(() => {
  vi.useFakeTimers();
  shell.projectId = 'proj_default';
  api.getCards.mockResolvedValue(board);
});

afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
  shell.projectId = '';
  stream.connected = true;
  stream.handlers = [];
  toast.clear();
});

test('the first read shows a loading row, and a failed read offers Try again', async () => {
  api.getCards.mockRejectedValueOnce(new Error('board read failed'));
  render(BoardPage);
  expect(screen.getByRole('status').textContent).toContain('Loading board');
  await vi.advanceTimersByTimeAsync(0);
  expect(screen.getByRole('alert').textContent).toContain('board read failed');
  await fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
  await vi.advanceTimersByTimeAsync(0);
  expect(screen.getByText('Export CSV')).toBeTruthy();
  expect(screen.queryByRole('alert')).toBeNull();
});

test('Run ready cards queues and says how many in a toast', async () => {
  api.getCards.mockResolvedValue({ ...board, readyCount: 2, running: false });
  api.startRun.mockResolvedValue({ queued: 2 });
  render(BoardPage);
  await vi.advanceTimersByTimeAsync(0);
  await fireEvent.click(screen.getByRole('button', { name: 'Run ready cards (2)' }));
  await vi.advanceTimersByTimeAsync(0);
  expect(api.startRun).toHaveBeenCalledOnce();
  expect(toast.items.map((item) => [item.tone, item.text])).toEqual([['success', 'Queued 2 cards.']]);
});

test('a 409 on a card Run reloads the board with an info toast', async () => {
  const ready = { ...card('card_2', 'Import CSV', 'ready', 'waiting'), actions: ['run'] };
  api.getCards.mockResolvedValue({ ...board, readyCount: 1, running: false, cards: [ready] });
  api.runCard.mockRejectedValue(new ApiError(409, 'conflict', 'card changed'));
  render(BoardPage);
  await vi.advanceTimersByTimeAsync(0);
  await fireEvent.click(screen.getByRole('button', { name: 'Run' }));
  await vi.advanceTimersByTimeAsync(0);
  expect(api.getCards).toHaveBeenCalledTimes(2);
  expect(toast.items.map((item) => [item.tone, item.text])).toEqual([['info', 'Card changed. Reloaded.']]);
});

test('the board reads once and does not poll while the stream is open', async () => {
  render(BoardPage);
  await vi.advanceTimersByTimeAsync(0);
  expect(screen.getByText('Export CSV')).toBeTruthy();
  await vi.advanceTimersByTimeAsync(10_000);
  expect(api.getCards).toHaveBeenCalledTimes(1);
});

test('pushed frames move cards and update the run button', async () => {
  render(BoardPage);
  await vi.advanceTimersByTimeAsync(0);
  push({ kind: 'cards', full: false, cards: [card('card_1', 'Export CSV', 'human_review', 'review')], removed: [] });
  push({ kind: 'cards', full: false, cards: [card('card_2', 'Import CSV', 'ready', 'waiting')], removed: [] });
  push({ kind: 'board', readyCount: 1, running: false, canRun: true });
  await vi.advanceTimersByTimeAsync(0);
  const review = screen.getByRole('region', { name: 'Review' });
  expect(review.textContent).toContain('Export CSV');
  expect(screen.getByRole('region', { name: 'Waiting' }).textContent).toContain('Import CSV');
  expect(screen.getByRole('button', { name: 'Run ready cards (1)' })).toBeTruthy();

  push({ kind: 'cards', full: false, cards: [], removed: ['card_2'] });
  await vi.advanceTimersByTimeAsync(0);
  expect(screen.queryByText('Import CSV')).toBeNull();
  expect(api.getCards).toHaveBeenCalledTimes(1);
});

test('the board polls every 3 seconds while the stream is down and work runs', async () => {
  stream.connected = false;
  render(BoardPage);
  await vi.advanceTimersByTimeAsync(0);
  await vi.advanceTimersByTimeAsync(3000);
  expect(api.getCards).toHaveBeenCalledTimes(2);
});
