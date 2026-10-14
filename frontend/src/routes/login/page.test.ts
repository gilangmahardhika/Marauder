import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { goto } from '$app/navigation';
import { afterEach, expect, test, vi } from 'vitest';
import { session } from '$lib/session';

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {
    status: number;
    constructor(status: number, _code: string, message: string) {
      super(message);
      this.status = status;
    }
  },
  openSession: vi.fn().mockResolvedValue({ bearer: 'bearer-1' })
}));

import LoginPage from './+page.svelte';

afterEach(() => sessionStorage.clear());

test('signing in stores the bearer and opens the tribe selector', async () => {
  render(LoginPage);
  await fireEvent.input(screen.getByLabelText('Email'), { target: { value: 'ana@example.com' } });
  await fireEvent.input(screen.getByLabelText('Password'), { target: { value: 'password-long-1' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
  await waitFor(() => expect(goto).toHaveBeenCalledWith('/choose-tribe'));
  expect(session.bearer()).toBe('bearer-1');
});

test('an empty submit names the missing fields and focuses email without a request', async () => {
  const { openSession } = await import('$lib/api');
  vi.mocked(openSession).mockClear();
  render(LoginPage);
  await fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
  const emailInput = screen.getByLabelText('Email');
  await waitFor(() => expect(document.activeElement).toBe(emailInput));
  expect(emailInput.getAttribute('aria-invalid')).toBe('true');
  expect(screen.getByText('Enter your email')).toBeTruthy();
  expect(screen.getByText('Enter your password')).toBeTruthy();
  expect(openSession).not.toHaveBeenCalled();
});

test('a wrong password shows the alert, clears the password, and focuses the alert', async () => {
  const api = await import('$lib/api');
  vi.mocked(api.openSession).mockRejectedValueOnce(new api.ApiError(401, 'unauthenticated', 'email or password'));
  render(LoginPage);
  await fireEvent.input(screen.getByLabelText('Email'), { target: { value: 'ana@example.com' } });
  await fireEvent.input(screen.getByLabelText('Password'), { target: { value: 'wrong-password-1' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
  const alert = await screen.findByRole('alert');
  expect(alert.textContent).toContain('Email or password is wrong.');
  await waitFor(() => expect(document.activeElement).toBe(alert));
  expect((screen.getByLabelText('Password') as HTMLInputElement).value).toBe('');
  expect(screen.getByLabelText('Email').hasAttribute('aria-invalid')).toBe(false);
});

test('too many attempts says to wait', async () => {
  const api = await import('$lib/api');
  vi.mocked(api.openSession).mockRejectedValueOnce(new api.ApiError(429, 'rate_limited', 'locked'));
  render(LoginPage);
  await fireEvent.input(screen.getByLabelText('Email'), { target: { value: 'ana@example.com' } });
  await fireEvent.input(screen.getByLabelText('Password'), { target: { value: 'password-long-1' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
  expect((await screen.findByRole('alert')).textContent).toContain('Too many attempts. Wait one minute.');
});
