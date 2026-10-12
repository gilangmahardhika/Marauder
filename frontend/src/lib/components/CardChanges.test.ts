import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import CardChanges from './CardChanges.svelte';
import type { CardDiff } from '$lib/types';

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
          header: '@@ -67,7 +67,7 @@',
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

test('lists files with counts and opens a diff', async () => {
  render(CardChanges, { diff });
  expect(screen.getByText('2 files')).toBeTruthy();
  expect(screen.getByText('+1 -1')).toBeTruthy();
  expect(screen.getByText('binary')).toBeTruthy();
  const row = screen.getByRole('button', { name: /Login\.tsx/ });
  expect(row.getAttribute('aria-expanded')).toBe('false');
  expect(screen.queryByText('-Username atau Email')).toBeNull();
  await fireEvent.click(row);
  expect(row.getAttribute('aria-expanded')).toBe('true');
  expect(screen.getByText('-Username atau Email').className).toContain('bg-red-50');
  expect(screen.getByText('+Username').className).toContain('bg-green-50');
  expect(screen.getByRole('link', { name: 'View these changes on GitHub' }).getAttribute('href')).toBe(diff.compareUrl);
});

test('warns when the diff is cut', () => {
  render(CardChanges, { diff: { ...diff, truncated: true } });
  expect(screen.getByText(/The diff is large, so it is cut/)).toBeTruthy();
});

test('says when the commit is not in the clone', () => {
  render(CardChanges, { diff: { ...diff, missing: true, files: [] } });
  expect(screen.getByText('Commit ae5d441 is not in the project clone yet.')).toBeTruthy();
});
