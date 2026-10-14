import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, expect, test, vi } from 'vitest';
import QuestionModeField from './QuestionModeField.svelte';
import { session } from '$lib/session';
import { toast } from '$lib/toast.svelte';
import type { Card, QuestionMode } from '$lib/types';

afterEach(() => {
  vi.unstubAllGlobals();
  toast.clear();
});

function card(questionMode: QuestionMode, actions: string[]): Card {
  return {
    id: 'card_0123456789ab',
    title: 'Export orders',
    status: 'ready',
    group: 'waiting',
    actions,
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
    questionMode,
    approvedSha: null,
    deployedSha: null,
    environment: 'dev',
    failureReason: null,
    blockedReason: null,
    updatedAt: '2026-10-14T05:00:00Z'
  };
}

test('the control saves auto and the hint follows', async () => {
  session.set('stored-bearer');
  session.setProject('proj_widget');
  const calls: { url: string; method: string; body: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, method: String(init.method ?? 'GET'), body: String(init.body ?? '') });
      return new Response(JSON.stringify(card('auto', ['question_mode'])), { status: 200 });
    })
  );
  render(QuestionModeField, { card: card('manual', ['question_mode']) });
  expect(screen.getByRole('group', { name: 'When the agent has a question' })).toBeTruthy();
  expect(screen.getByText('The card waits and the owner is notified.')).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Agent decides' }));
  expect(await screen.findByText('The agent picks its own suggested answer and keeps going.')).toBeTruthy();
  expect(calls).toEqual([
    {
      url: expect.stringContaining('/api/projects/proj_widget/cards/card_0123456789ab/question-mode'),
      method: 'PUT',
      body: '{"questionMode":"auto"}'
    }
  ]);
  expect(screen.getByRole('button', { name: 'Agent decides' }).getAttribute('aria-pressed')).toBe('true');
});

test('a failed save keeps manual', async () => {
  session.set('stored-bearer');
  session.setProject('proj_widget');
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify({ error: { code: 'forbidden', message: 'forbidden' } }), { status: 403 }))
  );
  render(QuestionModeField, { card: card('manual', ['question_mode']) });
  await fireEvent.click(screen.getByRole('button', { name: 'Agent decides' }));
  await vi.waitFor(() => expect(toast.items.map((item) => `${item.tone}: ${item.text}`)).toEqual(['error: forbidden']));
  expect(screen.getByRole('button', { name: 'A person answers' }).getAttribute('aria-pressed')).toBe('true');
  expect(screen.getByText('The card waits and the owner is notified.')).toBeTruthy();
});

test('without the action the mode reads as text', () => {
  render(QuestionModeField, { card: card('manual', []) });
  expect(screen.getByText('Questions: A person answers')).toBeTruthy();
  expect(screen.queryByRole('button')).toBeNull();
});
