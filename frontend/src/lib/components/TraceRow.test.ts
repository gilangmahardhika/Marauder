import { fireEvent, render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import TraceRow from './TraceRow.svelte';
import type { TraceEvent } from '$lib/types';

const tool: TraceEvent = {
  id: 'evt_tool',
  ts: '2026-10-11T13:07:58Z',
  attemptId: 'att_1',
  kind: 'tool_call',
  payload: { name: 'read_file', ok: true, rejected: false, summary: 'line one\nline two' }
};

test('a row opens to its details and the raw event', async () => {
  render(TraceRow, { event: tool });
  const row = screen.getByRole('button', { name: /Read file/ });
  expect(row.getAttribute('aria-expanded')).toBe('false');
  expect(screen.getByText('2 lines')).toBeTruthy();
  await fireEvent.click(row);
  expect(row.getAttribute('aria-expanded')).toBe('true');
  expect(screen.getByText(/line one\s+line two/)).toBeTruthy();
  expect(screen.getByText('att_1')).toBeTruthy();
  const raw = screen.getByRole('button', { name: 'Raw event' });
  await fireEvent.click(raw);
  expect(raw.getAttribute('aria-expanded')).toBe('true');
  expect(screen.getByText(/"name": "read_file"/)).toBeTruthy();
});

test('an error row is red and a long message offers Show all', async () => {
  const message = `provider_error: ${'x'.repeat(5000)}`;
  const { container } = render(TraceRow, { event: { id: 'evt_err', ts: '2026-10-11T13:17:00Z', kind: 'job_error', payload: { message } } });
  expect(container.querySelector('li')?.className).toContain('bg-red-50');
  expect(screen.getByText('Error').className).toContain('text-red-800');
  await fireEvent.click(screen.getByRole('button', { name: /Error/ }));
  const more = screen.getByRole('button', { name: /Show all/ });
  await fireEvent.click(more);
  expect(screen.queryByRole('button', { name: /Show all/ })).toBeNull();
});

test('binary output stays hidden until asked', async () => {
  const message = `git exit 125: pax${'\u0000'.repeat(500)}header`;
  render(TraceRow, { event: { id: 'evt_tar', ts: '2026-10-11T13:17:00Z', kind: 'job_error', payload: { message } } });
  await fireEvent.click(screen.getByRole('button', { name: /Error/ }));
  expect(screen.getByText(/Binary output .* is hidden/)).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Show anyway' }));
  expect(screen.queryByText(/Binary output .* is hidden/)).toBeNull();
});
