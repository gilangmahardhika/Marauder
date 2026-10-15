import { render, screen } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';
import type { Intake, Requirement } from '$lib/types';

const api = vi.hoisted(() => ({ getIntake: vi.fn(), ingest: vi.fn(), acceptRequirements: vi.fn() }));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: {}, url: new URL('http://127.0.0.1:4173/intake') }).subscribe(run)
  }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  ...api
}));

import IntakePage from './+page.svelte';

const open: Requirement = {
  id: 'REQ-001',
  sourceKind: 'manual',
  sourcePath: '',
  heading: '',
  statement: 'Export the board.',
  acceptance: [],
  accepted: false,
  createdBy: 'usr_ada',
  linkedCards: [],
  reviewers: [],
  canEditReviewers: false,
  selfReview: false,
  canSetSelfReview: false,
  aiReviewMode: 'continue'
};

const unnumbered = ['docs/product/a.md', 'docs/product/b.md', 'docs/product/c.md', 'docs/product/d.md', 'docs/product/e.md', 'docs/product/f.md', 'docs/product/g.md'];

const intake: Intake = {
  draftPending: true,
  unnumbered,
  planError: null,
  canAccept: true,
  requirements: [open],
  decisions: [],
  reviewerOptions: []
};

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  shell.projectId = 'proj_default';
  api.getIntake.mockResolvedValue(intake);
});

afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
  shell.projectId = '';
});

test('PRD files with no REQ- headings show a notice, no spinner, and Accept stays enabled', async () => {
  render(IntakePage);
  expect(await screen.findByText(/7 PRD files have no REQ- headings/)).toBeTruthy();
  for (const path of unnumbered.slice(0, 5)) expect(screen.getByText(path)).toBeTruthy();
  expect(screen.queryByText('docs/product/f.md')).toBeNull();
  expect(screen.getByText('and 2 more')).toBeTruthy();
  expect(screen.getByRole('link', { name: 'Repository settings' }).getAttribute('href')).toBe('/settings/repository');
  expect(screen.queryByText(/Planner is drafting/)).toBeNull();

  const accept = screen.getByRole('button', { name: /Accept all shown/ }) as HTMLButtonElement;
  expect(accept.disabled).toBe(false);

  await vi.advanceTimersByTimeAsync(10_000);
  expect(api.getIntake).toHaveBeenCalledTimes(1);
});

test('no notice when every PRD file has REQ- headings', async () => {
  api.getIntake.mockResolvedValue({ ...intake, draftPending: false, unnumbered: [] });
  render(IntakePage);
  expect(await screen.findByText('Export the board.')).toBeTruthy();
  expect(screen.queryByText(/no REQ- headings/)).toBeNull();
});
