import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { expect, test } from 'vitest';
import Button from './Button.svelte';

const children = createRawSnippet(() => ({ render: () => '<span>Save</span>' }));

test('icon renders a hidden svg before the label without changing the name', () => {
  render(Button, { icon: 'save', children });
  const button = screen.getByRole('button', { name: 'Save' });
  const svg = button.querySelector('svg');
  expect(svg?.getAttribute('aria-hidden')).toBe('true');
  expect(button.firstElementChild).toBe(svg);
});

test('loading disables the button, sets aria-busy, and keeps the label', () => {
  render(Button, { icon: 'save', loading: true, type: 'submit', children });
  const button = screen.getByRole('button', { name: 'Save' });
  expect((button as HTMLButtonElement).disabled).toBe(true);
  expect(button.getAttribute('aria-busy')).toBe('true');
  expect(button.querySelector('.animate-spin')).not.toBeNull();
});

test('aria-label and title pass through, and an href renders a link', () => {
  render(Button, { 'aria-label': 'Remove Ada', title: 'Remove', href: '/x', trailing: 'external', children });
  const link = screen.getByRole('link', { name: 'Remove Ada' });
  expect(link.getAttribute('href')).toBe('/x');
  expect(link.getAttribute('title')).toBe('Remove');
  expect(link.lastElementChild?.tagName.toLowerCase()).toBe('svg');
});
