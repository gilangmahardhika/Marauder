import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { toast } from '$lib/toast.svelte';
import type { Card, CardContext as Context } from '$lib/types';

const api = vi.hoisted(() => ({ getConventions: vi.fn(), setCardContext: vi.fn() }));

vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {
    fields: Record<string, string> = {};
  },
  unreachable: () => 'API unreachable',
  ...api
}));

import CardContext from './CardContext.svelte';

const at = '2026-10-13T00:00:00Z';

const context: Context = {
  rules: [
    { id: 'rule_naming', title: 'Naming', scope: 'global', stackName: '', category: 'naming', version: 1, origin: 'project', state: 'on' },
    { id: 'rule_db', title: 'Tables', scope: 'global', stackName: '', category: 'database', version: 1, origin: 'card', state: 'off' }
  ],
  skills: [{ id: 'skl_oauth2', slug: 'oauth2-integration', title: 'OAuth 2.0', useWhen: 'Sign-in', version: 1, origin: 'planner', state: 'on' }],
  mcp: [
    { id: 'mcp_tickets', slug: 'tickets', enabled: true, toolCount: 2, state: 'on', origin: 'scope' },
    { id: 'mcp_wiki', slug: 'wiki', enabled: true, toolCount: 1, state: 'off', origin: 'scope' }
  ],
  mcpScope: 'mentioned'
};

function card(actions: string[], next: Context = context): Card {
  return {
    id: 'card_1',
    title: 'Sign in',
    status: 'ready',
    group: 'ready',
    actions,
    unblockTo: null,
    owner: null,
    reviewers: [],
    skills: [],
    requirementIds: [],
    decisionIds: [],
    dependsOn: [],
    queued: false,
    waitingOn: [],
    risk: 'low',
    service: 'default',
    manual: false,
    localCheck: null,
    devCheck: null,
    attemptsUsed: 0,
    maxAttempts: 3,
    branch: null,
    prUrl: null,
    headSha: null,
    reviewedSha: null,
    reviewIndependent: null,
    reviewVerdict: null,
    selfReview: false,
    aiReviewMode: 'continue',
    approvedSha: null,
    deployedSha: null,
    environment: 'dev',
    failureReason: null,
    blockedReason: null,
    updatedAt: at,
    context: next
  };
}

function rowOf(text: string) {
  return screen.getAllByText(text)[0].closest('li') as HTMLElement;
}

beforeEach(() => {
  api.getConventions.mockResolvedValue({
    stacks: [],
    rules: [],
    skills: [
      { id: 'skl_oauth2', slug: 'oauth2-integration', title: 'OAuth 2.0', useWhen: 'Sign-in', body: 'b', version: 1, contentHash: 'h', disabled: false, updatedBy: null, updatedAt: at },
      { id: 'skl_pay', slug: 'payments', title: 'Payments', useWhen: 'Charging', body: 'b', version: 1, contentHash: 'h', disabled: false, updatedBy: null, updatedAt: at }
    ]
  });
});

afterEach(() => {
  vi.clearAllMocks();
  toast.clear();
});

test('without the context action the panel only lists the context', () => {
  render(CardContext, { card: card(['run']) });
  expect(screen.getByRole('heading', { name: 'Context' })).toBeTruthy();
  expect(screen.getByText('Changes apply from the next attempt.', { exact: false })).toBeTruthy();
  expect(within(rowOf('Naming')).getByText('Project')).toBeTruthy();
  expect(within(rowOf('OAuth 2.0')).getByText('Planner')).toBeTruthy();
  expect(screen.queryByRole('button', { name: /Detach/ })).toBeNull();
  expect(document.getElementById('attach-skill')).toBeNull();
  expect(api.getConventions).not.toHaveBeenCalled();
});

test('detach and restore call the API and replace the card', async () => {
  const after: Context = { ...context, rules: [{ ...context.rules[0], origin: 'card', state: 'off' }, context.rules[1]] };
  api.setCardContext.mockResolvedValueOnce(card(['context'], after));
  api.setCardContext.mockResolvedValueOnce(card(['context'], context));
  render(CardContext, { card: card(['context']) });
  await fireEvent.click(within(rowOf('Naming')).getByRole('button', { name: /Detach\s*Naming/ }));
  expect(api.setCardContext).toHaveBeenCalledWith('card_1', { kind: 'rule', itemId: 'rule_naming', mode: 'detach' });
  await waitFor(() => expect(within(rowOf('Naming')).getByText('Detached')).toBeTruthy());
  expect(toast.items.map((item) => `${item.tone}: ${item.text}`)).toEqual(['success: Card context saved']);
  await fireEvent.click(within(rowOf('Tables')).getByRole('button', { name: /Restore\s*Tables/ }));
  expect(api.setCardContext).toHaveBeenLastCalledWith('card_1', { kind: 'rule', itemId: 'rule_db', mode: 'inherit' });
});

test('a failed detach with no field errors shows an error toast', async () => {
  api.setCardContext.mockRejectedValueOnce(new Error('The card is running.'));
  render(CardContext, { card: card(['context']) });
  await fireEvent.click(within(rowOf('Naming')).getByRole('button', { name: /Detach\s*Naming/ }));
  await waitFor(() => expect(toast.items.map((item) => `${item.tone}: ${item.text}`)).toEqual(['error: The card is running.']));
  expect(screen.queryByRole('alert')).toBeNull();
});

test('an attach field error shows on that attach control and in the panel alert', async () => {
  api.setCardContext.mockRejectedValueOnce(
    Object.assign(new Error('validation'), { fields: { itemId: 'That skill is disabled.', mode: 'Pick attach or detach.' } })
  );
  render(CardContext, { card: card(['context']) });
  await waitFor(() => expect(api.getConventions).toHaveBeenCalled());
  await fireEvent.click(document.getElementById('attach-skill') as HTMLElement);
  await fireEvent.click((await screen.findAllByRole('option'))[0]);
  expect(await screen.findByText('That skill is disabled.')).toBeTruthy();
  expect(document.getElementById('attach-skill')?.getAttribute('aria-invalid')).toBe('true');
  expect(document.getElementById('attach-rule')?.getAttribute('aria-invalid')).toBeNull();
  const alert = screen.getByRole('alert');
  expect(alert.textContent).toContain('Check the fields marked below.');
  expect(alert.textContent).toContain('Pick attach or detach.');
  expect(toast.items).toEqual([]);
});

test('attach skill lists enabled skills the card lacks, and attach server needs attach-mcp', async () => {
  api.setCardContext.mockResolvedValue(card(['context']));
  render(CardContext, { card: card(['context']) });
  expect(document.getElementById('attach-mcp')).toBeNull();
  expect(within(rowOf('tickets')).getByRole('button', { name: /Detach\s*tickets/ })).toBeTruthy();
  await waitFor(() => expect(api.getConventions).toHaveBeenCalled());
  await fireEvent.click(document.getElementById('attach-skill') as HTMLElement);
  const options = await screen.findAllByRole('option');
  expect(options.map((option) => option.textContent?.trim() ?? '')).toEqual([expect.stringContaining('Payments')]);
  await fireEvent.click(options[0]);
  expect(api.setCardContext).toHaveBeenCalledWith('card_1', { kind: 'skill', itemId: 'skl_pay', mode: 'attach' });
});

test('a lead attaches a server that is off', async () => {
  api.setCardContext.mockResolvedValue(card(['context', 'attach-mcp']));
  render(CardContext, { card: card(['context', 'attach-mcp']) });
  await fireEvent.click(document.getElementById('attach-mcp') as HTMLElement);
  const options = await screen.findAllByRole('option');
  expect(options.map((option) => option.textContent?.trim() ?? '')).toEqual([expect.stringContaining('wiki')]);
  await fireEvent.click(options[0]);
  expect(api.setCardContext).toHaveBeenCalledWith('card_1', { kind: 'mcp', itemId: 'mcp_wiki', mode: 'attach' });
});
