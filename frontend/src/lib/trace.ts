import { matchText, type Option } from './filter';
import type { TraceEvent } from './types';

/** `kind` and `attempt` are empty for every event. */
export type TraceFilter = { q: string; kind: string; attempt: string };

function counted(values: string[], all: string): Option[] {
  const counts = new Map<string, number>();
  for (const value of values) counts.set(value, (counts.get(value) ?? 0) + 1);
  const options = [...counts.keys()].sort().map((value) => ({ value, label: `${value} (${counts.get(value)})` }));
  return [{ value: '', label: all }, ...options];
}

export function kindOptions(events: TraceEvent[]) {
  return counted(
    events.map((event) => event.kind),
    'All kinds'
  );
}

export function attemptOptions(events: TraceEvent[]) {
  return counted(
    events.flatMap((event) => (event.attemptId ? [event.attemptId] : [])),
    'All attempts'
  );
}

export function filterEvents(events: TraceEvent[], filter: TraceFilter) {
  return events.filter(
    (event) =>
      (filter.kind === '' || event.kind === filter.kind) &&
      (filter.attempt === '' || event.attemptId === filter.attempt) &&
      (filter.q.trim() === '' || matchText([event.kind, event.attemptId, JSON.stringify(event.payload)], filter.q))
  );
}
