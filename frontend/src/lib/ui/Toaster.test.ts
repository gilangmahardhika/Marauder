import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, expect, test } from 'vitest';
import { toast } from '$lib/toast.svelte';
import Toaster from './Toaster.svelte';

afterEach(() => {
  toast.clear();
});

test('both regions are present while empty', () => {
  render(Toaster);
  expect(screen.getByRole('status').childElementCount).toBe(0);
  expect(screen.getByRole('alert').childElementCount).toBe(0);
});

test('success and info go to the status region and errors to the alert region', async () => {
  render(Toaster);
  toast.success('Name saved');
  toast.error('Deploy failed');
  toast.info('Review: Export CSV', { href: '/cards/c1' });
  const status = screen.getByRole('status');
  const alert = screen.getByRole('alert');
  expect(await within(status).findByText('Name saved')).toBeTruthy();
  expect(within(status).getByText('Review: Export CSV')).toBeTruthy();
  expect(within(status).getByRole('link', { name: 'Open' }).getAttribute('href')).toBe('/cards/c1');
  expect(within(alert).getByText('Deploy failed')).toBeTruthy();
  expect(within(status).queryByText('Deploy failed')).toBeNull();
});

test('Dismiss removes the toast', async () => {
  render(Toaster);
  toast.success('Name saved');
  await fireEvent.click(await screen.findByRole('button', { name: 'Dismiss' }));
  expect(screen.queryByText('Name saved')).toBeNull();
});
