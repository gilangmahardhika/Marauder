import { expect, test } from 'vitest';
import { changedLines, groupByFolder, hunkLabel, numberLines, parseHunkHeader, splitPath } from './diff';
import type { DiffFile } from './types';

test('parses a hunk header with counts, without counts, and with context', () => {
  expect(parseHunkHeader('@@ -67,7 +67,8 @@ function login() {')).toEqual({
    oldStart: 67,
    oldCount: 7,
    newStart: 67,
    newCount: 8,
    context: 'function login() {'
  });
  expect(parseHunkHeader('@@ -1 +1 @@')).toEqual({ oldStart: 1, oldCount: 1, newStart: 1, newCount: 1, context: '' });
  expect(parseHunkHeader('not a header')).toBeNull();
});

test('numbers old and new lines', () => {
  const lines = numberLines({
    header: '@@ -10,4 +10,4 @@',
    lines: [
      { kind: 'ctx', text: 'a' },
      { kind: 'del', text: 'b' },
      { kind: 'add', text: 'B' },
      { kind: 'add', text: 'C' },
      { kind: 'meta', text: '\\ No newline at end of file' },
      { kind: 'ctx', text: 'd' }
    ]
  });
  expect(lines.map((line) => [line.old, line.new])).toEqual([
    [10, 10],
    [11, null],
    [null, 11],
    [null, 12],
    [null, null],
    [12, 13]
  ]);
});

test('a header that does not parse leaves lines unnumbered', () => {
  const lines = numberLines({ header: '@@ odd', lines: [{ kind: 'add', text: 'x' }] });
  expect(lines[0]).toMatchObject({ old: null, new: null });
});

test('labels a hunk by its new lines, or its old lines for a deletion', () => {
  expect(hunkLabel('@@ -67,7 +67,7 @@')).toBe('Lines 67 to 73');
  expect(hunkLabel('@@ -5 +5 @@')).toBe('Line 5');
  expect(hunkLabel('@@ -3,4 +2,0 @@')).toBe('Lines 3 to 6');
  expect(hunkLabel('@@ odd')).toBe('@@ odd');
});

const file = (path: string, added = 1, deleted = 0, binary = false): DiffFile => ({
  path,
  added,
  deleted,
  binary,
  truncated: false,
  hunks: []
});

test('groups files by folder in first-seen order and keeps their indexes', () => {
  const groups = groupByFolder([file('src/a.ts'), file('README.md'), file('src/b.ts')]);
  expect(groups.map((group) => [group.folder, group.files.map((entry) => entry.index)])).toEqual([
    ['src', [0, 2]],
    ['', [1]]
  ]);
  expect(splitPath('resources/js/Login.tsx')).toEqual({ folder: 'resources/js', name: 'Login.tsx' });
});

test('counts changed lines and ignores binary files', () => {
  expect(changedLines(file('a', 3, 2))).toBe(5);
  expect(changedLines(file('logo.png', -1, -1, true))).toBe(0);
});
