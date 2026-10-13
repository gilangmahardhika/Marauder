import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import type { ConventionCatalog } from '$lib/types';

const api = vi.hoisted(() => ({
  getConventions: vi.fn(),
  getTribeConventions: vi.fn(),
  putTribeConventions: vi.fn(),
  getTribes: vi.fn()
}));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: { id: 'tribe_a' }, url: new URL('http://127.0.0.1:4173/tribes/tribe_a/conventions') }).subscribe(run)
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

import TribeConventionsPage from './+page.svelte';

const at = '2026-10-13T00:00:00Z';
const catalog: ConventionCatalog = {
  stacks: [{ id: 'stk_go', name: 'Go', detect: { files: [], npmDeps: [], pyDeps: [] }, version: 1, contentHash: 'h', disabled: false, updatedBy: null, updatedAt: at }],
  skills: [{ id: 'skl_oauth2', slug: 'oauth2-integration', title: 'OAuth 2.0', useWhen: 'Sign-in', body: 'b', version: 1, contentHash: 'h', disabled: false, updatedBy: null, updatedAt: at }],
  rules: [
    { id: 'rule_naming', scope: 'global', stackId: '', category: 'naming', title: 'Naming', body: 'b', version: 1, contentHash: 'h', disabled: false, updatedBy: null, updatedAt: at },
    { id: 'rule_go', scope: 'stack', stackId: 'stk_go', category: 'style', title: 'Go errors', body: 'b', version: 1, contentHash: 'h', disabled: false, updatedBy: null, updatedAt: at }
  ]
};

beforeEach(() => {
  api.getConventions.mockResolvedValue(catalog);
  api.getTribes.mockResolvedValue({ tribes: [{ id: 'tribe_a', name: 'Alpha' }] });
});

afterEach(() => {
  vi.clearAllMocks();
});

test('a member reads the tribe conventions without controls', async () => {
  api.getTribeConventions.mockResolvedValue({ stackIds: ['stk_go'], skillIds: [], rulesOff: ['rule_naming'], canEdit: false });
  render(TribeConventionsPage);
  await waitFor(() => expect(screen.getByText('Only a tribe admin or an admin can change the tribe\'s conventions.')).toBeTruthy());
  expect(screen.queryByRole('button', { name: 'Save' })).toBeNull();
  const naming = screen.getByRole('group', { name: 'Naming for this tribe' });
  expect(naming.querySelector('[aria-pressed="true"]')?.textContent?.trim()).toBe('Off');
  for (const button of naming.querySelectorAll('button')) expect((button as HTMLButtonElement).disabled).toBe(true);
});

test('a tribe admin switches a rule off and saves the tribe layer', async () => {
  api.getTribeConventions.mockResolvedValue({ stackIds: ['stk_go'], skillIds: ['skl_oauth2'], rulesOff: [], canEdit: true });
  api.putTribeConventions.mockImplementation(async (_id: string, body: object) => ({ ...body, canEdit: true }));
  render(TribeConventionsPage);
  const group = await screen.findByRole('group', { name: 'Go errors for this tribe' });
  await fireEvent.click(group.querySelector('button:nth-child(2)') as HTMLElement);
  await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(api.putTribeConventions).toHaveBeenCalled());
  expect(api.putTribeConventions).toHaveBeenCalledWith('tribe_a', { stackIds: ['stk_go'], skillIds: ['skl_oauth2'], rulesOff: ['rule_go'] });
});
