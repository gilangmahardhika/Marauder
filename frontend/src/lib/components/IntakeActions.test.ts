import { render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import IntakeActions from './IntakeActions.svelte';
import { fromLines, toLines } from '$lib/requirements';

const noop = () => {};

test('header offers ingest and add, and no accept button', () => {
  render(IntakeActions, { oningest: noop });
  expect(screen.getByRole('button', { name: 'Ingest' })).toBeTruthy();
  expect(screen.getByRole('link', { name: 'Add requirement' })).toBeTruthy();
  expect(screen.queryByRole('button', { name: /accept/i })).toBeNull();
});

test('acceptance lines drop blanks and trim', () => {
  expect(toLines('  One row per card. \n\n Two \n')).toEqual(['One row per card.', 'Two']);
  expect(toLines('- a\n* b\n+ c\n1. d\n2) e\n- [ ] f\n- [x] g\n-\n\n**h** label\n-1 prose')).toEqual([
    'a',
    'b',
    'c',
    'd',
    'e',
    'f',
    'g'
  ]);
  expect(toLines('**h** stays\n-1 stays')).toEqual(['**h** stays', '-1 stays']);
});

test('saved checks load as a list and save back unchanged', () => {
  const saved = ['Uses `text/csv`', '**Status** uses board labels', '- a line that began with a dash'];
  expect(fromLines(saved)).toBe('- Uses `text/csv`\n- **Status** uses board labels\n- - a line that began with a dash');
  expect(toLines(fromLines(saved))).toEqual(saved);
  expect(toLines(`${fromLines(saved)}\n- added later`)).toEqual([...saved, 'added later']);
  expect(fromLines([])).toBe('');
});
