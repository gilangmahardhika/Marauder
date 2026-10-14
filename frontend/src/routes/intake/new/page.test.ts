import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { ApiError } from '$lib/api';
import { shell } from '$lib/shell.svelte';
import { toast } from '$lib/toast.svelte';

const api = vi.hoisted(() => ({ getIntake: vi.fn(), addRequirement: vi.fn(), setReviewers: vi.fn() }));

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
  addRequirement: api.addRequirement,
  setReviewers: api.setReviewers,
  streamRefine: vi.fn()
}));

import NewRequirementPage from './+page.svelte';

beforeEach(() => {
  shell.projectId = 'proj_default';
  api.getIntake.mockResolvedValue({
    requirements: [],
    reviewerOptions: [],
    mcpServers: [
      { slug: 'docs', enabled: false, toolCount: 1 },
      { slug: 'tickets', enabled: true, toolCount: 2 }
    ]
  });
});

afterEach(() => {
  vi.clearAllMocks();
  shell.projectId = '';
  toast.clear();
});

test('typing @ lists the tribe servers from the intake', async () => {
  render(NewRequirementPage);
  await vi.waitFor(() => expect(api.getIntake).toHaveBeenCalled());
  await Promise.resolve();
  const statement = screen.getByLabelText('Statement') as HTMLTextAreaElement;
  statement.value = 'Use @';
  statement.setSelectionRange(5, 5);
  await fireEvent.input(statement);
  const options = within(await screen.findByRole('listbox', { name: 'MCP servers' })).getAllByRole('option');
  expect(options.map((option) => option.textContent?.replace(/\s+/g, ' ').trim())).toEqual(['@docs Disabled', '@tickets 2 tools']);
});

test('an unverified tribe member can be named as a reviewer', async () => {
  api.getIntake.mockResolvedValue({
    requirements: [],
    reviewerOptions: [{ id: 'usr_hendro', name: 'Hendro', email: 'hendro@example.com', githubLogin: '' }],
    mcpServers: []
  });
  api.addRequirement.mockResolvedValue({ requirements: [{ id: 'REQ-100' }] });
  api.setReviewers.mockResolvedValue({});
  render(NewRequirementPage);
  expect(screen.getByText('Someone without a verified GitHub login is asked to review on GitHub after they verify.')).toBeTruthy();
  const picker = screen.getByRole('combobox', { name: 'Reviewers' });
  await vi.waitFor(() => expect(api.getIntake).toHaveBeenCalled());
  await fireEvent.input(picker, { target: { value: 'hen' } });
  const option = await screen.findByRole('option', { name: /Hendro/ });
  expect(option.textContent).toContain('No verified GitHub login · hendro@example.com');
  await fireEvent.click(option);
  await fireEvent.input(screen.getByLabelText('Statement'), { target: { value: 'Export CSV' } });
  await fireEvent.submit(picker.closest('form')!);
  await vi.waitFor(() => expect(api.setReviewers).toHaveBeenCalledWith('REQ-100', ['usr_hendro']));
  await vi.waitFor(() => expect(toast.items.map((item) => [item.tone, item.text])).toEqual([['success', 'Requirement added']]));
});

test('an empty statement is marked and focused without a request', async () => {
  render(NewRequirementPage);
  const statement = screen.getByLabelText('Statement');
  await fireEvent.submit(statement.closest('form')!);
  await vi.waitFor(() => expect(statement.getAttribute('aria-invalid')).toBe('true'));
  expect(screen.getByText('Enter a statement.')).toBeTruthy();
  expect(document.activeElement).toBe(statement);
  expect(api.addRequirement).not.toHaveBeenCalled();
});

test('a refused add shows the id error on its field and other errors in the alert', async () => {
  api.addRequirement.mockRejectedValue(
    new ApiError(422, 'invalid', 'validation failed', { id: 'REQ-001 is already taken.', heading: 'Heading is too long.' })
  );
  render(NewRequirementPage);
  await fireEvent.input(screen.getByLabelText('Statement'), { target: { value: 'Export CSV' } });
  await fireEvent.submit(screen.getByLabelText('Statement').closest('form')!);
  expect(await screen.findByText('REQ-001 is already taken.')).toBeTruthy();
  const alert = screen.getByRole('alert');
  expect(alert.textContent).toContain('Check the fields marked below.');
  expect(alert.textContent).toContain('Heading is too long.');
  expect(screen.getByLabelText('Id').getAttribute('aria-invalid')).toBe('true');
});
