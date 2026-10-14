import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';
import { toast } from '$lib/toast.svelte';
import type { Card, CardDiff } from '$lib/types';

const api = vi.hoisted(() => ({ getCard: vi.fn(), getCardDiff: vi.fn() }));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: { id: 'card_1' }, url: new URL('http://127.0.0.1:4173/cards/card_1') }).subscribe(run)
  }
}));
const stream = vi.hoisted(() => ({ handlers: [] as ((event: unknown) => void)[] }));
vi.mock('$lib/stream.svelte', () => ({
  live: {
    connected: true,
    on: (handler: (event: unknown) => void) => {
      stream.handlers.push(handler);
      return () => {
        stream.handlers = stream.handlers.filter((item) => item !== handler);
      };
    }
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
  questionMode: 'manual',
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
  toast.clear();
});

test('the first load shows a loading row, then the card', async () => {
  render(CardPage);
  expect(screen.getByRole('status').textContent).toContain('Loading card');
  expect(await screen.findByRole('heading', { level: 1, name: 'Change login label' })).toBeTruthy();
  expect(screen.queryByText('Loading card')).toBeNull();
});

test('a failed load shows the error with Try again', async () => {
  api.getCard.mockRejectedValueOnce(new Error('The board is down.'));
  render(CardPage);
  const alert = await screen.findByRole('alert');
  expect(alert.textContent).toContain('The board is down.');
  await fireEvent.click(within(alert).getByRole('button', { name: 'Try again' }));
  expect(await screen.findByRole('heading', { level: 1, name: 'Change login label' })).toBeTruthy();
  expect(api.getCard).toHaveBeenCalledTimes(2);
});

test('a failed reload keeps the card on screen and shows an error toast', async () => {
  render(CardPage);
  await screen.findByRole('heading', { level: 1, name: 'Change login label' });
  api.getCard.mockRejectedValueOnce(new Error('The board is down.'));
  for (const handler of stream.handlers) handler({ kind: 'cards', full: true, cards: [], removed: [] });
  await vi.waitFor(() => expect(toast.items.map((item) => `${item.tone}: ${item.text}`)).toEqual(['error: The board is down.']));
  expect(screen.getByRole('heading', { level: 1, name: 'Change login label' })).toBeTruthy();
  expect(screen.queryByText('Loading card')).toBeNull();
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

test('a pushed change to this card reads it again; other cards do not', async () => {
  render(CardPage);
  await screen.findByRole('heading', { name: 'REQ-001: Change login label' });
  expect(api.getCard).toHaveBeenCalledTimes(1);

  for (const handler of stream.handlers)
    handler({ kind: 'cards', full: false, cards: [{ ...card, id: 'card_2' }], removed: [] });
  await Promise.resolve();
  expect(api.getCard).toHaveBeenCalledTimes(1);

  api.getCard.mockResolvedValue({ ...card, status: 'approved', actions: ['deploy'], approvedSha: head });
  for (const handler of stream.handlers)
    handler({ kind: 'cards', full: false, cards: [{ ...card, status: 'approved' }], removed: [] });
  expect(await screen.findByRole('button', { name: /Deploy/ })).toBeTruthy();
  expect(api.getCard).toHaveBeenCalledTimes(2);
  expect(screen.queryByRole('button', { name: 'Approve ae5d441' })).toBeNull();
});

test('open questions replace the next step controls, and Details starts with the answer mode', async () => {
  const question = {
    id: 'qst_000000000001',
    attemptN: 1,
    question: 'Which format?',
    kind: 'single' as const,
    options: ['CSV', 'Excel'],
    allowOther: false,
    recommended: { selected: ['CSV'], text: '' },
    why: '',
    answer: null,
    answerSource: null,
    answeredBy: null,
    answeredAt: null,
    createdAt: '2026-10-14T05:00:00Z'
  };
  api.getCard.mockResolvedValue({
    ...card,
    status: 'waiting_answer',
    group: 'waiting',
    actions: ['answer', 'question_mode', 'context'],
    questions: [question]
  });
  render(CardPage);
  expect(await screen.findByText('The agent has a question')).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Submit answers' })).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Approve ae5d441' })).toBeNull();
  expect(screen.getByText('Needs answer')).toBeTruthy();
  const aside = screen.getByRole('complementary', { name: 'Card details' });
  const details = within(aside).getByRole('heading', { name: 'Details' }).closest('section')!;
  const body = [...details.children].find((child) => child.tagName === 'DIV');
  expect(body?.firstElementChild?.textContent ?? '').toContain('When the agent has a question');
  expect(document.getElementById('questions')).not.toBeNull();
});

test('without the answer action the next step keeps its controls and the mode reads as text', async () => {
  render(CardPage);
  expect(await screen.findByRole('button', { name: 'Approve ae5d441' })).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Submit answers' })).toBeNull();
  expect(screen.getByText('Questions: A person answers')).toBeTruthy();
  expect(document.getElementById('questions')).toBeNull();
});

test('a card with nothing to do says so in the next step', async () => {
  api.getCard.mockResolvedValue({ ...card, status: 'in_progress', actions: [], prUrl: null, pullRequest: null });
  render(CardPage);
  expect(await screen.findByText('No action is open for you right now.')).toBeTruthy();
  expect(screen.queryByRole('link', { name: 'Open pull request' })).toBeNull();
  expect(screen.getByText(/No pull request yet/)).toBeTruthy();
});
