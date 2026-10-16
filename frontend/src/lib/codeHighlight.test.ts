import { expect, test } from 'vitest';
import { escapeHtml, highlightLine, langFromPath } from './codeHighlight';

test('langFromPath maps common extensions', () => {
  expect(langFromPath('src/app.ts')).toBe('typescript');
  expect(langFromPath('main.go')).toBe('go');
  expect(langFromPath('styles.css')).toBe('css');
  expect(langFromPath('README.md')).toBe('markdown');
  expect(langFromPath('unknown.xyz')).toBe('plaintext');
});

test('highlightLine escapes HTML and marks keywords', () => {
  expect(escapeHtml('<b>&')).toBe('&lt;b&gt;&amp;');
  const html = highlightLine('const x = 1;', 'typescript');
  expect(html).toContain('hl-keyword');
  expect(html).toContain('hl-number');
  expect(html).not.toContain('<script');
});
