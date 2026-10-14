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
  expect(screen.queryByRole('button', { name: /^Approve/ })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Request changes' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Reject' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Unblock' })).toBeNull();
});

test('approved card shows request changes and keeps reject in the menu', async () => {
  render(CardActions, {
    card: card({ status: 'approved', group: 'shipping', actions: ['request_changes', 'reject'] })
  });
  expect(screen.getByRole('button', { name: 'Request changes' })).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Reject' })).toBeNull();
  await fireEvent.click(screen.getByRole('button', { name: 'More actions' }));
  expect(screen.getByRole('button', { name: 'Reject' })).toBeTruthy();
  expect(screen.queryByRole('button', { name: /^Approve/ })).toBeNull();
});

test('approve names the head SHA and shows the AI review beside it', async () => {
  session.set('stored-bearer');
  session.setProject('proj_widget');
  const bodies: unknown[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (_url: string, init: RequestInit) => {
      bodies.push(JSON.parse(String(init.body ?? '{}')));
      return new Response(JSON.stringify(card({ status: 'approved', group: 'shipping', actions: [] })), { status: 200 });
    })
  );
  const head = 'ae5d4418f1f5c54e02c2ced82de2a5b66ba65753';
  render(CardActions, {
    card: card({
      status: 'human_review',
      group: 'review',
      actions: ['approve', 'request_changes', 'reject'],
      headSha: head,
      reviewedSha: '1111111aaaaaaa',
      reviewVerdict: 'recommend_approve'
    })
  });
  expect(screen.getByText('AI review: Recommend approve')).toBeTruthy();
  expect(screen.getByText('1111111').getAttribute('title')).toBe('1111111aaaaaaa');
  expect(screen.getByText('The head moved after the review.')).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Request changes' })).toBeTruthy();
  expect(screen.queryByLabelText('SHA')).toBeNull();

  const other = screen.getByRole('button', { name: 'Approve a different SHA' });
  expect(other.getAttribute('aria-expanded')).toBe('false');
  await fireEvent.click(other);
  const field = screen.getByLabelText('SHA') as HTMLInputElement;
  expect(field.value).toBe(head);
  await fireEvent.input(field, { target: { value: 'bbbbbbb2222222' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Approve bbbbbbb' }));
  await vi.waitFor(() => expect(bodies).toEqual([{ sha: 'bbbbbbb2222222' }]));
});

test('approve without a head SHA shows the SHA field', () => {
  render(CardActions, { card: card({ status: 'human_review', group: 'review', actions: ['approve'] }) });
  expect(screen.getByRole('button', { name: 'Approve SHA' })).toBeTruthy();
  expect(screen.getByLabelText('SHA')).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Approve a different SHA' })).toBeNull();
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

test('merge shows Squash and merge instead of deploy and posts to merge', async () => {
  session.set('stored-bearer');
  session.setProject('proj_widget');
  const calls: { url: string; method: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, method: String(init.method) });
      return new Response(JSON.stringify(card({ status: 'done', group: 'finished', actions: [] })), { status: 200 });
    })
  );
  render(CardActions, {
    card: card({ status: 'approved', group: 'shipping', approvedSha: 'ae5d4418f1f5c54e02c2ced82de2a5b66ba65753', actions: ['request_changes', 'reject', 'merge'] })
  });
  expect(screen.queryByRole('button', { name: 'Deploy approved SHA' })).toBeNull();
  await fireEvent.click(screen.getByRole('button', { name: 'Squash and merge' }));
  await vi.waitFor(() => expect(screen.queryByRole('button', { name: 'Squash and merge' })).toBeNull());
  expect(calls).toEqual([{ url: 'http://127.0.0.1:8787/api/projects/proj_widget/cards/card_0123456789ab/merge', method: 'POST' }]);
});

test("won't fix asks for a reason beside the primary Unblock and posts it", async () => {
  session.set('stored-bearer');
  session.setProject('proj_widget');
  const calls: { url: string; body: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, body: String(init.body ?? '') });
      return new Response(JSON.stringify(card({ status: 'wont_fix', group: 'closed', actions: ['reopen'] })), { status: 200 });
    })
  );
  render(CardActions, {
    card: card({ status: 'blocked', group: 'stopped', actions: ['unblock', 'wont_fix'], unblockTo: 'ready', blockedReason: 'attempt_cap' })
  });
  expect(screen.getByRole('button', { name: 'Unblock' }).className).toContain('bg-accent');
  const open = screen.getByRole('button', { name: "Won't fix" });
  expect(open.className).not.toContain('bg-accent');
  await fireEvent.click(open);
  const submit = screen.getByRole('button', { name: "Close as won't fix" }) as HTMLButtonElement;
  expect(submit.disabled).toBe(true);
  await fireEvent.input(screen.getByLabelText("Why won't this be fixed?"), { target: { value: '  Superseded by REQ-009 ' } });
  expect(submit.disabled).toBe(false);
  await fireEvent.click(submit);
  await vi.waitFor(() => expect(screen.getByRole('button', { name: 'Reopen' })).toBeTruthy());
  expect(calls).toEqual([
    {
      url: 'http://127.0.0.1:8787/api/projects/proj_widget/cards/card_0123456789ab/wont-fix',
      body: JSON.stringify({ reason: 'Superseded by REQ-009' })
    }
  ]);
});

test('reopen posts to reopen', async () => {
  session.set('stored-bearer');
  session.setProject('proj_widget');
  const calls: { url: string; method: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, method: String(init.method) });
      return new Response(JSON.stringify(card({ status: 'blocked', group: 'stopped', actions: [] })), { status: 200 });
    })
  );
  render(CardActions, { card: card({ status: 'wont_fix', group: 'closed', actions: ['reopen'] }) });
  expect(screen.getByText('Reopen returns this card to Blocked.')).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Reopen' }));
  await vi.waitFor(() => expect(screen.queryByRole('button', { name: 'Reopen' })).toBeNull());
  expect(calls).toEqual([{ url: 'http://127.0.0.1:8787/api/projects/proj_widget/cards/card_0123456789ab/reopen', method: 'POST' }]);
});

test('blocked card with null unblockTo has no unblock button', () => {
  render(CardActions, {
    card: card({ status: 'blocked', group: 'stopped', actions: [], unblockTo: null, blockedReason: 'source_changed' })
  });
  expect(screen.queryByRole('button', { name: 'Unblock' })).toBeNull();
  expect(screen.getByText('Accept the changed requirements in Intake before this card can be released.')).toBeTruthy();
});
