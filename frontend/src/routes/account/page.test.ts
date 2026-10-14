import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, expect, test, vi } from 'vitest';
import { toast } from '$lib/toast.svelte';

const api = vi.hoisted(() => {
  class ApiError extends Error {
    status: number;
    fields: Record<string, string>;
    constructor(status: number, message: string, fields: Record<string, string> = {}) {
      super(message);
      this.status = status;
      this.fields = fields;
    }
  }
  return {
    ApiError,
    unreachable: () => 'unreachable',
    getMe: vi.fn(),
    getMyGitHub: vi.fn(),
    putMe: vi.fn(),
    putMyGitHub: vi.fn(),
    testMyGitHub: vi.fn(),
    changePassword: vi.fn(),
    testPush: vi.fn()
  };
});

vi.mock('$lib/api', () => api);
vi.mock('$lib/push', () => ({
  syncPush: vi.fn().mockResolvedValue('unsupported'),
  enablePush: vi.fn(),
  disablePush: vi.fn()
}));

import AccountPage from './+page.svelte';

afterEach(() => toast.clear());

function signedIn() {
  api.getMe.mockResolvedValue({ email: 'ada@example.com', name: 'Ada', role: 'developer', admin: false });
  api.getMyGitHub.mockResolvedValue({ host: 'github.com', login: '', tokenSet: false, verified: false });
}

test('a wrong current password is a field error and focuses that field', async () => {
  signedIn();
  api.changePassword.mockRejectedValueOnce(
    new api.ApiError(422, 'current password', { currentPassword: 'Current password is wrong.' })
  );
  render(AccountPage);
  const current = await screen.findByLabelText('Current password');
  await fireEvent.input(current, { target: { value: 'not-the-password' } });
  await fireEvent.input(screen.getByLabelText('New password'), { target: { value: 'password-long-2' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Change password' }));
  expect(await screen.findByText('Current password is wrong.')).toBeTruthy();
  await waitFor(() => expect(document.activeElement).toBe(current));
  expect(current.getAttribute('aria-invalid')).toBe('true');
});

test('saving the name shows a success toast', async () => {
  signedIn();
  api.putMe.mockResolvedValueOnce({ email: 'ada@example.com', name: 'Ada Lovelace', role: 'developer', admin: false });
  render(AccountPage);
  await fireEvent.input(await screen.findByLabelText('Name'), { target: { value: 'Ada Lovelace' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Save name' }));
  await waitFor(() => expect(toast.items.map((item) => item.text)).toContain('Name saved'));
});
