import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, expect, test, vi } from 'vitest';

const api = vi.hoisted(() => ({
  ApiError: class ApiError extends Error {},
  testPush: vi.fn(),
  unreachable: () => 'The orchestrator API is unreachable.'
}));
const push = vi.hoisted(() => ({ syncPush: vi.fn(), enablePush: vi.fn(), disablePush: vi.fn() }));

vi.mock('$lib/api', () => api);
vi.mock('$lib/push', () => push);

import NotificationsPanel from './NotificationsPanel.svelte';

beforeEach(() => {
  vi.clearAllMocks();
});

test('turns notifications on', async () => {
  push.syncPush.mockResolvedValue('off');
  push.enablePush.mockResolvedValue('on');
  render(NotificationsPanel);
  expect(await screen.findByText('Off')).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Turn on' }));
  expect(await screen.findByText('On')).toBeTruthy();
  expect(push.enablePush).toHaveBeenCalledOnce();
  expect(screen.getByRole('status').textContent).toBe('Notifications are on for this browser.');
  expect(screen.getByRole('button', { name: 'Send a test' })).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Turn on' })).toBeNull();
});

test('sends a test and turns off', async () => {
  push.syncPush.mockResolvedValue('on');
  push.disablePush.mockResolvedValue('off');
  api.testPush.mockResolvedValue({ sent: 1 });
  render(NotificationsPanel);
  await fireEvent.click(await screen.findByRole('button', { name: 'Send a test' }));
  expect(await screen.findByText('Test sent. It should appear in a few seconds.')).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Turn off' }));
  expect(await screen.findByText('Off')).toBeTruthy();
  expect(push.disablePush).toHaveBeenCalledOnce();
  expect(screen.getByRole('button', { name: 'Turn on' })).toBeTruthy();
});

test('a test no browser took says so', async () => {
  push.syncPush.mockResolvedValue('on');
  api.testPush.mockResolvedValue({ sent: 0 });
  render(NotificationsPanel);
  await fireEvent.click(await screen.findByRole('button', { name: 'Send a test' }));
  expect(await screen.findByText(/No browser took the test/)).toBeTruthy();
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
  expect(screen.getByRole('status').textContent).toBe('Notifications are blocked in browser settings.');
});

test('an unsupported browser says so', async () => {
  push.syncPush.mockResolvedValue('unsupported');
  render(NotificationsPanel);
  expect(await screen.findByText(/This browser cannot show notifications/)).toBeTruthy();
  expect(screen.queryByRole('button')).toBeNull();
});
