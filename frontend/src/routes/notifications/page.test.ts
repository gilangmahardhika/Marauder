import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { inbox } from '$lib/notifications.svelte';
import { session } from '$lib/session';
import type { AppNotification } from '$lib/types';

const api = vi.hoisted(() => ({ getNotifications: vi.fn(), readNotifications: vi.fn() }));

vi.mock('$lib/api', () => api);

import NotificationsPage from './+page.svelte';

const hour = 3_600_000;

function note(id: string, title: string, age: number, readAt: string | null = null): AppNotification {
  return {
    id,
    kind: 'review',
    title,
    body: 'Export CSV · Widget',
    projectId: 'proj_a',
    cardId: 'card_1',
    url: '/cards/card_1?project=proj_a',
    createdAt: new Date(Date.now() - age).toISOString(),
    readAt
  };
}

function rowLinks() {
  return screen.getAllByRole('listitem').map((item) => within(item).getByRole('link'));
}

beforeEach(() => {
  session.setProject('proj_a');
  api.getNotifications.mockImplementation(async (unread: boolean) =>
    unread
      ? { notifications: [note('ntf_3', 'Review needed', 0)], unread: 1 }
      : {
          notifications: [
            note('ntf_3', 'Review needed', 0),
            note('ntf_2', 'Changes requested', 24 * hour, '2026-10-14T06:00:00Z'),
            note('ntf_1', 'Blocked', 5 * 24 * hour, '2026-10-14T06:00:00Z')
          ],
          unread: 1
        }
  );
  api.readNotifications.mockResolvedValue({ unread: 0 });
});

afterEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
});

test('the page lists the newest notifications grouped by day, newest first', async () => {
  render(NotificationsPage);
  expect(await screen.findByRole('heading', { name: 'Today' })).toBeTruthy();
  const headings = screen.getAllByRole('heading', { level: 2 }).map((heading) => heading.textContent);
  expect(headings.slice(0, 2)).toEqual(['Today', 'Yesterday']);
  expect(headings).toHaveLength(3);
  expect(rowLinks().map((link) => link.textContent)).toEqual([
    expect.stringContaining('Review needed'),
    expect.stringContaining('Changes requested'),
    expect.stringContaining('Blocked')
  ]);
  expect(api.getNotifications).toHaveBeenCalledWith(false);
});

test('Unread reads only unread notifications', async () => {
  render(NotificationsPage);
  await screen.findByRole('heading', { name: 'Today' });
  await fireEvent.click(screen.getByRole('button', { name: /Unread/ }));
  await waitFor(() => expect(api.getNotifications).toHaveBeenLastCalledWith(true));
  await waitFor(() => expect(rowLinks()).toHaveLength(1));
  expect(rowLinks()[0].textContent).toContain('Review needed');
});

test('opening a notification marks it read and lowers the shared count', async () => {
  render(NotificationsPage);
  await screen.findByRole('heading', { name: 'Today' });
  const first = rowLinks()[0];
  first.addEventListener('click', (event) => event.preventDefault());
  await fireEvent.click(first);
  expect(api.readNotifications).toHaveBeenCalledWith({ ids: ['ntf_3'] });
  await waitFor(() => expect(inbox.unread).toBe(0));
  expect(rowLinks()[0].textContent).not.toContain('Unread');
});

test('Mark all read posts all and disables itself', async () => {
  render(NotificationsPage);
  await screen.findByRole('heading', { name: 'Today' });
  await fireEvent.click(screen.getByRole('button', { name: 'Mark all read' }));
  expect(api.readNotifications).toHaveBeenCalledWith({ all: true });
  await waitFor(() => expect(screen.getByRole('button', { name: 'Mark all read' }).hasAttribute('disabled')).toBe(true));
  expect(rowLinks().some((link) => link.textContent?.includes('Unread'))).toBe(false);
});

test('repeats for one card and kind on a day are one row, and opening it reads them all', async () => {
  const blocked = (id: string, age: number) => ({ ...note(id, 'Blocked', age), kind: 'blocked' });
  api.getNotifications.mockResolvedValue({
    notifications: [blocked('ntf_c', 0), blocked('ntf_b', 1000), blocked('ntf_a', 2000), note('ntf_r', 'Review needed', 3000)],
    unread: 4
  });
  render(NotificationsPage);
  await screen.findByRole('heading', { name: 'Today' });
  const links = rowLinks();
  expect(links).toHaveLength(2);
  expect(links[0].textContent).toContain('Blocked');
  expect(links[0].textContent).toContain('3x');
  links[0].addEventListener('click', (event) => event.preventDefault());
  await fireEvent.click(links[0]);
  expect(api.readNotifications).toHaveBeenCalledWith({ ids: ['ntf_c', 'ntf_b', 'ntf_a'] });
});

test('a type in the side panel filters the list, and its chip clears it', async () => {
  api.getNotifications.mockResolvedValue({
    notifications: [note('ntf_r', 'Review needed', 0), { ...note('ntf_x', 'Blocked', 1000), kind: 'blocked', cardId: 'card_2' }],
    unread: 2
  });
  render(NotificationsPage);
  await screen.findByRole('heading', { name: 'Today' });
  expect(rowLinks()).toHaveLength(2);
  const types = screen.getByRole('group', { name: 'Filter by type' });
  await fireEvent.click(within(types).getByRole('button', { name: /Blocked/ }));
  expect(rowLinks().map((link) => link.textContent)).toEqual([expect.stringContaining('Blocked')]);
  await fireEvent.click(screen.getByRole('button', { name: 'Remove filter Blocked' }));
  expect(rowLinks()).toHaveLength(2);
});

test('a link off this site links to /, and an empty list says so', async () => {
  api.getNotifications.mockResolvedValue({
    notifications: [{ ...note('ntf_9', 'Review needed', 0), url: 'https://evil.example/x' }],
    unread: 1
  });
  const { unmount } = render(NotificationsPage);
  await screen.findByRole('heading', { name: 'Today' });
  expect(rowLinks()[0].getAttribute('href')).toBe('/');
  unmount();
  api.getNotifications.mockResolvedValue({ notifications: [], unread: 0 });
  render(NotificationsPage);
  expect(await screen.findByText('No notifications yet')).toBeTruthy();
});
