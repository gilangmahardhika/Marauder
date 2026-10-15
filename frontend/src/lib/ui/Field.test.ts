import { render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import Checkbox from './Checkbox.svelte';
import Field from './Field.svelte';

test('the error follows the control and is named by aria-describedby with the hint', () => {
  const { container } = render(Field, { label: 'Name', id: 'name', hint: 'Shown on cards', error: 'Name is required' });
  const input = screen.getByLabelText('Name');
  expect(input.getAttribute('aria-invalid')).toBe('true');
  expect(input.getAttribute('aria-describedby')).toBe('name-hint name-error');
  const error = container.querySelector('#name-error');
  expect(error?.textContent).toContain('Name is required');
  const order = [...container.querySelectorAll('label, #name-hint, input, #name-error')].map(
    (node) => node.id || node.tagName.toLowerCase()
  );
  expect(order).toEqual(['label', 'name-hint', 'name', 'name-error']);
});

test('a field without an error is not invalid', () => {
  render(Field, { label: 'Name', id: 'name' });
  const input = screen.getByLabelText('Name');
  expect(input.hasAttribute('aria-invalid')).toBe(false);
  expect(input.hasAttribute('aria-describedby')).toBe(false);
});

test('an input and a textarea show focus by a darker border, not an outline', () => {
  render(Field, { label: 'Name', id: 'name' });
  render(Field, { label: 'Notes', id: 'notes', type: 'textarea' });
  for (const field of [screen.getByLabelText('Name'), screen.getByLabelText('Notes')]) {
    expect(field.className).toContain('focus-visible:outline-hidden');
    expect(field.className).toContain('focus-visible:border-muted');
  }
});

test('a checkbox shows its error', () => {
  render(Checkbox, { id: 'admin', label: 'Admin', error: 'Only an admin can grant admin' });
  const box = screen.getByLabelText('Admin');
  expect(box.getAttribute('aria-invalid')).toBe('true');
  expect(document.getElementById(box.getAttribute('aria-describedby') ?? '')?.textContent).toContain(
    'Only an admin can grant admin'
  );
});
