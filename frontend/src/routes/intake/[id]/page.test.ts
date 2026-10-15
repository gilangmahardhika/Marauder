import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { ApiError } from '$lib/api';
import { shell } from '$lib/shell.svelte';
import { toast } from '$lib/toast.svelte';
import type { Intake, RefineEvent, Requirement } from '$lib/types';

const api = vi.hoisted(() => ({
  getIntake: vi.fn(),
  updateRequirement: vi.fn(),
  streamRefine: vi.fn(),
  deleteRequirement: vi.fn(),
  getLLM: vi.fn(),
  getRouter: vi.fn()
}));

vi.mock('$app/stores', () => ({
  page: { subscribe: (run: (value: unknown) => void) => readable({ params: { id: 'REQ-005' } }).subscribe(run) }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {
    constructor(
      public status: number,
      public code: string,
      message: string,
      public fields: Record<string, string> = {}
    ) {
      super(message);
    }
  },
  getIntake: api.getIntake,
  updateRequirement: api.updateRequirement,
  streamRefine: api.streamRefine,
  deleteRequirement: api.deleteRequirement,
  getLLM: api.getLLM,
  getRouter: api.getRouter,
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
  api.getLLM.mockResolvedValue({
    families: [{ id: 'anthropic', protocol: 'anthropic', baseUrl: '', models: ['claude-a', 'claude-b'], keySet: true }]
  });
  api.getRouter.mockResolvedValue({ routes: { implement: { model: 'claude-a', fallback: '' } } });
});

afterEach(() => {
  vi.clearAllMocks();
  shell.projectId = '';
  toast.clear();
});

test('refining acceptance, Apply, and Save send the new acceptance with the statement unchanged', async () => {
  api.streamRefine.mockImplementation(async (_body: unknown, onEvent: (event: RefineEvent) => void) => {
    onEvent({ kind: 'start', modelId: 'model-a' });
    onEvent({ kind: 'delta', text: 'The export has ' });
    onEvent({ kind: 'delta', text: 'one row per card.' });
    onEvent({ kind: 'done', text: 'The export has one row per card.', acceptance: ['The export has one row per card.'], modelId: 'model-a' });
  });
  api.updateRequirement.mockImplementation((_id: string, body: { statement: string; acceptance: string[] }) =>
    Promise.resolve(intake({ ...requirement, ...body, accepted: false }))
  );
  render(RequirementPage);
  const statement = (await screen.findByLabelText('Statement')) as HTMLTextAreaElement;
  await waitFor(() => expect(statement.value).toBe('Export the board.'));

  await fireEvent.click(screen.getByRole('button', { name: 'Refine acceptance with AI' }));
  await waitFor(() => expect(screen.getByRole('combobox', { name: 'Model' }).textContent).toContain('claude-a'));
  await fireEvent.click(screen.getByRole('button', { name: 'Refine' }));
  expect(api.streamRefine).toHaveBeenCalledWith(
    {
      field: 'acceptance',
      statement: 'Export the board.',
      acceptance: ['Works well.'],
      instructions: '',
      model: 'claude-a',
      thinking: '',
      effort: '',
      requirementId: 'REQ-005'
    },
    expect.any(Function),
    expect.any(AbortSignal)
  );
  await screen.findByRole('region', { name: 'Refined acceptance' });
  expect(api.updateRequirement).not.toHaveBeenCalled();

  await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
  expect(api.updateRequirement).not.toHaveBeenCalled();
  await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() =>
    expect(api.updateRequirement).toHaveBeenCalledWith('REQ-005', {
      statement: 'Export the board.',
      acceptance: ['The export has one row per card.']
    })
  );
  await waitFor(() =>
    expect(toast.items.map((item) => [item.tone, item.text])).toEqual([['success', 'Requirement saved. It needs acceptance again.']])
  );
});

test('a rejected save shows the field error under the statement and keeps the text', async () => {
  api.updateRequirement.mockRejectedValue(
    new ApiError(422, 'invalid', 'validation failed', { statement: 'Keep the statement to 4000 characters or fewer.' })
  );
  render(RequirementPage);
  const statement = (await screen.findByLabelText('Statement')) as HTMLTextAreaElement;
  await waitFor(() => expect(statement.value).toBe('Export the board.'));
  await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(statement.getAttribute('aria-invalid')).toBe('true'));
  expect(screen.getByText('Keep the statement to 4000 characters or fewer.')).toBeTruthy();
  expect(screen.getByRole('alert').textContent).toContain('Check the fields marked below.');
  expect(statement.value).toBe('Export the board.');
  expect(toast.items).toEqual([]);
});

test('a failed load shows the error with Try again', async () => {
  api.getIntake.mockRejectedValueOnce(new Error('intake read failed'));
  render(RequirementPage);
  expect((await screen.findByRole('alert')).textContent).toContain('intake read failed');
  await fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
  expect(await screen.findByLabelText('Statement')).toBeTruthy();
});

test('a PRD row has no refine button', async () => {
  api.getIntake.mockResolvedValue(intake({ ...requirement, sourceKind: 'prd', sourcePath: 'docs/prd/one.md' }));
  render(RequirementPage);
  expect(await screen.findByText('Edit this requirement in the repository.')).toBeTruthy();
  expect(screen.queryByRole('button', { name: /with AI/ })).toBeNull();
});

test('a PRD row shows its stored mentions with their state', async () => {
  api.getIntake.mockResolvedValue({
    ...intake({ ...requirement, sourceKind: 'prd', sourcePath: 'docs/prd/one.md', mentions: ['tickets', 'docs', 'nothere'] }),
    mcpServers: [
      { slug: 'tickets', enabled: true, toolCount: 2 },
      { slug: 'docs', enabled: false, toolCount: 1 }
    ]
  });
  render(RequirementPage);
  const list = await screen.findByRole('list', { name: 'MCP servers mentioned' });
  expect(list.textContent).toContain('@tickets');
  expect(list.textContent).toContain('@docs');
  expect(screen.getByText('disabled: jobs will not call it')).toBeTruthy();
  expect(screen.getByText('no server with this slug')).toBeTruthy();
});
