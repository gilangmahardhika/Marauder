import { render, screen, within } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';
import type { Card, CardDiff } from '$lib/types';

const api = vi.hoisted(() => ({ getCard: vi.fn(), getCardDiff: vi.fn() }));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: { id: 'card_1' }, url: new URL('http://127.0.0.1:4173/cards/card_1') }).subscribe(run)
  }
}));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  getCard: api.getCard,
  getCardDiff: api.getCardDiff
}));

import CardPage from './+page.svelte';

const head = 'ae5d4418f1f5c54e02c2ced82de2a5b66ba65753';

const card: Card = {
  id: 'card_1',
  title: 'Change login label',
  status: 'human_review',
  group: 'review',
  actions: ['approve', 'request_changes', 'reject'],
  unblockTo: null,
  owner: { id: 'user_1', name: 'Grace Hopper', email: 'grace@example.com', githubLogin: 'grace' },
  reviewers: [],
  skills: [],
  requirements: [],
  notes: [],
  pullRequest: { title: 'REQ-001: Change login label', body: '## Summary\n\nUsername only.', attemptN: 1, sha: head },
  requirementIds: ['REQ-001'],
  decisionIds: [],
  dependsOn: [],
  queued: false,
  waitingOn: [],
  risk: 'low',
  service: 'default',
  manual: false,
  localCheck: 'make test',
  devCheck: null,
  attemptsUsed: 1,
  maxAttempts: 3,
  branch: 'orch/card_1',
  prUrl: 'https://github.com/acme/widget/pull/7',
  headSha: head,
  reviewedSha: head,
  reviewIndependent: true,
  reviewVerdict: 'recommend_approve',
  selfReview: false,
  aiReviewMode: 'continue',
  approvedSha: null,
  deployedSha: null,
  environment: 'dev',
  failureReason: null,
  blockedReason: null,
  updatedAt: '2026-10-14T05:00:00Z'
};

const diff: CardDiff = {
  sha: head,
  base: 'main',
  compareUrl: '',
  missing: false,
  truncated: false,
  files: [
    {
      path: 'src/Login.tsx',
      added: 1,
      deleted: 1,
      binary: false,
      truncated: false,
      hunks: [{ header: '@@ -1,2 +1,2 @@', lines: [{ kind: 'del', text: 'Username or email' }, { kind: 'add', text: 'Username' }] }]
    }
  ]
};

beforeEach(() => {
  shell.projectId = 'proj_default';
  api.getCard.mockResolvedValue(card);
  api.getCardDiff.mockResolvedValue(diff);
});

afterEach(() => {
  vi.clearAllMocks();
  shell.projectId = '';
});

test('orders the main column by priority and puts facts in the sidebar', async () => {
  render(CardPage);
  await screen.findByRole('heading', { name: 'REQ-001: Change login label' });
  await screen.findByText('Username or email');

  const main = screen.getByTestId('card-main');
  const panels = within(main)
    .getAllByRole('heading', { level: 2 })
    .map((heading) => heading.textContent?.trim());
  expect(panels.slice(0, 3)).toEqual(['Next step', 'Pull request #7', 'Changes']);

  const aside = screen.getByRole('complementary', { name: 'Card details' });
  expect(within(aside).getAllByRole('heading', { level: 2 }).map((heading) => heading.textContent?.trim())).toEqual([
    'Details',
    'People',
    'Intake'
  ]);
  expect(within(aside).getAllByTitle(head).length).toBe(2);
});

test('the header links the pull request and the next step leads with approve', async () => {
  render(CardPage);
  expect(await screen.findByRole('button', { name: 'Approve ae5d441' })).toBeTruthy();
  expect(screen.getByRole('link', { name: 'Open pull request' }).getAttribute('href')).toBe(card.prUrl);
  expect(screen.getByRole('link', { name: 'View trace' }).getAttribute('href')).toBe('/cards/card_1/trace');
  expect(screen.getByText('Attempt 1 of 3')).toBeTruthy();
  expect(screen.getByText('main')).toBeTruthy();
});

test('a card with nothing to do says so in the next step', async () => {
  api.getCard.mockResolvedValue({ ...card, status: 'in_progress', actions: [], prUrl: null, pullRequest: null });
  render(CardPage);
  expect(await screen.findByText('No action is open for you right now.')).toBeTruthy();
  expect(screen.queryByRole('link', { name: 'Open pull request' })).toBeNull();
  expect(screen.getByText(/No pull request yet/)).toBeTruthy();
});
