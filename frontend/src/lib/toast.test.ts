import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { toast } from './toast.svelte';

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  toast.clear();
  vi.useRealTimers();
});

test('success and info close after 5 seconds and errors after 8', () => {
  toast.success('Saved');
  toast.info('Card changed. Reloaded.');
  toast.error('Deploy failed');
  vi.advanceTimersByTime(4999);
  expect(toast.items).toHaveLength(3);
  vi.advanceTimersByTime(1);
  expect(toast.items.map((item) => item.text)).toEqual(['Deploy failed']);
  vi.advanceTimersByTime(3000);
  expect(toast.items).toHaveLength(0);
});

test('pause holds the timer and resume restarts it', () => {
  const id = toast.success('Saved');
  vi.advanceTimersByTime(4000);
  toast.pause(id);
  vi.advanceTimersByTime(10000);
  expect(toast.items).toHaveLength(1);
  toast.resume(id);
  vi.advanceTimersByTime(4999);
  expect(toast.items).toHaveLength(1);
  vi.advanceTimersByTime(1);
  expect(toast.items).toHaveLength(0);
});

test('a fourth toast drops the oldest', () => {
  toast.success('One');
  toast.success('Two');
  toast.success('Three');
  toast.success('Four');
  expect(toast.items.map((item) => item.text)).toEqual(['Two', 'Three', 'Four']);
});

test('the same text and tone as the newest toast adds nothing', () => {
  const first = toast.error('API unreachable');
  vi.advanceTimersByTime(6000);
  expect(toast.error('API unreachable')).toBe(first);
  expect(toast.items).toHaveLength(1);
  vi.advanceTimersByTime(7999);
  expect(toast.items).toHaveLength(1);
  toast.success('API unreachable');
  expect(toast.items).toHaveLength(2);
});

test('dismiss removes one, and blank text adds nothing', () => {
  const id = toast.info('Review: Export CSV', { href: '/cards/c1' });
  expect(toast.items[0]).toMatchObject({ href: '/cards/c1', tone: 'info' });
  toast.success('  ');
  toast.dismiss(id);
  expect(toast.items).toHaveLength(0);
});
