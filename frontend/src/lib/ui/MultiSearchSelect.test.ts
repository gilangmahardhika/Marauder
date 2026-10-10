import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, test, vi } from 'vitest';
import MultiSearchSelect from './MultiSearchSelect.svelte';

const options = [
  { value: 'usr_1', label: 'grace', detail: 'grace@example.com' },
  { value: 'usr_2', label: 'hopper', detail: 'hopper@example.com' },
  { value: 'usr_3', label: 'lovelace', detail: 'ada@example.com' }
];

function setup(values: string[] = [], extra: Record<string, unknown> = {}) {
  const onchange = vi.fn();
  render(MultiSearchSelect, { label: 'Reviewers', id: 'reviewers', options, values, onchange, ...extra });
  return { onchange, input: () => screen.queryByRole('combobox', { name: 'Reviewers' }) as HTMLInputElement | null };
}

test('picking an option adds a chip and clears the input', async () => {
  const { onchange, input } = setup();
  await fireEvent.input(input()!, { target: { value: 'hop' } });
  await fireEvent.click(screen.getByRole('option', { name: /hopper/ }));
  expect(onchange).toHaveBeenLastCalledWith(['usr_2']);
  expect(screen.getByRole('button', { name: 'Remove hopper' })).toBeTruthy();
  expect(input()!.value).toBe('');
});

test('chosen options are not offered again', async () => {
  const { input } = setup(['usr_1']);
  await fireEvent.click(input()!);
  const names = screen.getAllByRole('option').map((option) => option.textContent);
  expect(names).toHaveLength(2);
  expect(names.some((name) => name?.includes('grace'))).toBe(false);
});

test('the remove button drops a chip', async () => {
  const { onchange } = setup(['usr_1', 'usr_3']);
  await fireEvent.click(screen.getByRole('button', { name: 'Remove grace' }));
  expect(onchange).toHaveBeenLastCalledWith(['usr_3']);
  expect(screen.queryByRole('button', { name: 'Remove grace' })).toBeNull();
  expect(screen.getByRole('button', { name: 'Remove lovelace' })).toBeTruthy();
});

test('the input hides at the limit', () => {
  const { input } = setup(['usr_1', 'usr_2'], { max: 2 });
  expect(input()).toBeNull();
  expect(screen.getAllByRole('button')).toHaveLength(2);
});

test('disabled shows chips without remove buttons', () => {
  const { input } = setup(['usr_1'], { disabled: true });
  expect(input()).toBeNull();
  expect(screen.getByText('grace')).toBeTruthy();
  expect(screen.queryByRole('button')).toBeNull();
});
