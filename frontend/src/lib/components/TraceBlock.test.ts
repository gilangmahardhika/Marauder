import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import TraceBlock from './TraceBlock.svelte';
import { buildTimeline, type Block } from '$lib/trace';
import type { TraceAttempt, TraceEvent } from '$lib/types';

const attempt: TraceAttempt = {
  id: 'att_8',
  n: 8,
  taskClass: 'implement_hard',
  modelId: 'deepseek-v4-pro',
  fallbackModelId: '',
  inputTokens: 1616,
  outputTokens: 74,
  commitSha: 'ae5d4418f1f5c54e02c2ced82de2a5b66ba65753',
  reviewModelId: 'deepseek-flash',
  reviewVerdict: 'recommend_approve',
  reviewIndependent: false,
  startedAt: '2026-10-12T03:35:13Z',
  endedAt: '2026-10-12T03:36:21Z',
  outcome: 'checks_passed'
};

function block(events: TraceEvent[]): Block {
  const [item] = buildTimeline(events, [attempt]);
  if (item.type !== 'block') throw new Error('expected a block');
  return item;
}

const clean = block([
  { id: 'm1', ts: '2026-10-12T03:35:14Z', attemptId: 'att_8', kind: 'model_call', payload: { role: 'implementer', modelId: 'deepseek-v4-pro', inputTokens: 1616, outputTokens: 74 } },
  { id: 'c1', ts: '2026-10-12T03:35:15Z', attemptId: 'att_8', kind: 'tool_call', payload: { name: 'edit_file', ok: true, summary: 'wrote resources/js/Pages/Auth/Login.tsx' } }
]);

test('a clean attempt starts closed and shows its summary', async () => {
  render(TraceBlock, { block: clean });
  const header = screen.getByRole('button', { name: /Attempt 8/ });
  expect(header.getAttribute('aria-expanded')).toBe('false');
  expect(screen.getByText('Implement hard')).toBeTruthy();
  expect(screen.getByText('Checks passed')).toBeTruthy();
  expect(screen.getByText('ae5d441')).toBeTruthy();
  expect(screen.getByText('1m 8s')).toBeTruthy();
  expect(screen.queryByText('Edit file')).toBeNull();
  await fireEvent.click(header);
  expect(header.getAttribute('aria-expanded')).toBe('true');
  expect(screen.getByText('Edit file')).toBeTruthy();
});

test('an attempt with an error starts open', () => {
  const failed = block([
    ...clean.events,
    { id: 'x1', ts: '2026-10-12T03:35:16Z', attemptId: 'att_8', kind: 'job_error', payload: { message: 'git exit 128' } }
  ]);
  render(TraceBlock, { block: failed, startOpen: failed.problems > 0 });
  expect(screen.getByRole('button', { name: /Attempt 8/ }).getAttribute('aria-expanded')).toBe('true');
  expect(screen.getByText('1 problem')).toBeTruthy();
  expect(screen.getByText('git exit 128')).toBeTruthy();
});

test('a review block shows the verdict and that it was not independent', () => {
  const review = block([
    { id: 'r1', ts: '2026-10-12T04:25:55Z', attemptId: 'att_8', kind: 'model_call', payload: { role: 'reviewer', modelId: 'deepseek-flash', inputTokens: 209, outputTokens: 368 } },
    { id: 'r2', ts: '2026-10-12T04:25:58Z', attemptId: 'att_8', kind: 'tool_call', payload: { name: 'verdict', ok: true, summary: 'recommend_approve' } }
  ]);
  render(TraceBlock, { block: review });
  expect(screen.getByRole('button', { name: /Review of attempt 8/ })).toBeTruthy();
  expect(screen.getByText('deepseek-flash')).toBeTruthy();
  expect(screen.getByText('Recommend approve')).toBeTruthy();
  expect(screen.getByText('Not independent')).toBeTruthy();
});
