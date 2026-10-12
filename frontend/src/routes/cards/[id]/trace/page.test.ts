import { render, screen } from '@testing-library/svelte';
import { readable } from 'svelte/store';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { shell } from '$lib/shell.svelte';
import type { TraceAttempt, TraceEvent, TraceStreamEvent } from '$lib/types';

const api = vi.hoisted(() => ({ getTrace: vi.fn(), getCard: vi.fn(), streamTrace: vi.fn() }));
const route = vi.hoisted(() => ({ search: '' }));

vi.mock('$app/stores', () => ({
  page: {
    subscribe: (run: (value: unknown) => void) =>
      readable({ params: { id: 'card_1' }, url: new URL(`http://127.0.0.1:4173/cards/card_1/trace${route.search}`) }).subscribe(run)
  }
}));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api', () => ({
  ApiError: class ApiError extends Error {},
  unreachable: () => 'API unreachable',
  getTrace: api.getTrace,
  getCard: api.getCard,
  streamTrace: api.streamTrace
}));

import TracePage from './+page.svelte';

const attempt: TraceAttempt = {
  id: 'att_1',
  n: 1,
  taskClass: 'implement',
  modelId: 'model-a',
  fallbackModelId: '',
  inputTokens: 900,
  outputTokens: 100,
  maxTurns: 12,
  commitSha: '',
  reviewModelId: '',
  reviewVerdict: '',
  reviewIndependent: null,
  startedAt: '2026-10-12T10:00:00Z',
  endedAt: '',
  outcome: ''
};

const first: TraceEvent = {
  id: 'e1',
  seq: 1,
  ts: '2026-10-12T10:00:01Z',
  attemptId: 'att_1',
  kind: 'model_call',
  payload: { role: 'implementer', modelId: 'model-a', text: 'I will read the login page.', inputTokens: 900, outputTokens: 100 }
};

const second: TraceEvent = {
  id: 'e2',
  seq: 2,
  ts: '2026-10-12T10:00:04Z',
  attemptId: 'att_1',
  kind: 'tool_call',
  payload: { name: 'read_file', ok: true, summary: 'a\nb', args: { path: 'src/login.ts' } }
};

/** One open trace stream: the test sends its frames and ends it. */
type OpenStream = {
  since: string | undefined;
  signal: AbortSignal;
  emit: (event: TraceStreamEvent) => void;
  end: () => void;
  fail: (error: unknown) => void;
};
let streams: OpenStream[] = [];

async function send(stream: OpenStream, ...events: TraceStreamEvent[]) {
  for (const event of events) stream.emit(event);
  await vi.advanceTimersByTimeAsync(40);
}

function setVisibility(state: 'visible' | 'hidden') {
  Object.defineProperty(document, 'visibilityState', { value: state, configurable: true });
  document.dispatchEvent(new Event('visibilitychange'));
}

beforeEach(() => {
  vi.useFakeTimers({
    toFake: ['setInterval', 'clearInterval', 'setTimeout', 'clearTimeout', 'Date', 'requestAnimationFrame', 'cancelAnimationFrame']
  });
  streams = [];
  api.streamTrace.mockImplementation(
    (_id: string, since: string | undefined, onEvent: (event: TraceStreamEvent) => void, signal: AbortSignal) =>
      new Promise<void>((resolve, reject) => {
        streams.push({ since, signal, emit: onEvent, end: resolve, fail: reject });
      })
  );
  vi.setSystemTime(new Date('2026-10-12T10:00:10Z'));
  Element.prototype.scrollIntoView = vi.fn();
  shell.projectId = 'proj_default';
  api.getCard.mockResolvedValue({ id: 'card_1', title: 'Fix login', status: 'in_progress' });
});

afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
  shell.projectId = '';
  route.search = '';
  setVisibility('visible');
});

test('streams while live: merges stored events, shows the reply as it is written, and stops when the card is done', async () => {
  route.search = '?view=terminal';
  api.getTrace.mockResolvedValue({ attempts: [attempt], events: [first], live: true });
  render(TracePage);
  await vi.advanceTimersByTimeAsync(0);
  expect(api.getTrace).toHaveBeenCalledTimes(1);
  expect(api.streamTrace).toHaveBeenCalledTimes(1);
  expect(api.streamTrace.mock.calls[0][0]).toBe('card_1');
  expect(streams[0].since).toBe(first.ts);
  expect(screen.getByText('Live').parentElement?.getAttribute('title')).toBe('This page shows output as it arrives');
  const log = screen.getByRole('log', { name: 'Terminal output, oldest first' });

  const stream = streams[0];
  await send(stream, { kind: 'trace', attempts: [attempt], events: [first, second], live: true });
  expect(log.textContent).toContain('$ read_file  src/login.ts');
  expect(log.textContent?.match(/I will read the login page\./g)?.length).toBe(1);

  await send(stream, { kind: 'turn', attemptId: 'att_1', turn: 1, role: 'implementer', modelId: 'model-a' });
  expect(log.textContent).toContain('model  implementer  model-a  writing…');
  expect(log.textContent).toContain('Waiting for the model…');
  expect(log.getAttribute('aria-busy')).toBe('true');
  expect(screen.getByRole('region', { name: 'Now running' }).textContent).toContain('The model is writing…');

  await send(stream, { kind: 'delta', attemptId: 'att_1', turn: 1, text: 'Now I ' }, { kind: 'delta', attemptId: 'att_1', turn: 1, text: 'edit it.' });
  expect(log.textContent).toContain('Now I edit it.');
  expect(log.textContent).not.toContain('Waiting for the model…');

  await send(stream, { kind: 'reset', attemptId: 'att_1', turn: 1 });
  expect(log.textContent).not.toContain('Now I edit it.');
  await send(stream, { kind: 'delta', attemptId: 'att_1', turn: 1, text: 'Edit done.' }, { kind: 'turn_end', attemptId: 'att_1', turn: 1 });
  expect(log.textContent).toContain('Edit done.');

  const stored: TraceEvent = {
    id: 'e3',
    seq: 3,
    ts: '2026-10-12T10:00:09Z',
    attemptId: 'att_1',
    kind: 'model_call',
    payload: { role: 'implementer', modelId: 'model-a', text: 'Edit done.', inputTokens: 1000, outputTokens: 20 }
  };
  await send(stream, { kind: 'trace', attempts: [attempt], events: [stored], live: true });
  expect(log.textContent).not.toContain('writing…');
  expect(log.textContent?.match(/Edit done\./g)?.length).toBe(1);
  expect(log.getAttribute('aria-busy')).toBeNull();

  await send(
    stream,
    { kind: 'trace', attempts: [{ ...attempt, endedAt: '2026-10-12T10:00:20Z', outcome: 'checks_passed' }], events: [], live: false },
    { kind: 'done', live: false }
  );
  stream.end();
  await vi.advanceTimersByTimeAsync(10000);
  expect(screen.queryByText('Live')).toBeNull();
  expect(shell.message).toBe('Attempt 1 ended: Checks passed');
  expect(api.getTrace).toHaveBeenCalledTimes(1);
  expect(api.streamTrace).toHaveBeenCalledTimes(1);
});

test('a filter that hides model calls hides the reply being written', async () => {
  route.search = '?view=terminal&kind=tool_call';
  api.getTrace.mockResolvedValue({ attempts: [attempt], events: [first, second], live: true });
  render(TracePage);
  await vi.advanceTimersByTimeAsync(0);
  await send(streams[0], { kind: 'turn', attemptId: 'att_1', turn: 1, role: 'implementer', modelId: 'model-a' }, { kind: 'delta', attemptId: 'att_1', turn: 1, text: 'Hidden reply.' });
  const log = screen.getByRole('log', { name: 'Terminal output, oldest first' });
  expect(log.textContent).not.toContain('Hidden reply.');
  expect(log.getAttribute('aria-busy')).toBeNull();
});

test('does not poll a card that is not live', async () => {
  api.getTrace.mockResolvedValue({ attempts: [{ ...attempt, endedAt: '2026-10-12T10:00:20Z', outcome: 'checks_passed' }], events: [first], live: false });
  render(TracePage);
  await vi.advanceTimersByTimeAsync(10000);
  expect(api.getTrace).toHaveBeenCalledTimes(1);
  expect(api.streamTrace).not.toHaveBeenCalled();
  expect(screen.queryByRole('region', { name: 'Now running' })).toBeNull();
});

test('the terminal view renders the log and keeps the filters', async () => {
  route.search = '?view=terminal&kind=tool_call';
  api.getTrace.mockResolvedValue({ attempts: [{ ...attempt, endedAt: '2026-10-12T10:00:20Z', outcome: 'checks_passed' }], events: [first, second], live: false });
  render(TracePage);
  await vi.advanceTimersByTimeAsync(0);
  const log = screen.getByRole('log', { name: 'Terminal output, oldest first' });
  expect(log.textContent).toContain('$ read_file  src/login.ts');
  expect(log.textContent).toContain('a\nb');
  expect(log.textContent).not.toContain('I will read the login page.');
  expect(screen.getByRole('button', { name: 'Terminal' }).getAttribute('aria-pressed')).toBe('true');
});

test('when the stream fails the page polls, then tries the stream again after 5 seconds', async () => {
  api.getTrace.mockResolvedValue({ attempts: [attempt], events: [first], live: true });
  render(TracePage);
  await vi.advanceTimersByTimeAsync(0);
  expect(api.streamTrace).toHaveBeenCalledTimes(1);
  streams[0].fail(new Error('network'));
  await vi.advanceTimersByTimeAsync(0);

  await vi.advanceTimersByTimeAsync(3000);
  expect(api.getTrace).toHaveBeenCalledTimes(2);
  expect(api.getTrace).toHaveBeenLastCalledWith('card_1', first.ts);
  expect(api.streamTrace).toHaveBeenCalledTimes(1);

  await vi.advanceTimersByTimeAsync(2000);
  expect(api.streamTrace).toHaveBeenCalledTimes(2);
  await vi.advanceTimersByTimeAsync(9000);
  expect(api.getTrace).toHaveBeenCalledTimes(2);
});

test('the terminal view polls every second while the stream is down', async () => {
  route.search = '?view=terminal';
  api.getTrace.mockResolvedValue({ attempts: [attempt], events: [first], live: true });
  render(TracePage);
  await vi.advanceTimersByTimeAsync(0);
  streams[0].end();
  await vi.advanceTimersByTimeAsync(0);
  await vi.advanceTimersByTimeAsync(1000);
  expect(api.getTrace).toHaveBeenCalledTimes(2);
  await vi.advanceTimersByTimeAsync(1000);
  expect(api.getTrace).toHaveBeenCalledTimes(3);
});

test('closes the stream while the tab is hidden, and connects again on reconnect and resync', async () => {
  api.getTrace.mockResolvedValue({ attempts: [attempt], events: [first], live: true });
  render(TracePage);
  await vi.advanceTimersByTimeAsync(0);
  expect(streams).toHaveLength(1);

  setVisibility('hidden');
  await vi.advanceTimersByTimeAsync(0);
  expect(streams[0].signal.aborted).toBe(true);
  await vi.advanceTimersByTimeAsync(10000);
  expect(streams).toHaveLength(1);
  expect(api.getTrace).toHaveBeenCalledTimes(1);

  setVisibility('visible');
  await vi.advanceTimersByTimeAsync(0);
  expect(streams).toHaveLength(2);
  expect(streams[1].since).toBe(first.ts);

  await send(streams[1], { kind: 'done', reconnect: true });
  streams[1].end();
  await vi.advanceTimersByTimeAsync(0);
  expect(streams).toHaveLength(3);

  await send(streams[2], { kind: 'resync' });
  streams[2].end();
  await vi.advanceTimersByTimeAsync(0);
  expect(streams).toHaveLength(4);
  expect(api.getTrace).toHaveBeenCalledTimes(2);
  expect(api.getTrace).toHaveBeenLastCalledWith('card_1', undefined);
});
