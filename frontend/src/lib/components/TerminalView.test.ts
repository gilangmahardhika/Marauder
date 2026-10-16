import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import TerminalView from './TerminalView.svelte';
import type { PendingReply } from '$lib/terminal';
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

test('shows the reply being written as the last entry, with a caret, and marks the log busy', async () => {
  const pending: PendingReply = { attemptId: 'att_1', turn: 2, role: 'reviewer', modelId: 'model-b', text: '', since: '2026-10-12T10:00:05Z' };
  const view = render(TerminalView, { props: { events: [tool(1)], pending } });
  const log = screen.getByRole('log', { name: 'Terminal output, oldest first' });
  const row = screen.getByTestId('pending-reply');
  expect(log.lastElementChild).toBe(row);
  expect(row.textContent).toContain('model  reviewer  model-b  writing…');
  expect(row.textContent).toContain('Waiting for the model…');
  expect(row.querySelector('[aria-hidden="true"]')?.textContent).toBe('▍');
  expect(log.getAttribute('aria-busy')).toBe('true');

  await view.rerender({ events: [tool(1)], pending: { ...pending, text: 'The diff \u001b[1mlooks\u001b[0m right.' } });
  expect(screen.getByTestId('pending-reply').textContent).toContain('The diff looks right.');
  expect(screen.getByTestId('pending-reply').textContent).not.toContain('Waiting for the model…');

  await view.rerender({ events: [tool(1)], pending: null });
  expect(screen.queryByTestId('pending-reply')).toBeNull();
  expect(log.getAttribute('aria-busy')).toBeNull();
});

test('renders an edit card for tool_call args with old and new', () => {
  render(TerminalView, {
    props: {
      events: [
        {
          id: 'e1',
          seq: 1,
          ts: '2026-10-12T10:00:01Z',
          attemptId: '',
          kind: 'tool_call',
          payload: {
            name: 'edit_file',
            ok: true,
            summary: 'wrote src/a.ts',
            args: { path: 'src/a.ts', bytes: 20, lines: 2, old: 'const x = 1;\n', new: 'const x = 2;\n' }
          }
        }
      ]
    }
  });
  expect(screen.getByTestId('terminal-edit-card').textContent).toContain('src/a.ts');
  expect(screen.getByTestId('terminal-edit-card').textContent).toContain('const x = 2;');
});

test('shows live thinking above the reply being written', () => {
  render(TerminalView, {
    props: {
      events: [tool(1)],
      thinking: {
        attemptId: 'att_1',
        turn: 1,
        text: 'Building the foundation first.',
        since: '2026-10-12T10:00:05Z'
      }
    }
  });
  const row = screen.getByTestId('pending-thinking');
  expect(row.textContent).toContain('thinking');
  expect(row.textContent).toContain('Building the foundation first.');
});
