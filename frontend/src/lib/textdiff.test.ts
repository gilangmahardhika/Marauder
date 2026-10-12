import { expect, test } from 'vitest';
import { changeCount, diffLines, diffWords, type DiffPart } from './textdiff';

const side = (parts: DiffPart[], kind: 'add' | 'del') =>
  parts
    .filter((part) => part.kind !== kind)
    .map((part) => part.text)
    .join('');

test('a word diff marks the changed words and rebuilds both sides', () => {
  const before = 'Export the board quickly.';
  const after = 'Export the whole board as CSV.';
  const parts = diffWords(before, after);
  expect(side(parts, 'add')).toBe(before);
  expect(side(parts, 'del')).toBe(after);
  expect(parts.filter((part) => part.kind === 'add').map((part) => part.text.trim())).toContain('whole');
  expect(parts.filter((part) => part.kind === 'del').map((part) => part.text)).toContain('quickly.');
  expect(parts[0]).toEqual({ kind: 'same', text: 'Export the ' });
});

test('identical and empty texts', () => {
  expect(diffWords('Same text.', 'Same text.')).toEqual([{ kind: 'same', text: 'Same text.' }]);
  expect(diffWords('', '')).toEqual([]);
  expect(diffWords('', 'New.')).toEqual([{ kind: 'add', text: 'New.' }]);
  expect(diffWords('Old.', '')).toEqual([{ kind: 'del', text: 'Old.' }]);
});

test('newlines survive a word diff', () => {
  const parts = diffWords('# Title\n\nBody one.', '# Title\n\nBody two.');
  expect(side(parts, 'del')).toBe('# Title\n\nBody two.');
});

test('a line diff keeps unchanged lines and marks added and removed ones', () => {
  const parts = diffLines(['One row per card.', 'Works well.', 'Uses CSV.'], ['One row per card.', 'Loads in 2 s.', 'Uses CSV.']);
  expect(parts).toEqual([
    { kind: 'same', text: 'One row per card.' },
    { kind: 'del', text: 'Works well.' },
    { kind: 'add', text: 'Loads in 2 s.' },
    { kind: 'same', text: 'Uses CSV.' }
  ]);
  expect(changeCount(parts)).toEqual({ added: 1, removed: 1 });
  expect(diffLines([], ['A.'])).toEqual([{ kind: 'add', text: 'A.' }]);
  expect(diffLines(['A.'], ['A.'])).toEqual([{ kind: 'same', text: 'A.' }]);
});

test('a very large change still rebuilds both sides', () => {
  const before = Array.from({ length: 2000 }, (_, i) => `a${i}`).join(' ');
  const after = Array.from({ length: 2000 }, (_, i) => `b${i}`).join(' ');
  const parts = diffWords(before, after);
  expect(side(parts, 'add')).toBe(before);
  expect(side(parts, 'del')).toBe(after);
});
