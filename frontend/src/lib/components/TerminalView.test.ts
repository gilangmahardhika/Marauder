import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import TerminalView from './TerminalView.svelte';
import type { TraceEvent } from '$lib/types';

function tool(n: number, summary = `out ${n}`): TraceEvent {
  return {
    id: `e${n}`,
    seq: n,
    ts: '2026-10-12T10:00:01Z',
    attemptId: '',
    kind: 'tool_call',
    payload: { name: 'read_file', ok: true, summary, args: { path: `f${n}.ts` } }
  };
}

test('renders each event as a command head over its output', () => {
  render(TerminalView, {
    props: {
      events: [
        { id: 'm1', seq: 1, ts: '2026-10-12T10:00:00Z', attemptId: '', kind: 'model_call', payload: { role: 'implementer', modelId: 'model-a', text: 'Reading.' } },
        tool(2, 'export const a = 1;')
      ]
    }
  });
  const log = screen.getByRole('log', { name: 'Terminal output, oldest first' });
  expect(log.textContent).toContain('model  implementer  model-a');
  expect(log.textContent).toContain('Reading.');
  expect(screen.getByText('$ read_file f2.ts')).toBeTruthy();
  expect(screen.getByText('export const a = 1;')).toBeTruthy();
});

test('folds long output until Show all is pressed', async () => {
  const output = Array.from({ length: 250 }, (_, i) => `row ${i}`).join('\n');
  render(TerminalView, { props: { events: [tool(1, output)] } });
  expect(screen.queryByText(/row 249/)).toBeNull();
  await fireEvent.click(screen.getByRole('button', { name: 'Show all 250 lines' }));
  expect(screen.getByText(/row 249/)).toBeTruthy();
  expect(screen.queryByRole('button', { name: /Show all/ })).toBeNull();
});

test('shows the newest 300 events and pages in earlier ones', async () => {
  render(TerminalView, { props: { events: Array.from({ length: 350 }, (_, i) => tool(i + 1)) } });
  expect(screen.queryByText('$ read_file f50.ts')).toBeNull();
  expect(screen.getByText('$ read_file f51.ts')).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Show earlier events (50 hidden)' }));
  expect(screen.getByText('$ read_file f1.ts')).toBeTruthy();
  expect(screen.queryByRole('button', { name: /Show earlier events/ })).toBeNull();
});

test('scrolling up stops following and Jump to latest follows again', async () => {
  render(TerminalView, { props: { events: [tool(1), tool(2)] } });
  expect(screen.queryByRole('button', { name: 'Jump to latest' })).toBeNull();
  const log = screen.getByRole('log');
  Object.defineProperty(log, 'scrollHeight', { configurable: true, value: 1000 });
  Object.defineProperty(log, 'clientHeight', { configurable: true, value: 200 });
  log.scrollTop = 0;
  await fireEvent.scroll(log);
  await fireEvent.click(screen.getByRole('button', { name: 'Jump to latest' }));
  expect(screen.queryByRole('button', { name: 'Jump to latest' })).toBeNull();
  expect(log.scrollTop).toBe(1000);
});
