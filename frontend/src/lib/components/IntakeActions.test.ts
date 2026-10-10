import { render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import IntakeActions from './IntakeActions.svelte';
import { toLines } from '$lib/requirements';
import type { Intake } from '$lib/types';

function intake(partial: Partial<Intake>): Intake {
  return { draftPending: false, planError: null, canAccept: false, requirements: [], decisions: [], ...partial };
}

const noop = () => {};

test('accept is hidden when canAccept is false', () => {
  render(IntakeActions, { intake: intake({ canAccept: false }), oningest: noop, onaccept: noop });
  expect(screen.queryByRole('button', { name: 'Accept unaccepted ids' })).toBeNull();
  expect(screen.getByRole('link', { name: 'Add requirement' })).toBeTruthy();
});

test('accept renders when canAccept is true', () => {
  render(IntakeActions, { intake: intake({ canAccept: true }), oningest: noop, onaccept: noop });
  expect(screen.getByRole('button', { name: 'Accept unaccepted ids' })).toBeTruthy();
});

test('acceptance lines drop blanks and trim', () => {
  expect(toLines('  One row per card. \n\n Two \n')).toEqual(['One row per card.', 'Two']);
});
