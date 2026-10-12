import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, expect, test, vi } from 'vitest';
import CardActions from './CardActions.svelte';
import { session } from '$lib/session';
import type { Card } from '$lib/types';

afterEach(() => {
  vi.unstubAllGlobals();
});

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

test('request changes asks for a reason and sends it', async () => {
  session.set('stored-bearer');
  session.setProject('proj_widget');
  const calls: { url: string; body: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, body: String(init.body ?? '') });
      const body = JSON.parse(String(init.body ?? '{}'));
      if (body.reason.length > 10) {
        return new Response(
          JSON.stringify({
            error: { code: 'validation', message: 'too long', fields: { reason: 'Keep the reason to 10 characters.' } }
          }),
          { status: 422 }
        );
      }
      return new Response(JSON.stringify(card({ status: 'changes_requested', group: 'running', actions: [] })), {
        status: 200
      });
    })
  );
  render(CardActions, {
    card: card({ status: 'human_review', group: 'review', actions: ['approve', 'request_changes'] })
  });
  await fireEvent.click(screen.getByRole('button', { name: 'Actions' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Request changes' }));
  const box = screen.getByLabelText('What should change?');

  await fireEvent.input(box, { target: { value: 'Use the lint scripts instead of tsc.' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Request changes' }));
  expect(await screen.findByText('Keep the reason to 10 characters.')).toBeTruthy();

  await fireEvent.input(box, { target: { value: 'Use lint.' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Request changes' }));
  await vi.waitFor(() => expect(screen.queryByLabelText('What should change?')).toBeNull());
  expect(calls.map((call) => call.url)).toEqual([
    'http://127.0.0.1:8787/api/projects/proj_widget/cards/card_0123456789ab/request-changes',
    'http://127.0.0.1:8787/api/projects/proj_widget/cards/card_0123456789ab/request-changes'
  ]);
  expect(JSON.parse(calls[1].body)).toEqual({ reason: 'Use lint.' });
});

test('run opens a box prefilled with the instructions and sends only changed text', async () => {
  session.set('stored-bearer');
  session.setProject('proj_widget');
  const bodies: unknown[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (_url: string, init: RequestInit) => {
      const body = JSON.parse(String(init.body ?? '{}'));
      bodies.push(body);
      if (typeof body.instructions === 'string' && body.instructions.length > 20) {
        return new Response(
          JSON.stringify({
            error: { code: 'validation', message: 'too long', fields: { instructions: 'Keep them short.' } }
          }),
          { status: 422 }
        );
      }
      return new Response(JSON.stringify(card({ status: 'changes_requested', queued: true, actions: [] })), {
        status: 200
      });
    })
  );
  const saved = 'Use the lint script.';
  const first = render(CardActions, {
    card: card({ status: 'changes_requested', actions: ['run', 'instructions'], instructions: saved })
  });
  await fireEvent.click(screen.getByRole('button', { name: 'Run' }));
  const box = screen.getByLabelText('Instructions for the next attempt') as HTMLTextAreaElement;
  expect(box.value).toBe(saved);
  await fireEvent.click(screen.getByRole('button', { name: 'Run' }));
  await vi.waitFor(() => expect(screen.queryByLabelText('Instructions for the next attempt')).toBeNull());
  first.unmount();

  render(CardActions, {
    card: card({ status: 'changes_requested', actions: ['run', 'instructions'], instructions: saved })
  });
  await fireEvent.click(screen.getByRole('button', { name: 'Run' }));
  const next = screen.getByLabelText('Instructions for the next attempt');
  await fireEvent.input(next, { target: { value: 'Use the lint script and keep tsc.' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Run' }));
  expect(await screen.findByText('Keep them short.')).toBeTruthy();
  await fireEvent.input(next, { target: { value: 'Keep tsc.' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Run' }));
  await vi.waitFor(() => expect(screen.queryByLabelText('Instructions for the next attempt')).toBeNull());

  expect(bodies).toEqual([{}, { instructions: 'Use the lint script and keep tsc.' }, { instructions: 'Keep tsc.' }]);
});

test('retry AI review shows only when listed and posts to retry-review', async () => {
  session.set('stored-bearer');
  session.setProject('proj_widget');
  const first = render(CardActions, { card: card({ status: 'agent_review', group: 'review', actions: [] }) });
  expect(screen.queryByRole('button', { name: 'Retry AI review' })).toBeNull();
  first.unmount();

  const calls: { url: string; method: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, method: String(init.method) });
      return new Response(JSON.stringify(card({ status: 'pr_open', group: 'review', actions: [] })), { status: 200 });
    })
  );
  render(CardActions, {
    card: card({ status: 'agent_review', group: 'review', reviewVerdict: 'failed', actions: ['retry_review'] })
  });
  await fireEvent.click(screen.getByRole('button', { name: 'Retry AI review' }));
  await vi.waitFor(() => expect(screen.queryByRole('button', { name: 'Retry AI review' })).toBeNull());
  expect(calls).toEqual([
    { url: 'http://127.0.0.1:8787/api/projects/proj_widget/cards/card_0123456789ab/retry-review', method: 'POST' }
  ]);
});

test('blocked card with null unblockTo has no unblock button', () => {
  render(CardActions, {
    card: card({ status: 'blocked', group: 'stopped', actions: [], unblockTo: null, blockedReason: 'source_changed' })
  });
  expect(screen.queryByRole('button', { name: 'Unblock' })).toBeNull();
  expect(screen.getByText('Accept the changed requirements in Intake before this card can be released.')).toBeTruthy();
});
