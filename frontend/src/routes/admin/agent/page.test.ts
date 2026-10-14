import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { toast } from '$lib/toast.svelte';
import type { AgentSettings } from '$lib/types';

const api = vi.hoisted(() => ({
  ApiError: class ApiError extends Error {},
  getAgent: vi.fn(),
  putAgent: vi.fn(),
  testAgent: vi.fn(),
  getLLM: vi.fn(),
  getRouter: vi.fn()
}));

vi.mock('$lib/api', () => ({
  ApiError: api.ApiError,
  unreachable: () => 'API unreachable',
  getAgent: api.getAgent,
  putAgent: api.putAgent,
  testAgent: api.testAgent,
  getLLM: api.getLLM,
  getRouter: api.getRouter
}));

import AgentPage from './+page.svelte';

function agent(over: Partial<AgentSettings> = {}): AgentSettings {
  return {
    kind: 'builtin',
    claudeCode: { family: '', model: '', smallModel: '', maxTurns: 0, auth: 'auto', allowWeb: false, instructions: '', binary: '' },
    ...over
  };
}

beforeEach(() => {
  api.getAgent.mockResolvedValue(agent());
  api.getLLM.mockResolvedValue({
    families: [
      { id: 'deepseek', protocol: 'anthropic', baseUrl: 'https://api.deepseek.com/anthropic', models: ['deepseek-chat'], keySet: true },
      { id: 'nokey', protocol: 'anthropic', baseUrl: '', models: ['claude-a'], keySet: false },
      { id: 'openai-main', protocol: 'openai', baseUrl: '', models: ['gpt-x'], keySet: true }
    ]
  });
  api.getRouter.mockResolvedValue({ routes: {}, budgets: { maxAttempts: 3, maxModelTurnsPerAttempt: 30, maxTokensPerCard: 0, maxWallMinutesPerAttempt: 30 }, reviewIndependentPossible: false });
  api.putAgent.mockImplementation(async (body: AgentSettings) => body);
  api.testAgent.mockResolvedValue({ ok: true, version: '1.0.0 (Claude Code)' });
});

afterEach(() => {
  vi.clearAllMocks();
  toast.clear();
});

test('the Claude Code panel shows only for Claude Code', async () => {
  render(AgentPage);
  const group = await screen.findByRole('group', { name: 'Implement agent' });
  expect(screen.queryByLabelText('Model family')).toBeNull();
  await fireEvent.click(screen.getByRole('button', { name: 'Claude Code' }));
  expect(group).toBeTruthy();
  const family = screen.getByLabelText('Model family') as HTMLSelectElement;
  const options = [...family.options].map((option) => [option.value, option.disabled, option.textContent]);
  expect(options).toEqual([
    ['', false, 'Choose a family'],
    ['deepseek', false, 'deepseek'],
    ['nokey', true, 'nokey (No API key)']
  ]);
  expect((screen.getByLabelText(/Max turns/) as HTMLInputElement).placeholder).toBe('30');
});

test('saving Claude Code sends the options without a key', async () => {
  render(AgentPage);
  await fireEvent.click(await screen.findByRole('button', { name: 'Claude Code' }));
  await fireEvent.change(screen.getByLabelText('Model family'), { target: { value: 'deepseek' } });
  await fireEvent.change(screen.getByLabelText('Model'), { target: { value: 'deepseek-chat' } });
  await fireEvent.click(screen.getByLabelText(/Allow web fetch/));
  await fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(api.putAgent).toHaveBeenCalled());
  const body = api.putAgent.mock.calls[0][0] as AgentSettings;
  expect(body).toEqual({
    kind: 'claude_code',
    claudeCode: { family: 'deepseek', model: 'deepseek-chat', smallModel: '', maxTurns: 0, auth: 'auto', allowWeb: true, instructions: '', binary: '' }
  });
  expect(JSON.stringify(body)).not.toMatch(/apiKey"|key"/i);
  await waitFor(() => expect(toast.items.map((t) => t.text)).toEqual(['Implement jobs now use Claude Code.']));
});

test('a 422 puts each claudeCode field error under its control', async () => {
  api.getAgent.mockResolvedValue(agent({ kind: 'claude_code' }));
  api.putAgent.mockRejectedValue(
    Object.assign(new Error('validation failed'), {
      fields: { 'claudeCode.family': 'Claude Code needs a family with the anthropic protocol', 'claudeCode.binary': 'the claude binary must be an absolute path' }
    })
  );
  render(AgentPage);
  await fireEvent.click(await screen.findByRole('button', { name: 'Save' }));
  expect(await screen.findByText('Claude Code needs a family with the anthropic protocol')).toBeTruthy();
  expect(screen.getByText('the claude binary must be an absolute path')).toBeTruthy();
  expect(screen.getByText('Check the fields marked below.')).toBeTruthy();
});

test('Check shows the version, or the message in red', async () => {
  api.getAgent.mockResolvedValue(agent({ kind: 'claude_code' }));
  render(AgentPage);
  await fireEvent.click(await screen.findByRole('button', { name: 'Check' }));
  expect((await screen.findByRole('status')).textContent).toBe('1.0.0 (Claude Code)');
  api.testAgent.mockResolvedValue({ ok: false, message: 'claude not found on PATH' });
  await fireEvent.click(screen.getByRole('button', { name: 'Check' }));
  await waitFor(() => expect(screen.getByRole('status').textContent).toBe('claude not found on PATH'));
  expect(screen.getByRole('status').className).toContain('text-red-800');
});
