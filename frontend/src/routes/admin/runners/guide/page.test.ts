import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, test, vi } from 'vitest';

vi.mock('$lib/api', () => ({ apiOrigin: 'https://orch.example.com' }));

import GuidePage from './+page.svelte';

test('shows register, update, and unregister steps with this server in the commands', () => {
  render(GuidePage);
  for (const name of ['Register a runner', 'Register without the install script', 'Update a runner', 'Pause a runner', 'Unregister a runner']) {
    expect(screen.getByRole('heading', { name })).toBeTruthy();
  }
  expect(
    screen.getByText(/curl -fsSL https:\/\/orch\.example\.com\/runner\/install\.sh \| sudo bash -s -- --url https:\/\/orch\.example\.com --token/)
  ).toBeTruthy();
  expect(screen.getByText(/sudo userdel --remove orchestrator/)).toBeTruthy();
  for (const link of screen.getAllByRole('link', { name: 'Add runner' })) {
    expect(link.getAttribute('href')).toBe('/admin/runners/new');
  }
});

test('copies a command', async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
  render(GuidePage);
  await fireEvent.click(screen.getByRole('button', { name: 'Copy example' }));
  expect(writeText).toHaveBeenCalledWith(expect.stringContaining('--token <token>'));
  expect(await screen.findByRole('button', { name: 'Copied' })).toBeTruthy();
});
