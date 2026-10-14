import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mergeCards } from './board';
import { createLive, maxDelay } from './stream.svelte';
import type { Card, StreamEvent } from './types';

type Call = {
  project: string;
  emit: (event: StreamEvent) => void;
  end: () => void;
  fail: () => void;
  signal: AbortSignal;
};

function fakeOpen() {
  const calls: Call[] = [];
  const open = (project: string, onEvent: (event: StreamEvent) => void, signal: AbortSignal) =>
    new Promise<void>((resolve, reject) => {
      calls.push({ project, emit: onEvent, end: resolve, fail: () => reject(new Error('down')), signal });
    });
  return { calls, open };
}

const flush = () => vi.advanceTimersByTimeAsync(0);

describe('createLive', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('opens one stream for the project and passes its frames on', async () => {
    const { calls, open } = fakeOpen();
    const live = createLive(open);
    const seen: string[] = [];
    live.on((event) => seen.push(event.kind));
    live.start('proj_a');
    await flush();
    expect(calls.map((call) => call.project)).toEqual(['proj_a']);
    expect(live.connected).toBe(false);
    calls[0].emit({ kind: 'ready', unread: 2 });
    calls[0].emit({ kind: 'board', readyCount: 1, running: false, canRun: true });
    expect(live.connected).toBe(true);
    expect(seen).toEqual(['ready', 'board']);
  });

  it('waits 1 second, then twice as long, up to 30 seconds, after failed connects', async () => {
    const { calls, open } = fakeOpen();
    const live = createLive(open);
    live.start('');
    await flush();
    const waits: number[] = [];
    for (let i = 0; i < 7; i++) {
      calls[calls.length - 1].fail();
      await flush();
      const before = calls.length;
      let waited = 0;
      while (calls.length === before) {
        await vi.advanceTimersByTimeAsync(500);
        waited += 500;
      }
      waits.push(waited);
    }
    expect(waits).toEqual([1000, 2000, 4000, 8000, 16000, maxDelay, maxDelay]);
    live.stop();
  });

  it('reconnects at once after the server ends a stream, and says resync first', async () => {
    const { calls, open } = fakeOpen();
    const live = createLive(open);
    const seen: string[] = [];
    live.on((event) => seen.push(event.kind));
    live.start('proj_a');
    await flush();
    calls[0].emit({ kind: 'ready', unread: 0 });
    calls[0].emit({ kind: 'done', reconnect: true });
    calls[0].end();
    await flush();
    expect(calls).toHaveLength(2);
    expect(live.connected).toBe(false);
    calls[1].emit({ kind: 'ready', unread: 0 });
    calls[1].emit({ kind: 'cards', full: true, cards: [], removed: [] });
    expect(seen).toEqual(['ready', 'resync', 'ready', 'cards']);
    live.stop();
  });

  it('wake reconnects at once during a wait', async () => {
    const { calls, open } = fakeOpen();
    const live = createLive(open);
    live.start('');
    await flush();
    calls[0].fail();
    await flush();
    expect(calls).toHaveLength(1);
    live.wake();
    await flush();
    expect(calls).toHaveLength(2);
    live.stop();
  });

  it('stop aborts the stream and opens no other', async () => {
    const { calls, open } = fakeOpen();
    const live = createLive(open);
    const stop = live.start('proj_a');
    await flush();
    stop();
    expect(calls[0].signal.aborted).toBe(true);
    calls[0].fail();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(calls).toHaveLength(1);
  });

  it('a new project closes the old stream and ignores its frames', async () => {
    const { calls, open } = fakeOpen();
    const live = createLive(open);
    const seen: string[] = [];
    live.on((event) => seen.push(event.kind));
    live.start('proj_a');
    await flush();
    live.start('proj_b');
    await flush();
    calls[0].emit({ kind: 'ready', unread: 0 });
    expect(calls.map((call) => call.project)).toEqual(['proj_a', 'proj_b']);
    expect(calls[0].signal.aborted).toBe(true);
    expect(seen).toEqual([]);
    live.stop();
  });
});

describe('mergeCards', () => {
  const card = (id: string, title = id) => ({ id, title }) as Card;

  it('replaces the list on a full frame', () => {
    expect(mergeCards([card('a')], { full: true, cards: [card('b')], removed: [] })).toEqual([card('b')]);
  });

  it('updates changed cards in place, adds new ones at the end, and drops removed ones', () => {
    const next = mergeCards([card('a'), card('b'), card('c')], {
      full: false,
      cards: [card('b', 'B2'), card('d')],
      removed: ['a']
    });
    expect(next).toEqual([card('b', 'B2'), card('c'), card('d')]);
  });
});
