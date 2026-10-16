import { render, screen, waitFor } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { toast } from '$lib/toast.svelte';
import type { TribeSummary } from '$lib/types';

const summary: TribeSummary = {
  from: '2026-09-17',
  to: '2026-10-16',
  groups: {
    waiting: 2,
    question: 0,
    running: 1,
    review: 0,
    shipping: 0,
    finished: 5,
    stopped: 0,
    closed: 0
  },
  attention: {
    readyCount: 2,
    waitingAnswer: 0,
    humanReview: 0,
    blocked: [],
    openRuns: 1,
    locked: 1,
    implementationCap: 4,
    openQuestions: 0
  },
  range: {
    done: 5,
    wontFix: 1,
    rework: 1,
    budgetStops: [],
    answerSources: { person: 1, suggestion: 0, auto: 2 },
    medianAnswerHours: 1.2
  },
  projects: [
    {
      projectId: 'proj_a',
      projectName: 'Shop',
      groups: {
        waiting: 2,
        question: 0,
        running: 1,
        review: 0,
        shipping: 0,
        finished: 5,
        stopped: 0,
        closed: 0
      },
      attention: {
        readyCount: 2,
        waitingAnswer: 0,
        humanReview: 0,
        blocked: [],
        openRuns: 1,
        locked: 1,
        implementationCap: 2,
        openQuestions: 0
      }
    }
  ],
  mcp: { changed: 1, toolErrors: 0, missingMentions: 2 }
};

const api = vi.hoisted(() => ({
  getTribeSummary: vi.fn(),
  getTribes: vi.fn()
}));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({
        params: { id: 'tribe_a' },
        url: new URL('http://127.0.0.1:4173/tribes/tribe_a/overview')
      }).subscribe(run)
  }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  ...api
}));

import TribeOverviewPage from './+page.svelte';

beforeEach(() => {
  api.getTribeSummary.mockResolvedValue(summary);
  api.getTribes.mockResolvedValue({ tribes: [{ id: 'tribe_a', name: 'Alpha' }] });
});

afterEach(() => {
  vi.clearAllMocks();
  toast.clear();
});

test('tribe overview shows projects and MCP health', async () => {
  render(TribeOverviewPage);
  expect(await screen.findByRole('heading', { name: 'Overview: Alpha' })).toBeTruthy();
  await waitFor(() => expect(api.getTribeSummary).toHaveBeenCalledWith('tribe_a', expect.any(String), expect.any(String)));
  expect(screen.getByText('Shop')).toBeTruthy();
  expect(screen.getByText('MCP health')).toBeTruthy();
  expect(screen.getByText('Schema changed')).toBeTruthy();
  expect(screen.getByRole('link', { name: 'Back to tribe' }).getAttribute('href')).toBe('/tribes/tribe_a');
  expect(screen.getByRole('link', { name: 'Token usage' }).getAttribute('href')).toBe('/tribes/tribe_a/usage');
});
