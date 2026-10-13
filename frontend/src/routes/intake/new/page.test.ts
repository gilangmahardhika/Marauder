import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';

const api = vi.hoisted(() => ({ getIntake: vi.fn() }));

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  getIntake: api.getIntake,
  addRequirement: vi.fn(),
  setReviewers: vi.fn(),
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
