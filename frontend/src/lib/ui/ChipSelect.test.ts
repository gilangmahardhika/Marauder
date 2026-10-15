import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { expect, test, vi } from 'vitest';
import ChipSelect from './ChipSelect.svelte';

const options = [
  { value: 'claude-a', label: 'claude-a', group: 'anthropic', tag: 'Implement' },
  { value: 'claude-b', label: 'claude-b', group: 'anthropic' },
  { value: 'gpt-b', label: 'gpt-b', group: 'openai' }
];

function setup(extra: Record<string, unknown> = {}) {
  const onchange = vi.fn();
  render(ChipSelect, { label: 'Model', id: 'model', value: 'claude-a', display: 'claude-a', options, onchange, ...extra });
  return { onchange, chip: screen.getByRole('combobox', { name: 'Model' }) };
}

test('a click opens the listbox with grouped options and a check on the selected one', async () => {
  const { chip } = setup();
  expect(chip.getAttribute('aria-expanded')).toBe('false');
  expect(screen.queryByRole('listbox')).toBeNull();
  await fireEvent.click(chip);
  expect(chip.getAttribute('aria-expanded')).toBe('true');
  const list = screen.getByRole('listbox', { name: 'Model' });
  const anthropic = within(list).getByRole('group', { name: 'anthropic' });
  expect(within(anthropic).getAllByRole('option').map((option) => option.textContent?.replace(/\s+/g, ' ').trim())).toEqual([
    'claude-a Implement',
    'claude-b'
  ]);
  expect(within(list).getByRole('group', { name: 'openai' })).toBeTruthy();
  expect(screen.getByRole('option', { name: /claude-a/ }).getAttribute('aria-selected')).toBe('true');
  expect(chip.getAttribute('aria-activedescendant')).toBe('model-opt-0');
});

test('ArrowDown then Enter picks the next option and calls back', async () => {
  const { chip, onchange } = setup();
  await fireEvent.keyDown(chip, { key: 'ArrowDown' });
  expect(screen.getByRole('listbox')).toBeTruthy();
  await fireEvent.keyDown(chip, { key: 'ArrowDown' });
  expect(chip.getAttribute('aria-activedescendant')).toBe('model-opt-1');
  await fireEvent.keyDown(chip, { key: 'Enter' });
  expect(onchange).toHaveBeenCalledWith('claude-b');
  expect(screen.queryByRole('listbox')).toBeNull();
});

test('a click on an option picks it, and End and Home jump', async () => {
  const { chip, onchange } = setup();
  await fireEvent.click(chip);
  await fireEvent.keyDown(chip, { key: 'End' });
  expect(chip.getAttribute('aria-activedescendant')).toBe('model-opt-2');
  await fireEvent.keyDown(chip, { key: 'Home' });
  expect(chip.getAttribute('aria-activedescendant')).toBe('model-opt-0');
  await fireEvent.click(screen.getByRole('option', { name: 'gpt-b' }));
  expect(onchange).toHaveBeenCalledWith('gpt-b');
});

test('Escape closes with focus on the chip', async () => {
  const { chip, onchange } = setup();
  chip.focus();
  await fireEvent.keyDown(chip, { key: 'Enter' });
  await fireEvent.keyDown(chip, { key: 'Escape' });
  expect(screen.queryByRole('listbox')).toBeNull();
  expect(document.activeElement).toBe(chip);
  expect(onchange).not.toHaveBeenCalled();
});

test('a letter jumps to the next option starting with it', async () => {
  const { chip } = setup();
  await fireEvent.click(chip);
  await fireEvent.keyDown(chip, { key: 'g' });
  expect(chip.getAttribute('aria-activedescendant')).toBe('model-opt-2');
  await fireEvent.keyDown(chip, { key: 'c' });
  expect(chip.getAttribute('aria-activedescendant')).toBe('model-opt-0');
});

test('a disabled chip does not open', async () => {
  const { chip } = setup({ disabled: true });
  expect((chip as HTMLButtonElement).disabled).toBe(true);
  await fireEvent.click(chip);
  await fireEvent.keyDown(chip, { key: 'ArrowDown' });
  expect(screen.queryByRole('listbox')).toBeNull();
});
