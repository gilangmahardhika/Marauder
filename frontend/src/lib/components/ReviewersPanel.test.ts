import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import ReviewersPanel from './ReviewersPanel.svelte';
import { canSetSelfReview } from '$lib/requirements';
import type { Requirement } from '$lib/types';

const noop = () => {};

function req(partial: Partial<Requirement> = {}): Requirement {
  return {
    id: 'REQ-001',
    sourceKind: 'manual',
    sourcePath: '',
    heading: 'REQ-001',
    statement: 'text',
    acceptance: [],
    accepted: false,
    createdBy: null,
    linkedCards: [],
    reviewers: [],
    canEditReviewers: false,
    selfReview: false,
    canSetSelfReview: false,
    aiReviewMode: 'continue',
    ...partial
  };
}

test('only a Lead Developer or an admin may set Self review', () => {
  expect(canSetSelfReview('lead_developer', false)).toBe(true);
  expect(canSetSelfReview('product_owner', true)).toBe(true);
  expect(canSetSelfReview('developer', false)).toBe(false);
  expect(canSetSelfReview('product_owner', false)).toBe(false);
});

test('a lead sees the Self review checkbox', () => {
  render(ReviewersPanel, {
    requirement: req({ canEditReviewers: true, canSetSelfReview: true, selfReview: true }),
    options: [],
    onsaved: noop,
    onmessage: noop
  });
  const box = screen.getByRole('checkbox', { name: /Self review/ }) as HTMLInputElement;
  expect(box.checked).toBe(true);
  expect(screen.getByRole('button', { name: 'Save reviewers' })).toBeTruthy();
});

test('others see a read-only Self review line', () => {
  render(ReviewersPanel, {
    requirement: req({ selfReview: true }),
    options: [],
    onsaved: noop,
    onmessage: noop
  });
  expect(screen.queryByRole('checkbox', { name: /Self review/ })).toBeNull();
  expect(screen.getByText(/Self review: on/)).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Save reviewers' })).toBeNull();
});

test('nothing about Self review shows when it is off and locked', () => {
  render(ReviewersPanel, { requirement: req(), options: [], onsaved: noop, onmessage: noop });
  expect(screen.queryByText(/Self review/)).toBeNull();
});

test('a lead chooses what an AI request for changes does', async () => {
  render(ReviewersPanel, {
    requirement: req({ canEditReviewers: true, canSetSelfReview: true, aiReviewMode: 'manual' }),
    options: [],
    onsaved: noop,
    onmessage: noop
  });
  const group = screen.getByRole('group', { name: 'When the AI review asks for changes' });
  const manual = within(group).getByRole('button', { name: 'A person reviews' });
  const next = within(group).getByRole('button', { name: 'Run the next attempt' });
  expect(manual.getAttribute('aria-pressed')).toBe('true');
  const save = screen.getByRole('button', { name: 'Save reviewers' }) as HTMLButtonElement;
  expect(save.disabled).toBe(true);
  await fireEvent.click(next);
  expect(next.getAttribute('aria-pressed')).toBe('true');
  expect(save.disabled).toBe(false);
});

test('others see a read-only line only when the mode is manual', () => {
  const { unmount } = render(ReviewersPanel, {
    requirement: req({ aiReviewMode: 'manual' }),
    options: [],
    onsaved: noop,
    onmessage: noop
  });
  expect(screen.queryByRole('group', { name: 'When the AI review asks for changes' })).toBeNull();
  expect(screen.getByText(/a person reviews before another attempt/)).toBeTruthy();
  unmount();
  render(ReviewersPanel, { requirement: req(), options: [], onsaved: noop, onmessage: noop });
  expect(screen.queryByText(/a person reviews before another attempt/)).toBeNull();
});
