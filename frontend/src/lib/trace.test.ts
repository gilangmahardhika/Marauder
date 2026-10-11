import { describe, expect, it } from 'vitest';
import { attemptOptions, filterEvents, kindOptions } from './trace';
import type { TraceEvent } from './types';

const events: TraceEvent[] = [
  { id: 'e1', ts: '2026-10-10T00:00:00Z', kind: 'transition', payload: { to: 'planned' } },
  { id: 'e2', ts: '2026-10-10T00:01:00Z', attemptId: 'att_1', kind: 'check', payload: { name: 'go test', exitCode: 1 } },
  { id: 'e3', ts: '2026-10-10T00:02:00Z', attemptId: 'att_2', kind: 'check', payload: { name: 'go test', exitCode: 0 } }
];

describe('trace filters', () => {
  it('filters by kind, attempt, and payload text', () => {
    const ids = (filter: { q: string; kind: string; attempt: string }) => filterEvents(events, filter).map((event) => event.id);
    expect(ids({ q: '', kind: 'check', attempt: '' })).toEqual(['e2', 'e3']);
    expect(ids({ q: '', kind: '', attempt: 'att_2' })).toEqual(['e3']);
    expect(ids({ q: '"exitCode":1', kind: '', attempt: '' })).toEqual(['e2']);
    expect(ids({ q: 'PLANNED', kind: '', attempt: '' })).toEqual(['e1']);
  });

  it('builds options from the events present', () => {
    expect(kindOptions(events).map((option) => option.label)).toEqual(['All kinds', 'check (2)', 'transition (1)']);
    expect(attemptOptions(events).map((option) => option.value)).toEqual(['', 'att_1', 'att_2']);
  });
});
