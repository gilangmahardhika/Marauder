import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';
import { toast } from '$lib/toast.svelte';
import type { ConventionCatalog, ProjectConventions } from '$lib/types';

const api = vi.hoisted(() => ({
  getConventions: vi.fn(),
  getProjectConventions: vi.fn(),
  putProjectConventions: vi.fn()
}));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: {}, url: new URL('http://127.0.0.1:4173/settings/conventions') }).subscribe(run)
  }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {
    fields: Record<string, string> = {};
  },
  unreachable: () => 'API unreachable',
  ...api
}));

import ProjectConventionsPage from './+page.svelte';

const at = '2026-10-13T00:00:00Z';
const catalog: ConventionCatalog = {
  stacks: [
    { id: 'stk_go', name: 'Go', detect: { files: [], npmDeps: [], pyDeps: [] }, version: 1, contentHash: 'h', disabled: false, updatedBy: null, updatedAt: at },
    { id: 'stk_react', name: 'React', detect: { files: [], npmDeps: [], pyDeps: [] }, version: 1, contentHash: 'h', disabled: false, updatedBy: null, updatedAt: at }
  ],
  skills: [{ id: 'skl_oauth2', slug: 'oauth2-integration', title: 'OAuth 2.0', useWhen: 'Sign-in', body: 'b', version: 1, contentHash: 'h', disabled: false, updatedBy: null, updatedAt: at }],
  rules: [
    { id: 'rule_naming', scope: 'global', stackId: '', category: 'naming', title: 'Naming', body: 'b', version: 1, contentHash: 'h', disabled: false, updatedBy: null, updatedAt: at },
    { id: 'rule_go', scope: 'stack', stackId: 'stk_go', category: 'style', title: 'Go errors', body: 'b', version: 1, contentHash: 'h', disabled: false, updatedBy: null, updatedAt: at }
  ]
};

const view: ProjectConventions = {
  tribe: { stackIds: [], skillIds: ['skl_oauth2'], rulesOff: ['rule_naming'] },
  choices: { stacks: { on: [], off: [] }, skills: { on: [], off: [] }, rules: { on: [], off: [] } },
  detectedStackIds: ['stk_go'],
  stacks: [{ id: 'stk_go', name: 'Go', origin: 'detected' }],
  skills: [{ id: 'skl_oauth2', slug: 'oauth2-integration', title: 'OAuth 2.0', useWhen: 'Sign-in', version: 1, origin: 'tribe' }],
  rules: [{ id: 'rule_go', title: 'Go errors', scope: 'stack', stackName: 'Go', category: 'style', version: 1, origin: 'catalog' }],
  canEdit: true
};

function row(name: string) {
  return screen.getByText(name, { selector: 'span.font-medium' }).closest('tr') as HTMLElement;
}

beforeEach(() => {
  shell.projectId = 'proj_a';
  api.getConventions.mockResolvedValue(catalog);
});

afterEach(() => {
  vi.clearAllMocks();
  toast.clear();
  shell.projectId = '';
});

test('each item shows where it comes from', async () => {
  api.getProjectConventions.mockResolvedValue(view);
  render(ProjectConventionsPage);
  await waitFor(() => expect(screen.getByText('Go', { selector: 'span.font-medium' })).toBeTruthy());
  expect(within(row('Go')).getByText('Detected')).toBeTruthy();
  expect(within(row('OAuth 2.0')).getByText('Tribe')).toBeTruthy();
  expect(within(row('Naming')).getByText('Off at tribe')).toBeTruthy();
  expect(within(row('Go errors')).getByText('Catalog')).toBeTruthy();
  expect(within(row('Naming')).getByRole('button', { name: 'Inherit (off)' }).getAttribute('aria-pressed')).toBe('true');
});

test('the project switches items on and off and saves its choices', async () => {
  api.getProjectConventions.mockResolvedValue(view);
  api.putProjectConventions.mockImplementation(async (choices: ProjectConventions['choices']) => ({ ...view, choices }));
  render(ProjectConventionsPage);
  await waitFor(() => expect(screen.getByText('Naming', { selector: 'span.font-medium' })).toBeTruthy());
  await fireEvent.click(within(row('Naming')).getByRole('button', { name: 'On' }));
  await fireEvent.click(within(row('OAuth 2.0')).getByRole('button', { name: 'Off' }));
  expect(within(row('OAuth 2.0')).getByText('Project')).toBeTruthy();
  await fireEvent.click(within(row('Go')).getByRole('button', { name: 'Off' }));
  expect(screen.queryByText('Go errors', { selector: 'span.font-medium' })).toBeNull();
  await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(api.putProjectConventions).toHaveBeenCalled());
  expect(api.putProjectConventions).toHaveBeenCalledWith({
    stacks: { on: [], off: ['stk_go'] },
    skills: { on: [], off: ['skl_oauth2'] },
    rules: { on: ['rule_naming'], off: [] }
  });
  await waitFor(() => expect(toast.items.map((item) => item.text)).toEqual(['Conventions saved']));
});

test('a refused list shows its error on the add picker', async () => {
  api.getProjectConventions.mockResolvedValue(view);
  api.putProjectConventions.mockRejectedValue(
    Object.assign(new Error('validation failed'), { fields: { 'skills.on': 'not enabled: skl_gone' } })
  );
  render(ProjectConventionsPage);
  await waitFor(() => expect(screen.getByText('Go', { selector: 'span.font-medium' })).toBeTruthy());
  await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(await screen.findByText('not enabled: skl_gone')).toBeTruthy();
  expect(document.getElementById('add-skills')?.getAttribute('aria-invalid')).toBe('true');
  expect(screen.getByRole('alert').textContent).toContain('Check the fields marked below.');
});

test('without canEdit the controls are disabled and there are no add pickers', async () => {
  api.getProjectConventions.mockResolvedValue({ ...view, canEdit: false });
  render(ProjectConventionsPage);
  await waitFor(() => expect(screen.getByText('Go', { selector: 'span.font-medium' })).toBeTruthy());
  expect((within(row('Go')).getByRole('button', { name: 'Off' }) as HTMLButtonElement).disabled).toBe(true);
  expect(document.getElementById('add-stacks')).toBeNull();
  expect((screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement).disabled).toBe(true);
});
