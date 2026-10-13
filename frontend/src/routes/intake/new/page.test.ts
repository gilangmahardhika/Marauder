import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';

const api = vi.hoisted(() => ({ getIntake: vi.fn(), addRequirement: vi.fn(), setReviewers: vi.fn() }));

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
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
});
