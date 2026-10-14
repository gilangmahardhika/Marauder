import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { toast } from '$lib/toast.svelte';

const api = vi.hoisted(() => ({
  ApiError: class ApiError extends Error {},
  testPush: vi.fn()
}));
const push = vi.hoisted(() => ({ syncPush: vi.fn(), enablePush: vi.fn(), disablePush: vi.fn() }));

vi.mock('$lib/api', () => api);
vi.mock('$lib/push', () => push);

import NotificationsPanel from './NotificationsPanel.svelte';

const toasts = () => toast.items.map((item) => [item.tone, item.text]);

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(() => {
  toast.clear();
});

test('turns notifications on', async () => {
  push.syncPush.mockResolvedValue('off');
  push.enablePush.mockResolvedValue('on');
  render(NotificationsPanel);
  expect(await screen.findByText('Off')).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Turn on' }));
  expect(await screen.findByText('On')).toBeTruthy();
  expect(push.enablePush).toHaveBeenCalledOnce();
  expect(toasts()).toEqual([['success', 'Notifications are on for this browser.']]);
  expect(screen.getByRole('button', { name: 'Send a test' })).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Turn on' })).toBeNull();
});

test('Turn on shows a spinner while the browser asks', async () => {
  push.syncPush.mockResolvedValue('off');
  push.enablePush.mockReturnValue(new Promise(() => {}));
  render(NotificationsPanel);
  await fireEvent.click(await screen.findByRole('button', { name: 'Turn on' }));
  const button = screen.getByRole('button', { name: 'Turn on' }) as HTMLButtonElement;
  expect(button.disabled).toBe(true);
  expect(button.getAttribute('aria-busy')).toBe('true');
});

test('sends a test and turns off', async () => {
  push.syncPush.mockResolvedValue('on');
  push.disablePush.mockResolvedValue('off');
  api.testPush.mockResolvedValue({ sent: 1 });
  render(NotificationsPanel);
  await fireEvent.click(await screen.findByRole('button', { name: 'Send a test' }));
  await waitFor(() => expect(toasts()).toEqual([['success', 'Test sent. It should appear in a few seconds.']]));
  await fireEvent.click(screen.getByRole('button', { name: 'Turn off' }));
  expect(await screen.findByText('Off')).toBeTruthy();
  expect(push.disablePush).toHaveBeenCalledOnce();
  expect(toasts().at(-1)).toEqual(['success', 'Notifications are off for this browser.']);
  expect(screen.getByRole('button', { name: 'Turn on' })).toBeTruthy();
});

test('a test no browser took says so', async () => {
  push.syncPush.mockResolvedValue('on');
  api.testPush.mockResolvedValue({ sent: 0 });
  render(NotificationsPanel);
  await fireEvent.click(await screen.findByRole('button', { name: 'Send a test' }));
  await waitFor(() => expect(toasts()).toEqual([['error', 'No browser took the test. Turn notifications off and on again.']]));
});

test('a failed test shows the error in a toast', async () => {
  push.syncPush.mockResolvedValue('on');
  api.testPush.mockRejectedValue(new api.ApiError('push is not configured'));
  render(NotificationsPanel);
  await fireEvent.click(await screen.findByRole('button', { name: 'Send a test' }));
  await waitFor(() => expect(toasts()).toEqual([['error', 'push is not configured']]));
});

test('blocked in the browser shows no buttons', async () => {
  push.syncPush.mockResolvedValue('blocked');
  render(NotificationsPanel);
  expect(await screen.findByText('Blocked in browser settings')).toBeTruthy();
  expect(screen.getByText(/Allow them in your browser’s site settings/)).toBeTruthy();
  expect(screen.queryByRole('button')).toBeNull();
});

test('a refused permission reports blocked', async () => {
  push.syncPush.mockResolvedValue('off');
  push.enablePush.mockResolvedValue('blocked');
  render(NotificationsPanel);
  await fireEvent.click(await screen.findByRole('button', { name: 'Turn on' }));
  expect(await screen.findByText('Blocked in browser settings')).toBeTruthy();
  expect(toasts()).toEqual([['error', 'Notifications are blocked in browser settings.']]);
});

test('an unsupported browser says so', async () => {
  push.syncPush.mockResolvedValue('unsupported');
  render(NotificationsPanel);
  expect(await screen.findByText(/This browser cannot show notifications/)).toBeTruthy();
  expect(screen.queryByRole('button')).toBeNull();
});
