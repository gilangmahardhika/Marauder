import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import PasswordInput from './PasswordInput.svelte';

test('the eye toggles the input between masked and plain text', async () => {
  const { container } = render(PasswordInput, { id: 'pw', label: 'Password', value: 'hunter2-secret' });
  const input = container.querySelector('input#pw') as HTMLInputElement;
  expect(input.type).toBe('password');

  const show = screen.getByRole('button', { name: 'Show Password' });
  expect(show.getAttribute('type')).toBe('button');
  expect(show.getAttribute('aria-pressed')).toBe('false');
  expect(show.getAttribute('aria-controls')).toBe('pw');

  await fireEvent.click(show);
  expect(input.type).toBe('text');
  const hide = screen.getByRole('button', { name: 'Hide Password' });
  expect(hide.getAttribute('aria-pressed')).toBe('true');

  await fireEvent.click(hide);
  expect(input.type).toBe('password');
});
