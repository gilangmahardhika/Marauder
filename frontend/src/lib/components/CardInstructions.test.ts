import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, expect, test, vi } from 'vitest';
import CardInstructions from './CardInstructions.svelte';
import { session } from '$lib/session';
import { toast } from '$lib/toast.svelte';
import type { Card } from '$lib/types';

afterEach(() => {
  vi.unstubAllGlobals();
  toast.clear();
});

function card(instructions: string | null): Card {
  return {
    id: 'card_0123456789ab',
    title: 'Parse headings',
    status: 'changes_requested',
    group: 'running',
    actions: ['run', 'instructions'],
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
    attemptsUsed: 1,
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
    updatedAt: '2026-10-12T05:00:00Z',
    instructions
  };
}

test('no instructions say what attempts follow', () => {
  render(CardInstructions, { card: card(null) });
  expect(screen.getByRole('heading', { name: 'Instructions' })).toBeTruthy();
  expect(screen.getByText('No instructions. Attempts follow the requirements and the newest note.')).toBeTruthy();
});

test('instructions render markdown and save through the API', async () => {
  session.set('stored-bearer');
  session.setProject('proj_widget');
  const calls: { url: string; method: string; body: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, method: String(init.method ?? 'GET'), body: String(init.body ?? '') });
      const body = JSON.parse(String(init.body ?? '{}'));
      if (body.instructions.length > 20) {
        return new Response(
          JSON.stringify({
            error: { code: 'validation', message: 'too long', fields: { instructions: 'Keep them short.' } }
          }),
          { status: 422 }
        );
      }
      return new Response(JSON.stringify(card(body.instructions || null)), { status: 200 });
    })
  );
  render(CardInstructions, { card: card('Run the **lint** script.') });
  expect(screen.getByText('lint').tagName).toBe('STRONG');

  await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
  const box = screen.getByLabelText('Instructions for later attempts') as HTMLTextAreaElement;
  expect(box.value).toBe('Run the **lint** script.');
  await fireEvent.input(box, { target: { value: 'Run the lint script and keep tsc.' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(await screen.findByText('Keep them short.')).toBeTruthy();
  expect(box.getAttribute('aria-invalid')).toBe('true');
  expect(document.activeElement).toBe(box);
  expect(toast.items).toEqual([]);

  await fireEvent.input(box, { target: { value: 'Keep tsc.' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(await screen.findByText('Keep tsc.')).toBeTruthy();
  expect(screen.queryByLabelText('Instructions for later attempts')).toBeNull();
  expect(toast.items.map((item) => item.text)).toEqual(['Instructions saved']);
  expect(calls.map((call) => `${call.method} ${call.url}`)).toEqual([
    'PUT http://127.0.0.1:8787/api/projects/proj_widget/cards/card_0123456789ab/instructions',
    'PUT http://127.0.0.1:8787/api/projects/proj_widget/cards/card_0123456789ab/instructions'
  ]);
  expect(JSON.parse(calls[1].body)).toEqual({ instructions: 'Keep tsc.' });
});
