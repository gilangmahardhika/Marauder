import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { session } from '$lib/session';
import { toast } from '$lib/toast.svelte';
import type { AppNotification } from '$lib/types';

const api = vi.hoisted(() => ({ getNotifications: vi.fn(), readNotifications: vi.fn() }));
const stream = vi.hoisted(() => ({ handlers: [] as ((event: unknown) => void)[] }));

vi.mock('$lib/api', () => api);
vi.mock('$lib/stream.svelte', () => ({
  live: {
    on: (handler: (event: unknown) => void) => {
      stream.handlers.push(handler);
      return () => {
        stream.handlers = stream.handlers.filter((item) => item !== handler);
      };
    }
  }
}));

import NotificationBell from './NotificationBell.svelte';

function note(id: string, title: string, readAt: string | null = null, projectId = 'proj_a'): AppNotification {
  return {
    id,
    kind: 'review',
    title,
    body: 'Export CSV · Widget',
    projectId,
    cardId: 'card_1',
    url: `/cards/card_1?project=${projectId}`,
    createdAt: '2026-10-14T05:00:00Z',
    readAt
  };
}

function push(event: unknown) {
  for (const handler of stream.handlers) handler(event);
}

function rowLinks() {
  return within(screen.getByRole('list', { name: 'Latest notifications' })).getAllByRole('link');
}

beforeEach(() => {
  session.setProject('proj_a');
  api.getNotifications.mockResolvedValue({
    notifications: [note('ntf_2', 'Review needed'), note('ntf_1', 'Done', '2026-10-14T06:00:00Z')],
    unread: 1
  });
  api.readNotifications.mockResolvedValue({ unread: 0 });
});

afterEach(() => {
  vi.clearAllMocks();
  stream.handlers = [];
  sessionStorage.clear();
  toast.clear();
});

test('the bell shows the unread count and lists notifications as links', async () => {
  render(NotificationBell);
  const bell = await screen.findByRole('button', { name: 'Notifications, 1 unread' });
  await fireEvent.click(bell);
  const links = rowLinks();
  expect(links.map((link) => link.getAttribute('href'))).toEqual(['/cards/card_1?project=proj_a', '/cards/card_1?project=proj_a']);
  expect(links[0].textContent).toContain('Review needed');
  expect(links[0].textContent).toContain('Unread');
  expect(links[1].textContent).not.toContain('Unread');
});

test('the panel lists the newest 8 and links to all notifications', async () => {
  api.getNotifications.mockResolvedValue({
    notifications: Array.from({ length: 10 }, (_, i) => note(`ntf_${i}`, `Note ${i}`)),
    unread: 10
  });
  render(NotificationBell);
  await fireEvent.click(await screen.findByRole('button', { name: 'Notifications, 10 unread' }));
  const links = rowLinks();
  expect(links).toHaveLength(8);
  expect(links[7].textContent).toContain('Note 7');
  const all = within(screen.getByRole('group', { name: 'Notifications' })).getByRole('link', { name: 'Open all notifications' });
  expect(all.getAttribute('href')).toBe('/notifications');
  all.addEventListener('click', (event) => event.preventDefault());
  await fireEvent.click(all);
  expect(screen.queryByRole('group', { name: 'Notifications' })).toBeNull();
});

test('a pushed notification raises the count, joins the top of the list, and is announced', async () => {
  render(NotificationBell);
  await screen.findByRole('button', { name: 'Notifications, 1 unread' });
  push({ kind: 'notification', notification: note('ntf_3', 'Changes requested'), unread: 2 });
  expect(await screen.findByRole('button', { name: 'Notifications, 2 unread' })).toBeTruthy();
  expect(toast.items.map(({ tone, text, href, label }) => ({ tone, text, href, label }))).toEqual([
    { tone: 'info', text: 'Changes requested: Export CSV · Widget', href: '/cards/card_1?project=proj_a', label: 'Open' }
  ]);
  api.getNotifications.mockReturnValue(new Promise(() => {}));
  await fireEvent.click(screen.getByRole('button', { name: 'Notifications, 2 unread' }));
  const links = rowLinks();
  expect(links.map((link) => link.textContent)).toEqual([
    expect.stringContaining('Changes requested'),
    expect.stringContaining('Review needed'),
    expect.stringContaining('Done')
  ]);
});

test('Mark all read clears the count', async () => {
  render(NotificationBell);
  await fireEvent.click(await screen.findByRole('button', { name: 'Notifications, 1 unread' }));
  await fireEvent.click(screen.getByRole('button', { name: 'Mark all read' }));
  expect(api.readNotifications).toHaveBeenCalledWith({ all: true });
  expect(await screen.findByRole('button', { name: 'Notifications' })).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Mark all read' }).hasAttribute('disabled')).toBe(true);
});

test('opening a notification marks it read', async () => {
  render(NotificationBell);
  await fireEvent.click(await screen.findByRole('button', { name: 'Notifications, 1 unread' }));
  const first = rowLinks()[0];
  first.addEventListener('click', (event) => event.preventDefault());
  await fireEvent.click(first);
  expect(api.readNotifications).toHaveBeenCalledWith({ ids: ['ntf_2'] });
  await waitFor(() => expect(screen.getByRole('button', { name: 'Notifications' })).toBeTruthy());
});

test('a resync reads the list again, and a link off this site is not followed', async () => {
  render(NotificationBell);
  await screen.findByRole('button', { name: 'Notifications, 1 unread' });
  api.getNotifications.mockResolvedValue({
    notifications: [{ ...note('ntf_9', 'Review needed'), url: 'https://evil.example/x' }],
    unread: 1
  });
  push({ kind: 'resync' });
  await waitFor(() => expect(api.getNotifications).toHaveBeenCalledTimes(2));
  await fireEvent.click(screen.getByRole('button', { name: 'Notifications, 1 unread' }));
  await waitFor(() => expect(rowLinks()[0].getAttribute('href')).toBe('/'));
});

test('a pushed notification with a link off this site opens / from its toast', async () => {
  render(NotificationBell);
  await screen.findByRole('button', { name: 'Notifications, 1 unread' });
  push({ kind: 'notification', notification: { ...note('ntf_4', 'Blocked'), url: '//evil.example/x' }, unread: 2 });
  expect(toast.items.map((item) => item.href)).toEqual(['/']);
});
