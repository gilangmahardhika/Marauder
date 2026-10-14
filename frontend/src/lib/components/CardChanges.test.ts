import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import CardChanges from './CardChanges.svelte';
import type { CardDiff, DiffFile } from '$lib/types';

const diff: CardDiff = {
  sha: 'ae5d4418f1f5c54e02c2ced82de2a5b66ba65753',
  base: 'main',
  compareUrl: 'https://github.com/acme/widget/compare/main...ae5d4418f1f5c54e02c2ced82de2a5b66ba65753',
  missing: false,
  truncated: false,
  files: [
    {
      path: 'resources/js/Pages/Auth/Login.tsx',
      added: 1,
      deleted: 1,
      binary: false,
      truncated: false,
      hunks: [
        {
          header: '@@ -67,3 +67,3 @@ export default function Login() {',
          lines: [
            { kind: 'ctx', text: '<div>' },
            { kind: 'del', text: 'Username atau Email' },
            { kind: 'add', text: 'Username' }
          ]
        }
      ]
    },
    { path: 'public/logo.png', added: -1, deleted: -1, binary: true, truncated: false, hunks: [] }
  ]
};

const loginRow = () => screen.getByRole('button', { name: /Login\.tsx/ });

test('opens a small diff with numbered, labelled lines', () => {
  render(CardChanges, { diff });
  expect(screen.getByText('2 files')).toBeTruthy();
  expect(loginRow().getAttribute('aria-expanded')).toBe('true');
  expect(screen.getByText('Lines 67 to 69')).toBeTruthy();
  expect(screen.getByText('export default function Login() {')).toBeTruthy();

  const removed = screen.getByText('Username atau Email');
  const removedRow = removed.parentElement!;
  expect(removedRow.className).toContain('bg-red-50');
  expect(removedRow.textContent).toContain('68');
  expect(removedRow.textContent).toContain('Removed line');
  const addedRow = screen.getByText('Username').parentElement!;
  expect(addedRow.className).toContain('bg-green-50');
  expect(addedRow.textContent).toContain('Added line');
  expect(screen.getByRole('link', { name: 'View these changes on GitHub' }).getAttribute('href')).toBe(diff.compareUrl);
});

test('collapse all and expand all toggle every file', async () => {
  render(CardChanges, { diff });
  await fireEvent.click(screen.getByRole('button', { name: 'Collapse all' }));
  expect(loginRow().getAttribute('aria-expanded')).toBe('false');
  expect(screen.queryByText('Username atau Email')).toBeNull();
  await fireEvent.click(screen.getByRole('button', { name: 'Expand all' }));
  expect(loginRow().getAttribute('aria-expanded')).toBe('true');
  expect(screen.getByText('Binary file. Its contents are not shown.')).toBeTruthy();
  await fireEvent.click(loginRow());
  expect(loginRow().getAttribute('aria-expanded')).toBe('false');
});

test('wrap lines switches the code to wrapping', async () => {
  render(CardChanges, { diff });
  expect(screen.getByText('Username').className).toContain('whitespace-pre');
  expect(screen.getByText('Username').className).not.toContain('whitespace-pre-wrap');
  await fireEvent.click(screen.getByLabelText('Wrap lines'));
  expect(screen.getByText('Username').className).toContain('whitespace-pre-wrap');
});

test('a large diff starts closed and the file list opens a file', async () => {
  const many: DiffFile[] = Array.from({ length: 12 }, (_, i) => ({
    path: `src/file${i}.ts`,
    added: 1,
    deleted: 0,
    binary: false,
    truncated: false,
    hunks: [{ header: '@@ -1,0 +1,1 @@', lines: [{ kind: 'add', text: `line ${i}` }] }]
  }));
  render(CardChanges, { diff: { ...diff, files: many } });
  const nav = screen.getByRole('navigation', { name: 'Changed files' });
  expect(nav.textContent).toContain('src/');
  const row = screen.getByRole('button', { name: /file3\.ts/ });
  expect(row.getAttribute('aria-expanded')).toBe('false');
  const link = screen.getAllByRole('link').find((a) => a.getAttribute('href') === '#diff-file-3')!;
  await fireEvent.click(link);
  expect(row.getAttribute('aria-expanded')).toBe('true');
  expect(screen.getByText('line 3')).toBeTruthy();
});

test('warns when the diff is cut', () => {
  render(CardChanges, { diff: { ...diff, truncated: true } });
  expect(screen.getByText(/The diff is large, so it is cut/)).toBeTruthy();
});

test('says when the commit is not in the clone', () => {
  render(CardChanges, { diff: { ...diff, missing: true, files: [] } });
  expect(screen.getByText('Commit ae5d441 is not in the project clone yet.')).toBeTruthy();
});
