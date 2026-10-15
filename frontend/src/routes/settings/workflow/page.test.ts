import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';
import { toast } from '$lib/toast.svelte';
import type { WorkflowSettings } from '$lib/types';

const api = vi.hoisted(() => {
  class ApiError extends Error {
    fields: Record<string, string>;
    constructor(message: string, fields: Record<string, string> = {}) {
      super(message);
      this.fields = fields;
    }
  }
  return { ApiError, getWorkflow: vi.fn(), putWorkflow: vi.fn() };
});

vi.mock('$lib/api', () => ({
  ApiError: api.ApiError,
  unreachable: () => 'API unreachable',
  getWorkflow: api.getWorkflow,
  putWorkflow: api.putWorkflow
}));

import WorkflowPage from './+page.svelte';

const saved: WorkflowSettings = { implementationCap: 2, soloApproval: false, canEdit: true };

beforeEach(() => {
  shell.projectId = 'proj_default';
  api.getWorkflow.mockResolvedValue(saved);
});

afterEach(() => {
  vi.clearAllMocks();
  toast.clear();
  shell.projectId = '';
});

test('the cap and Solo approval load and save together', async () => {
  api.putWorkflow.mockImplementation((body: Record<string, unknown>) => Promise.resolve({ ...saved, ...body }));
  render(WorkflowPage);
  const cap = within(await screen.findByRole('group', { name: 'Implementation cap' }));
  const approval = within(screen.getByRole('group', { name: 'Solo approval' }));
  await waitFor(() => expect(cap.getByRole('button', { name: '2' }).getAttribute('aria-pressed')).toBe('true'));
  expect(approval.getByRole('button', { name: 'Second reviewer' }).getAttribute('aria-pressed')).toBe('true');
  expect(screen.getByText(/Up to 2 cards run at once/)).toBeTruthy();

  await fireEvent.click(cap.getByRole('button', { name: '4' }));
  await fireEvent.click(approval.getByRole('button', { name: 'Solo' }));
  expect(screen.getByText(/Up to 4 cards run at once/)).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: /save/i }));
  await waitFor(() => expect(api.putWorkflow).toHaveBeenCalledTimes(1));
  expect(api.putWorkflow.mock.calls[0][0]).toEqual({ implementationCap: 4, soloApproval: true });
  await waitFor(() => expect(toast.items.map((item) => item.text)).toEqual(['Workflow settings saved']));
});

test('a refused cap shows under its field', async () => {
  api.putWorkflow.mockRejectedValue(new api.ApiError('validation failed', { implementationCap: 'implementation cap is from 1 to 8' }));
  render(WorkflowPage);
  const cap = await screen.findByRole('group', { name: 'Implementation cap' });
  await fireEvent.click(screen.getByRole('button', { name: /save/i }));
  expect(await screen.findByText('implementation cap is from 1 to 8')).toBeTruthy();
  expect(cap.parentElement?.getAttribute('aria-invalid')).toBe('true');
});

test('a member who cannot edit sees the fields locked', async () => {
  api.getWorkflow.mockResolvedValue({ ...saved, canEdit: false });
  render(WorkflowPage);
  expect(await screen.findByText(/Only a Lead Developer, a tribe admin, or an admin/)).toBeTruthy();
  expect(screen.getByRole('group', { name: 'Implementation cap' }).closest('fieldset')?.disabled).toBe(true);
});
