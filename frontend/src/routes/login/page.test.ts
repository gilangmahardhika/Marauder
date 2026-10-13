import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { goto } from '$app/navigation';
import { afterEach, expect, test, vi } from 'vitest';
import { session } from '$lib/session';

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
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
