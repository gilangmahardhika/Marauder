import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import CardActions from './CardActions.svelte';
import type { Card } from '$lib/types';

function card(partial: Partial<Card>): Card {
  return {
    id: 'card_0123456789ab',
    title: 'Parse headings',
    status: 'in_progress',
    group: 'running',
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
    approvedSha: null,
    deployedSha: null,
    environment: 'dev',
    failureReason: null,
    blockedReason: null,
    updatedAt: '2026-10-10T05:00:00Z',
    ...partial
  };
}

test('empty actions render no controls', () => {
  render(CardActions, { card: card({ actions: [] }) });
  expect(screen.queryByRole('button', { name: 'Approve SHA' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Request changes' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Reject' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Unblock' })).toBeNull();
});

test('approved card renders request changes and reject', async () => {
  render(CardActions, {
    card: card({ status: 'approved', group: 'shipping', actions: ['request_changes', 'reject'] })
  });
  await fireEvent.click(screen.getByRole('button', { name: 'Actions' }));
  expect(screen.getByRole('button', { name: 'Request changes' })).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Reject' })).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Approve SHA' })).toBeNull();
});

test('blocked card with null unblockTo has no unblock button', () => {
  render(CardActions, {
    card: card({ status: 'blocked', group: 'stopped', actions: [], unblockTo: null, blockedReason: 'source_changed' })
  });
  expect(screen.queryByRole('button', { name: 'Unblock' })).toBeNull();
  expect(screen.getByText('Accept the changed requirements in Intake before this card can be released.')).toBeTruthy();
});
