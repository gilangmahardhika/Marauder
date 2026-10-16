import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, expect, test, vi } from 'vitest';
import CardPlan from './CardPlan.svelte';
import { session } from '$lib/session';
import { toast } from '$lib/toast.svelte';
import type { Card } from '$lib/types';

afterEach(() => {
  vi.unstubAllGlobals();
  toast.clear();
});

function openCard(actions = ['accept_plan', 'plan_mode']): Card {
  return {
    id: 'card_0123456789ab',
    title: 'Export orders',
    status: 'waiting_answer',
    group: 'question',
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
    questionMode: 'manual',
    planMode: 'required',
    plan: {
      id: 'pln_1',
      attemptN: 1,
      title: 'Add CSV export',
      steps: [
        { id: 'stp_1', text: 'Find the handler', detail: '' },
        { id: 'stp_2', text: 'Write the CSV', detail: 'Reuse auth' }
      ],
      status: 'open',
      acceptedBy: null,
      acceptedAt: null,
      createdAt: '2026-10-16T05:00:00Z'
    },
    approvedSha: null,
    deployedSha: null,
    environment: 'dev',
    failureReason: null,
    blockedReason: null,
    updatedAt: '2026-10-16T05:00:00Z'
  };
}

test('accept posts the edited steps', async () => {
  session.set('stored-bearer');
  session.setProject('proj_widget');
  const calls: { url: string; body: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      calls.push({ url, body: String(init?.body ?? '') });
      const next = { ...openCard(), status: 'ready', group: 'waiting', actions: ['run'], queued: true, plan: { ...openCard().plan!, status: 'accepted' } };
      return new Response(JSON.stringify(next), { status: 200 });
    })
  );
  render(CardPlan, { card: openCard() });
  expect(screen.getByText('Add CSV export')).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: /Accept plan/i }));
  await waitFor(() => expect(calls.some((c) => c.url.includes('/plan/accept'))).toBe(true));
  await waitFor(() =>
    expect(toast.items.map((item) => item.text).some((t) => /Plan accepted/i.test(t))).toBe(true)
  );
});

test('skip is shown when listed', () => {
  render(CardPlan, { card: openCard(['accept_plan', 'skip_plan']) });
  expect(screen.getByRole('button', { name: /Skip plan/i })).toBeTruthy();
});

test('steps render as text until clicked', async () => {
  render(CardPlan, { card: openCard() });
  expect(screen.getByText('Find the handler')).toBeTruthy();
  expect(screen.getByText('Reuse auth')).toBeTruthy();
  expect(screen.queryByRole('textbox', { name: /^Step 1$/i })).toBeNull();
  expect(screen.getAllByRole('button', { name: /Drag to reorder/i }).length).toBe(2);
  expect(screen.getAllByRole('button', { name: /Edit step/i }).length).toBe(2);

  await fireEvent.click(screen.getByRole('button', { name: /Edit step 1/i }));
  expect(screen.getByRole('textbox', { name: /^Step 1$/i })).toBeTruthy();
  expect(screen.getByText(/Detail/i)).toBeTruthy();
});

test('clicking Edit step keeps the textarea mounted', async () => {
  render(CardPlan, { card: openCard() });
  await fireEvent.click(screen.getByRole('button', { name: /Edit step 1/i }));
  const box = screen.getByRole('textbox', { name: /^Step 1$/i });
  expect(box).toBeTruthy();
  await fireEvent.focusOut(box, { relatedTarget: null });
  expect(screen.getByRole('textbox', { name: /^Step 1$/i })).toBeTruthy();
});

test('Done and Escape close step edit', async () => {
  render(CardPlan, { card: openCard() });
  await fireEvent.click(screen.getByRole('button', { name: /Edit step 1/i }));
  expect(screen.getByRole('textbox', { name: /^Step 1$/i })).toBeTruthy();

  await fireEvent.click(screen.getByRole('button', { name: /^Done$/i }));
  expect(screen.queryByRole('textbox', { name: /^Step 1$/i })).toBeNull();
  expect(screen.getByRole('button', { name: /Edit step 1/i })).toBeTruthy();

  await fireEvent.click(screen.getByRole('button', { name: /Edit step 2/i }));
  const box = screen.getByRole('textbox', { name: /^Step 2$/i });
  await fireEvent.keyDown(box, { key: 'Escape' });
  expect(screen.queryByRole('textbox', { name: /^Step 2$/i })).toBeNull();
});

test('arrow keys on drag handle reorder steps', async () => {
  render(CardPlan, { card: openCard() });
  const handles = screen.getAllByRole('button', { name: /Drag to reorder/i });
  await fireEvent.keyDown(handles[0], { key: 'ArrowDown' });
  const edits = screen.getAllByRole('button', { name: /Edit step/i });
  expect(edits[0].textContent).toMatch(/Write the CSV/);
  expect(edits[1].textContent).toMatch(/Find the handler/);
});
