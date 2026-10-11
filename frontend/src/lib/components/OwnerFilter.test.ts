import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, test, vi } from 'vitest';
import type { OwnerChoice } from '$lib/board';
import OwnerFilter from './OwnerFilter.svelte';

const person = (id: string, name: string) => ({ id, name, email: `${id}@example.com`, githubLogin: '' });

const choices: OwnerChoice[] = [
  { id: 'ada', person: person('ada', 'Ada Lovelace'), count: 2 },
  { id: 'grace', person: person('grace', 'Grace Hopper'), count: 1 },
  { id: 'none', person: null, count: 1 }
];

test('clicking an avatar adds the owner, and clicking it again removes it', async () => {
  const onchange = vi.fn();
  render(OwnerFilter, { choices, selected: ['grace'], onchange });
  const ada = screen.getByRole('button', { name: 'Cards owned by Ada Lovelace' });
  expect(ada.getAttribute('aria-pressed')).toBe('false');
  await fireEvent.click(ada);
  expect(onchange).toHaveBeenLastCalledWith(['grace', 'ada']);
  const grace = screen.getByRole('button', { name: 'Cards owned by Grace Hopper' });
  expect(grace.getAttribute('aria-pressed')).toBe('true');
  await fireEvent.click(grace);
  expect(onchange).toHaveBeenLastCalledWith([]);
  expect(screen.getByRole('button', { name: 'Cards with no owner' })).toBeTruthy();
});

test('owners past the limit sit behind a +n checklist', async () => {
  const onchange = vi.fn();
  render(OwnerFilter, { choices, selected: ['none'], onchange, max: 2 });
  expect(screen.queryByRole('button', { name: 'Cards owned by Grace Hopper' })).toBeNull();
  const more = screen.getByRole('button', { name: '2 more owners, 1 selected' });
  await fireEvent.click(more);
  await fireEvent.click(screen.getByRole('checkbox', { name: /Grace Hopper/ }));
  expect(onchange).toHaveBeenLastCalledWith(['none', 'grace']);
});
