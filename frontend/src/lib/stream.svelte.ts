import { openStream } from './api';
import type { StreamEvent } from './types';

type Opener = (project: string, onEvent: (event: StreamEvent) => void, signal: AbortSignal) => Promise<void>;
type Handler = (event: StreamEvent) => void;

/** Reconnect delays after a failed connect, doubling between them. */
export const minDelay = 1000;
export const maxDelay = 30_000;

/**
 * One push stream per tab (RFC-113). `start` opens it for a project and keeps it open, reconnecting after
 * minDelay doubling to maxDelay. A reconnect dispatches `resync` before the server's full `cards` frame, so
 * listeners read what they may have missed. `wake` reconnects at once, for a tab that became visible.
 */
export function createLive(open: Opener = (project, onEvent, signal) => openStream(project, onEvent, signal)) {
  const state = $state({ connected: false });
  const handlers = new Set<Handler>();
  let generation = 0;
  let controller: AbortController | null = null;
  let wakeUp: (() => void) | null = null;

  function dispatch(event: StreamEvent) {
    for (const handler of handlers) handler(event);
  }

  function pause(ms: number) {
    return new Promise<void>((resolve) => {
      const timer = setTimeout(done, ms);
      function done() {
        clearTimeout(timer);
        wakeUp = null;
        resolve();
      }
      wakeUp = done;
    });
  }

  async function loop(project: string, gen: number, signal: AbortSignal) {
    let delay = minDelay;
    let opened = false;
    while (gen === generation) {
      let framed = false;
      try {
        await open(
          project,
          (event) => {
            if (gen !== generation) return;
            if (!framed) {
              framed = true;
              state.connected = true;
              delay = minDelay;
              if (opened) dispatch({ kind: 'resync' });
              opened = true;
            }
            if (event.kind !== 'done') dispatch(event);
          },
          signal
        );
      } catch {
        if (signal.aborted) return;
      }
      if (gen !== generation) return;
      state.connected = false;
      if (!framed) {
        await pause(delay);
        delay = Math.min(delay * 2, maxDelay);
      }
    }
  }

  function stop() {
    generation++;
    controller?.abort();
    controller = null;
    wakeUp?.();
    state.connected = false;
  }

  return {
    get connected() {
      return state.connected;
    },
    /** Opens the stream for project ('' for notifications only) and returns stop. */
    start(project: string) {
      stop();
      const gen = generation;
      controller = new AbortController();
      void loop(project, gen, controller.signal);
      return stop;
    },
    stop,
    wake() {
      wakeUp?.();
    },
    /** Adds a listener and returns its removal. */
    on(handler: Handler) {
      handlers.add(handler);
      return () => {
        handlers.delete(handler);
      };
    }
  };
}

export const live = createLive();
