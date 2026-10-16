import { render, screen, waitFor } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';
import { session } from '$lib/session';
import { toast } from '$lib/toast.svelte';
import type { ProjectSummary } from '$lib/types';

const summary: ProjectSummary = {
  from: '2026-09-17',
  to: '2026-10-16',
  groups: {
    waiting: 2,
    question: 1,
    running: 0,
    review: 1,
    shipping: 0,
    finished: 4,
    stopped: 1,
    closed: 0
  },
  attention: {
    readyCount: 2,
    waitingAnswer: 1,
    humanReview: 1,
    blocked: [{ reason: 'token_cap', count: 1 }],
    openRuns: 0,
    locked: 0,
    implementationCap: 2,
    openQuestions: 1
  },
  aging: [
    {
      cardId: 'card_old',
      title: 'Export CSV',
      status: 'waiting_answer',
      updatedAt: '2026-10-01T12:00:00Z'
    }
  ],
  range: {
    done: 3,
    wontFix: 0,
    rework: 2,
    budgetStops: [{ cap: 'token_cap', count: 1 }],
    answerSources: { person: 2, suggestion: 1, auto: 0 },
    medianAnswerHours: 4.5
  }
};

const api = vi.hoisted(() => ({
  getProjectSummary: vi.fn(),
  getProjects: vi.fn()
}));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({
        params: {},
        url: new URL('http://127.0.0.1:4173/overview')
      }).subscribe(run)
  }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  ...api
}));

import OverviewPage from './+page.svelte';

beforeEach(() => {
  session.setProject('proj_a');
  shell.projectId = 'proj_a';
  shell.admin = false;
  shell.tribeAdminOf = [];
  api.getProjectSummary.mockResolvedValue(summary);
  api.getProjects.mockResolvedValue({
    projects: [{ id: 'proj_a', tribeId: 'tribe_a', name: 'Shop', repo: 'shop', baseBranch: 'main' }]
  });
});

afterEach(() => {
  vi.clearAllMocks();
  toast.clear();
  session.clear();
  shell.projectId = '';
  shell.admin = false;
  shell.tribeAdminOf = [];
});

test('project overview shows pipeline and attention totals', async () => {
  render(OverviewPage);
  expect(await screen.findByRole('heading', { name: 'Overview: Shop' })).toBeTruthy();
  await waitFor(() => expect(api.getProjectSummary).toHaveBeenCalled());
  expect(screen.getByText('Ready to run')).toBeTruthy();
  expect(screen.getAllByText('Needs answer').length).toBeGreaterThan(0);
  expect(screen.getAllByText('token_cap').length).toBeGreaterThan(0);
  expect(screen.getByRole('link', { name: 'Export CSV' }).getAttribute('href')).toBe('/cards/card_old');
  expect(screen.getByRole('link', { name: 'Board' }).getAttribute('href')).toBe('/');
});

test('a failed first load shows Try again', async () => {
  api.getProjectSummary.mockRejectedValueOnce(new Error('API unreachable'));
  render(OverviewPage);
  expect(await screen.findByText('API unreachable')).toBeTruthy();
  api.getProjectSummary.mockResolvedValue(summary);
  await screen.findByRole('button', { name: 'Try again' }).then((btn) => btn.click());
  expect(await screen.findByRole('heading', { name: 'Overview: Shop' })).toBeTruthy();
});
