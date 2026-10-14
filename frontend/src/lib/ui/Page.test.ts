import { fireEvent, render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { expect, test, vi } from 'vitest';
import Page from './Page.svelte';

const children = createRawSnippet(() => ({ render: () => '<p>Body</p>' }));

test('loading shows a status row instead of the body', () => {
  render(Page, { title: 'Board', loading: 'Loading board', children });
  expect(screen.getByRole('heading', { name: 'Board' })).toBeTruthy();
  expect(screen.getByRole('status').textContent).toContain('Loading board');
  expect(screen.queryByText('Body')).toBeNull();
});

test('error shows the message and Try again calls onretry', async () => {
  const onretry = vi.fn();
  render(Page, { title: 'Board', error: 'That project is not available.', onretry, children });
  expect(screen.getByRole('alert').textContent).toContain('That project is not available.');
  expect(screen.queryByText('Body')).toBeNull();
  await fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
  expect(onretry).toHaveBeenCalledOnce();
});

test('the body renders when neither is set', () => {
  render(Page, { title: 'Board', children });
  expect(screen.getByText('Body')).toBeTruthy();
  expect(screen.queryByRole('status')).toBeNull();
});
