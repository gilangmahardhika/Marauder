import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, expect, test, vi } from 'vitest';
import { ApiError } from '$lib/api';
import { canSetSelfReview } from '$lib/requirements';
import { toast } from '$lib/toast.svelte';
import type { Requirement } from '$lib/types';

const api = vi.hoisted(() => ({ setReviewers: vi.fn(), setSelfReview: vi.fn(), setAIReviewMode: vi.fn() }));

vi.mock('$lib/api', async (original) => ({ ...(await original<typeof import('$lib/api')>()), ...api }));

import ReviewersPanel from './ReviewersPanel.svelte';

const noop = () => {};

afterEach(() => {
  vi.clearAllMocks();
  toast.clear();
});

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
    onsaved: noop
  });
  const box = screen.getByRole('checkbox', { name: /Self review/ }) as HTMLInputElement;
  expect(box.checked).toBe(true);
  expect(screen.getByRole('button', { name: 'Save reviewers' })).toBeTruthy();
});

test('others see a read-only Self review line', () => {
  render(ReviewersPanel, {
    requirement: req({ selfReview: true }),
    options: [],
    onsaved: noop
  });
  expect(screen.queryByRole('checkbox', { name: /Self review/ })).toBeNull();
  expect(screen.getByText(/Self review: on/)).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Save reviewers' })).toBeNull();
});

test('nothing about Self review shows when it is off and locked', () => {
  render(ReviewersPanel, { requirement: req(), options: [], onsaved: noop });
  expect(screen.queryByText(/Self review/)).toBeNull();
});

test('a lead chooses what an AI request for changes does', async () => {
  render(ReviewersPanel, {
    requirement: req({ canEditReviewers: true, canSetSelfReview: true, aiReviewMode: 'manual' }),
    options: [],
    onsaved: noop
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
    onsaved: noop
  });
  expect(screen.queryByRole('group', { name: 'When the AI review asks for changes' })).toBeNull();
  expect(screen.getByText(/a person reviews before another attempt/)).toBeTruthy();
  unmount();
  render(ReviewersPanel, { requirement: req(), options: [], onsaved: noop });
  expect(screen.queryByText(/a person reviews before another attempt/)).toBeNull();
});

test('the editable panel says unverified members are asked after they verify', () => {
  render(ReviewersPanel, {
    requirement: req({ canEditReviewers: true }),
    options: [{ id: 'usr_hendro', name: 'Hendro', email: 'hendro@example.com', githubLogin: '' }],
    onsaved: noop
  });
  expect(screen.getByText(/Someone without a verified GitHub login is asked to review on GitHub after they verify\./)).toBeTruthy();
  expect(screen.getByRole('combobox', { name: 'Reviewers' }).getAttribute('placeholder')).toBe('Search by name, login, or email');
});

test('saving says so in a toast and hands the intake back', async () => {
  const onsaved = vi.fn();
  api.setAIReviewMode.mockResolvedValue({ requirements: [] });
  render(ReviewersPanel, { requirement: req({ canSetSelfReview: true }), options: [], onsaved });
  await fireEvent.click(screen.getByRole('button', { name: 'A person reviews' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Save reviewers' }));
  await waitFor(() => expect(toast.items.map((item) => [item.tone, item.text])).toEqual([['success', 'Reviewers saved']]));
  expect(api.setAIReviewMode).toHaveBeenCalledWith('REQ-001', 'manual');
  expect(onsaved).toHaveBeenCalledOnce();
});

test('a refused save marks the control and shows the error in a toast', async () => {
  api.setAIReviewMode.mockRejectedValue(new ApiError(422, 'invalid', 'validation failed', { aiReviewMode: 'Pick continue or manual.' }));
  render(ReviewersPanel, { requirement: req({ canSetSelfReview: true }), options: [], onsaved: noop });
  await fireEvent.click(screen.getByRole('button', { name: 'A person reviews' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Save reviewers' }));
  expect(await screen.findByText('Pick continue or manual.')).toBeTruthy();
  expect(toast.items.map((item) => [item.tone, item.text])).toEqual([['error', 'Pick continue or manual.']]);
});
