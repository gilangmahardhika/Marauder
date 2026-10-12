import { render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import Markdown from './Markdown.svelte';

test('HTML in a statement renders as text', () => {
  const { container } = render(Markdown, { text: '<script>alert(1)</script> <img src=x onerror=alert(1)>' });
  expect(container.querySelector('script, img')).toBeNull();
  expect(screen.getByText('<script>alert(1)</script> <img src=x onerror=alert(1)>')).toBeTruthy();
});

test('only allowed links become anchors', () => {
  const { container } = render(Markdown, { text: '[repo](https://github.com/acme/widget) and [bad](javascript:alert(1))' });
  const links = container.querySelectorAll('a');
  expect(links).toHaveLength(1);
  expect(links[0].getAttribute('href')).toBe('https://github.com/acme/widget');
  expect(links[0].getAttribute('target')).toBe('_blank');
  expect(links[0].getAttribute('rel')).toBe('noreferrer noopener');
  expect(container.textContent).toContain('bad (javascript:alert(1))');
});

test('headings start at h3 and code keeps its text', () => {
  const { container } = render(Markdown, { text: '# Scope\n\n```\nline one\n  line two\n```' });
  expect(screen.getByRole('heading', { level: 3, name: 'Scope' })).toBeTruthy();
  expect(container.querySelector('pre')?.textContent).toBe('line one\n  line two');
});

test('inline markup adds no stray spaces', () => {
  const { container } = render(Markdown, { text: 'a **b** c `d`e' });
  expect(container.querySelector('p')?.textContent).toBe('a b c de');
});

test('lists keep their start and nesting', () => {
  const { container } = render(Markdown, { text: '3. three\n4. four\n   - inner' });
  expect(container.querySelector('ol')?.getAttribute('start')).toBe('3');
  expect(container.querySelector('ol ul li')?.textContent).toBe('inner');
});

test('inline mode renders no paragraph', () => {
  const { container } = render(Markdown, { text: 'The **CSV** has one row', inline: true });
  expect(container.querySelector('p')).toBeNull();
  expect(container.querySelector('strong')?.textContent).toBe('CSV');
});
