import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';
import type { Intake, Requirement } from '$lib/types';

const api = vi.hoisted(() => ({
  getIntake: vi.fn(),
  updateRequirement: vi.fn(),
  refineRequirement: vi.fn(),
  deleteRequirement: vi.fn()
}));

vi.mock('$app/stores', () => ({
  page: { subscribe: (run: (value: unknown) => void) => readable({ params: { id: 'REQ-005' } }).subscribe(run) }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  getIntake: api.getIntake,
  updateRequirement: api.updateRequirement,
  refineRequirement: api.refineRequirement,
  deleteRequirement: api.deleteRequirement,
  setReviewers: vi.fn(),
  setSelfReview: vi.fn(),
  setAIReviewMode: vi.fn()
}));

import RequirementPage from './+page.svelte';

const requirement: Requirement = {
  id: 'REQ-005',
  sourceKind: 'manual',
  sourcePath: '',
  heading: 'REQ-005',
  statement: 'Export the board.',
  acceptance: ['Works well.'],
  accepted: true,
  createdBy: 'dev@example.com',
  linkedCards: [],
  reviewers: [],
  canEditReviewers: false,
  selfReview: false,
  canSetSelfReview: false,
  aiReviewMode: 'continue'
};

const intake = (req: Requirement): Intake => ({ requirements: [req], reviewerOptions: [] }) as unknown as Intake;

beforeEach(() => {
  shell.projectId = 'proj_default';
  api.getIntake.mockResolvedValue(intake(requirement));
});

afterEach(() => {
  vi.clearAllMocks();
  shell.projectId = '';
});

test('Save sends the applied refinement, and saving clears acceptance', async () => {
  api.refineRequirement.mockResolvedValue({
    statement: 'Members export the board as CSV.',
    acceptance: ['The export has one row per card.'],
    notes: 'Made the check testable.',
    modelId: 'model-a'
  });
  api.updateRequirement.mockImplementation((_id: string, body: { statement: string; acceptance: string[] }) =>
    Promise.resolve(intake({ ...requirement, ...body, accepted: false }))
  );
  render(RequirementPage);
  const statement = (await screen.findByLabelText('Statement')) as HTMLTextAreaElement;
  await waitFor(() => expect(statement.value).toBe('Export the board.'));

  await fireEvent.click(screen.getByRole('button', { name: 'Refine with AI' }));
  expect(api.refineRequirement).toHaveBeenCalledWith({
    statement: 'Export the board.',
    acceptance: ['Works well.'],
    instructions: '',
    requirementId: 'REQ-005'
  });
  await screen.findByRole('region', { name: 'Refined requirement' });
  expect(api.updateRequirement).not.toHaveBeenCalled();

  await fireEvent.click(screen.getByRole('button', { name: 'Apply to form' }));
  expect(api.updateRequirement).not.toHaveBeenCalled();
  await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() =>
    expect(api.updateRequirement).toHaveBeenCalledWith('REQ-005', {
      statement: 'Members export the board as CSV.',
      acceptance: ['The export has one row per card.']
    })
  );
  await waitFor(() => expect(shell.message).toBe('Requirement saved. It needs acceptance again.'));
});

test('a PRD row has no refine button', async () => {
  api.getIntake.mockResolvedValue(intake({ ...requirement, sourceKind: 'prd', sourcePath: 'docs/prd/one.md' }));
  render(RequirementPage);
  expect(await screen.findByText('Edit this requirement in the repository.')).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Refine with AI' })).toBeNull();
});
