import { fireEvent, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { expect, test, vi } from 'vitest';
import type { RefineEvent, RefineRequest } from '$lib/types';
import RefinePanel from './RefinePanel.svelte';

/** A stream the test drives: send pushes an event, end resolves, and an abort rejects like fetch. */
function controlled() {
  const state: { onEvent?: (event: RefineEvent) => void; end?: () => void; signal?: AbortSignal } = {};
  const stream = vi.fn(
    (_body: RefineRequest, onEvent: (event: RefineEvent) => void, signal: AbortSignal) =>
      new Promise<void>((resolve, reject) => {
        state.onEvent = onEvent;
        state.end = resolve;
        state.signal = signal;
        signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
      })
  );
  return {
    stream,
    state,
    async send(event: RefineEvent) {
      state.onEvent?.(event);
      await tick();
    }
  };
}

const props = { field: 'statement' as const, statement: 'Export the board.', acceptance: '- Works well.' };

test('deltas appear in order while streaming, then done shows the diff and Apply calls back', async () => {
  const fake = controlled();
  const onapply = vi.fn();
  render(RefinePanel, { ...props, stream: fake.stream, onapply });
  await fireEvent.input(screen.getByLabelText('What to improve (optional)'), { target: { value: ' Say who needs it ' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine' }));
  expect(fake.stream).toHaveBeenCalledWith(
    { field: 'statement', statement: 'Export the board.', acceptance: ['Works well.'], instructions: 'Say who needs it' },
    expect.any(Function),
    expect.any(AbortSignal)
  );
  expect(screen.getByText('Waiting for the implement model…')).toBeTruthy();

  await fake.send({ kind: 'start', modelId: 'model-a' });
  await fake.send({ kind: 'delta', text: 'Members ' });
  const live = screen.getByRole('region', { name: 'Refined statement so far' });
  expect(live.getAttribute('aria-live')).toBe('polite');
  expect(live.textContent).toBe('Members ');
  await fake.send({ kind: 'delta', text: 'export the board.' });
  expect(live.textContent).toBe('Members export the board.');
  expect(screen.getByText('Writing with model-a')).toBeTruthy();

  await fake.send({ kind: 'done', text: 'Members export the board.', modelId: 'model-a' });
  fake.state.end?.();
  expect(await screen.findByRole('region', { name: 'Refined statement' })).toBeTruthy();
  expect(screen.queryByRole('region', { name: 'Refined statement so far' })).toBeNull();
  await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
  expect(onapply).toHaveBeenCalledWith('Members export the board.');
});

test('Stop aborts the stream and changes nothing', async () => {
  const fake = controlled();
  const onapply = vi.fn();
  render(RefinePanel, { ...props, stream: fake.stream, onapply });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine' }));
  await fake.send({ kind: 'delta', text: 'Members ' });
  await fireEvent.click(screen.getByRole('button', { name: 'Stop' }));
  expect(fake.state.signal?.aborted).toBe(true);
  expect(await screen.findByText('Stopped. Nothing changed.')).toBeTruthy();
  expect(screen.queryByRole('region', { name: 'Refined statement so far' })).toBeNull();
  expect(screen.queryByRole('alert')).toBeNull();
  expect(onapply).not.toHaveBeenCalled();
  expect(screen.getByRole('button', { name: 'Refine' })).toBeTruthy();
});

test('an error event shows the message and Try again asks again', async () => {
  const fake = controlled();
  render(RefinePanel, { ...props, field: 'acceptance', stream: fake.stream, onapply: vi.fn() });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine' }));
  await fake.send({ kind: 'delta', text: 'One row' });
  await fake.send({ kind: 'error', code: 'provider_error', message: 'the refine request to the model failed' });
  fake.state.end?.();
  expect((await screen.findByRole('alert')).textContent).toContain('refine request to the model failed');
  await fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
  expect(fake.stream).toHaveBeenCalledTimes(2);
  expect(fake.stream.mock.calls[1][0].field).toBe('acceptance');
});

test('a rejected request shows its message and an empty statement sends nothing', async () => {
  const stream = vi.fn().mockRejectedValue(new Error('set a model for the implement route in Router settings to refine requirements'));
  const { rerender } = render(RefinePanel, { ...props, statement: '  ', stream, onapply: vi.fn() });
  await fireEvent.click(screen.getByRole('button', { name: 'Refine' }));
  expect(screen.getByRole('alert').textContent).toBe('Write a statement first, then refine it.');
  expect(stream).not.toHaveBeenCalled();

  await rerender({ ...props, stream, onapply: vi.fn() });
  await fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
  await vi.waitFor(() => expect(screen.getByRole('alert').textContent).toContain('implement route'));
});
