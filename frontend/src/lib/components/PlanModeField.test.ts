import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, expect, test, vi } from 'vitest';
import PlanModeField from './PlanModeField.svelte';
import { session } from '$lib/session';
import { toast } from '$lib/toast.svelte';
import type { Card, PlanMode } from '$lib/types';

afterEach(() => {
  vi.unstubAllGlobals();
  toast.clear();
});

function card(planMode: PlanMode, actions: string[]): Card {
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
    questionMode: 'manual',
    planMode,
    approvedSha: null,
    deployedSha: null,
    environment: 'dev',
    failureReason: null,
    blockedReason: null,
    updatedAt: '2026-10-16T05:00:00Z'
  };
}

test('the control saves off', async () => {
  session.set('stored-bearer');
  session.setProject('proj_widget');
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => {
      return new Response(JSON.stringify(card('off', ['plan_mode'])), { status: 200 });
    })
  );
  render(PlanModeField, { card: card('required', ['plan_mode']) });
  await fireEvent.click(screen.getByRole('button', { name: 'Skip planning' }));
  await waitFor(() =>
    expect(toast.items.map((item) => item.text)).toContain('Planning skipped for this card')
  );
});

test('without the action it reads the mode', () => {
  render(PlanModeField, { card: card('required', []) });
  expect(screen.getByText('Plan: Plan before coding')).toBeTruthy();
});
