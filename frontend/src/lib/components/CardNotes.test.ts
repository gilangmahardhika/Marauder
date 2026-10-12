import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import CardNotes from './CardNotes.svelte';
import type { CardNote } from '$lib/types';

const review: CardNote = {
  source: 'review',
  note: 'The frontend job does not do what the card requires:\n\n1. Run the **lint** scripts.\n2. Read `.nvmrc`.',
  createdAt: '2026-10-12T05:00:00Z',
  attemptN: 2
};
const check: CardNote = {
  source: 'local_check',
  note: '**not bold** in check output',
  createdAt: '2026-10-12T04:00:00Z',
  attemptN: 1
};

test('no notes render nothing', () => {
  const { container } = render(CardNotes, { notes: [] });
  expect(container.textContent?.trim()).toBe('');
});

test('the newest review note renders markdown and earlier notes wait behind a button', async () => {
  render(CardNotes, { notes: [review, check] });
  expect(screen.getByRole('heading', { name: 'Notes' })).toBeTruthy();
  expect(screen.getByText('AI review')).toBeTruthy();
  expect(screen.getByText('· attempt 2')).toBeTruthy();
  expect(screen.getByText('lint').tagName).toBe('STRONG');
  expect(screen.getByText('.nvmrc').tagName).toBe('CODE');
  expect(screen.queryByText('Local check')).toBeNull();

  await fireEvent.click(screen.getByRole('button', { name: 'Show 1 earlier note' }));
  expect(screen.getByText('Local check')).toBeTruthy();
  expect(screen.getByText('**not bold** in check output').tagName).toBe('PRE');
});

test('instructions render markdown and a blank one reads as cleared', async () => {
  render(CardNotes, {
    notes: [
      { source: 'instructions', note: '', createdAt: '2026-10-12T07:00:00Z', attemptN: 0 },
      { source: 'instructions', note: 'Run the **lint** script.', createdAt: '2026-10-12T06:00:00Z', attemptN: 0 }
    ]
  });
  expect(screen.getByText('Instructions')).toBeTruthy();
  expect(screen.getByText('Instructions cleared')).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Show 1 earlier note' }));
  expect(screen.getByText('lint').tagName).toBe('STRONG');
});

test('the current instructions and the newest other note both stay open', async () => {
  render(CardNotes, {
    notes: [
      { source: 'instructions', note: 'Use a self-hosted runner.', createdAt: '2026-10-12T08:00:00Z', attemptN: 0 },
      review,
      check
    ]
  });
  expect(screen.getByText('Use a self-hosted runner.')).toBeTruthy();
  expect(screen.getByText('AI review')).toBeTruthy();
  expect(screen.queryByText('Local check')).toBeNull();
  await fireEvent.click(screen.getByRole('button', { name: 'Show 1 earlier note' }));
  expect(screen.getByText('Local check')).toBeTruthy();
});

test('an approver note has no attempt number', () => {
  render(CardNotes, {
    notes: [{ source: 'approver', note: 'changes requested', createdAt: '2026-10-12T06:00:00Z', attemptN: 0 }]
  });
  expect(screen.getByText('Approver')).toBeTruthy();
  expect(screen.queryByText(/· attempt/)).toBeNull();
});
