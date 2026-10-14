import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';
import type { DeploySettings } from '$lib/types';

const api = vi.hoisted(() => {
  class ApiError extends Error {
    fields: Record<string, string>;
    constructor(message: string, fields: Record<string, string> = {}) {
      super(message);
      this.fields = fields;
    }
  }
  return { ApiError, getDeploy: vi.fn(), putDeploy: vi.fn() };
});

vi.mock('$lib/api', () => ({
  ApiError: api.ApiError,
  unreachable: () => 'API unreachable',
  getDeploy: api.getDeploy,
  putDeploy: api.putDeploy
}));

import DeployPage from './+page.svelte';

const saved: DeploySettings = {
  environment: 'dev',
  devUrl: '',
  devHosts: [],
  devCommand: '',
  rollbackCommand: '',
  healthUrl: '',
  deployTokenSet: false,
  canEdit: true,
  localCheck: 'go test ./...',
  devCheck: '',
  allowExecutables: ['go'],
  commandExecutables: ['go', 'gofmt'],
  sandboxEngine: 'podman',
  sandboxImage: '',
  implementationCap: 2,
  prdPath: 'docs/prd',
  rfcPath: 'docs/rfc',
  soloApproval: false,
  deployEnabled: true
};

beforeEach(() => {
  shell.projectId = 'proj_default';
  api.getDeploy.mockResolvedValue(saved);
});

afterEach(() => {
  vi.clearAllMocks();
  shell.projectId = '';
});

test('allowed commands load one per line and save as a list', async () => {
  api.putDeploy.mockImplementation((body: Record<string, unknown>) => Promise.resolve({ ...saved, ...body }));
  render(DeployPage);
  const field = screen.getByLabelText('Allowed commands') as HTMLTextAreaElement;
  await waitFor(() => expect(field.value).toBe('go\ngofmt'));
  expect(screen.getByText(/Leave empty to turn run_command off/)).toBeTruthy();

  await fireEvent.input(field, { target: { value: 'go\n  npx \n\n' } });
  await fireEvent.click(screen.getByRole('button', { name: /save/i }));
  await waitFor(() => expect(api.putDeploy).toHaveBeenCalledTimes(1));
  const body = api.putDeploy.mock.calls[0][0] as Record<string, unknown>;
  expect(body.commandExecutables).toEqual(['go', 'npx']);
  expect(body.allowExecutables).toEqual(['go']);
});

test('the deploy switch loads from and saves deployEnabled', async () => {
  api.putDeploy.mockImplementation((body: Record<string, unknown>) => Promise.resolve({ ...saved, ...body }));
  render(DeployPage);
  const box = screen.getByRole('checkbox', { name: /Deploy approved cards/ }) as HTMLInputElement;
  await waitFor(() => expect(box.checked).toBe(true));
  expect(screen.getByText(/an approved card gets Squash and merge instead of Deploy/)).toBeTruthy();

  await fireEvent.click(box);
  await fireEvent.click(screen.getByRole('button', { name: /save/i }));
  await waitFor(() => expect(api.putDeploy).toHaveBeenCalledTimes(1));
  expect((api.putDeploy.mock.calls[0][0] as Record<string, unknown>).deployEnabled).toBe(false);
});

test('a refused command shows the field error', async () => {
  api.putDeploy.mockRejectedValue(
    new api.ApiError('validation failed', { commandExecutables: 'bash is a shell or runs another program, so it cannot be an allowed command' })
  );
  render(DeployPage);
  const field = screen.getByLabelText('Allowed commands') as HTMLTextAreaElement;
  await waitFor(() => expect(field.value).toBe('go\ngofmt'));
  await fireEvent.input(field, { target: { value: 'bash' } });
  await fireEvent.click(screen.getByRole('button', { name: /save/i }));
  expect(await screen.findByText(/bash is a shell or runs another program/)).toBeTruthy();
});
