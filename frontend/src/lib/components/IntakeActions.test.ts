import { render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import IntakeActions from './IntakeActions.svelte';
import { toLines } from '$lib/requirements';

const noop = () => {};

test('header offers ingest and add, and no accept button', () => {
  render(IntakeActions, { oningest: noop });
  expect(screen.getByRole('button', { name: 'Ingest' })).toBeTruthy();
  expect(screen.getByRole('link', { name: 'Add requirement' })).toBeTruthy();
  expect(screen.queryByRole('button', { name: /accept/i })).toBeNull();
});

test('acceptance lines drop blanks and trim', () => {
  expect(toLines('  One row per card. \n\n Two \n')).toEqual(['One row per card.', 'Two']);
});
