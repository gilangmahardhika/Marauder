import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, test, vi } from 'vitest';
import type { Card } from '$lib/types';
import BoardCard from './BoardCard.svelte';

function card(partial: Partial<Card>): Card {
  return {
    id: 'card_0123456789ab',
    title: 'Parse headings',
    status: 'ready',
    group: 'waiting',
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
    updatedAt: '2026-10-10T05:00:00Z',
    ...partial
  };
}

test('Run shows only when the API lists the run action', () => {
  const onrun = vi.fn();
  const { unmount } = render(BoardCard, { card: card({ actions: [] }), onrun });
  expect(screen.queryByRole('button', { name: 'Run' })).toBeNull();
  unmount();
  render(BoardCard, { card: card({ actions: ['run'] }), onrun });
  expect(screen.getByRole('button', { name: 'Run' })).toBeTruthy();
});

test('a queued card shows Queued and a waiting card counts its dependencies', () => {
  const onrun = vi.fn();
  const { unmount } = render(BoardCard, { card: card({ queued: true }), onrun });
  expect(screen.getByText('Queued')).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Run' })).toBeNull();
  unmount();
  render(BoardCard, { card: card({ waitingOn: ['card_a', 'card_b'] }), onrun });
  expect(screen.getByText('Waiting on 2 cards')).toBeTruthy();
});

test('clicking Run calls onrun and sits outside the card link', async () => {
  const onrun = vi.fn();
  const item = card({ actions: ['run'] });
  render(BoardCard, { card: item, onrun });
  const button = screen.getByRole('button', { name: 'Run' });
  expect(button.closest('a')).toBeNull();
  await fireEvent.click(button);
  expect(onrun).toHaveBeenCalledWith(item);
});
