import { describe as group, expect, it } from 'vitest';
import { attemptOptions, buildTimeline, cleanText, describe, filterEvents, kindOptions, mergeEvents, outcomeLabel, summarize } from './trace';
import type { TraceAttempt, TraceEvent } from './types';

const events: TraceEvent[] = [
  { id: 'e1', ts: '2026-10-10T00:00:00Z', kind: 'transition', payload: { from: 'ready', to: 'in_progress', evidence: 'recorded' } },
  { id: 'e2', ts: '2026-10-10T00:01:00Z', attemptId: 'att_1', kind: 'tool_call', payload: { name: 'run_local_check', ok: true, rejected: false, summary: 'exit 1\nFAIL' } },
  { id: 'e3', ts: '2026-10-10T00:02:00Z', attemptId: 'att_2', kind: 'tool_call', payload: { name: 'run_local_check', ok: true, rejected: false, summary: 'exit 0\nok' } }
];

const attempt = (id: string, n: number, extra: Partial<TraceAttempt> = {}): TraceAttempt => ({
  id,
  n,
  taskClass: 'implement',
  modelId: 'model-a',
  fallbackModelId: '',
  inputTokens: 0,
  outputTokens: 0,
  commitSha: '',
  reviewModelId: '',
  reviewVerdict: '',
  reviewIndependent: null,
  startedAt: '',
  endedAt: '',
  outcome: '',
  ...extra
});

group('trace filters', () => {
  it('filters by kind, attempt, payload text, and the words on the row', () => {
    const ids = (filter: { q: string; kind: string; attempt: string }) => filterEvents(events, filter).map((event) => event.id);
    expect(ids({ q: '', kind: 'tool_call', attempt: '' })).toEqual(['e2', 'e3']);
    expect(ids({ q: '', kind: '', attempt: 'att_2' })).toEqual(['e3']);
    expect(ids({ q: 'exit 1', kind: '', attempt: '' })).toEqual(['e2']);
    expect(ids({ q: 'IN PROGRESS', kind: '', attempt: '' })).toEqual(['e1']);
    expect(ids({ q: 'run local check', kind: '', attempt: '' })).toEqual(['e2', 'e3']);
  });

  it('builds readable options from the events present', () => {
    expect(kindOptions(events).map((option) => option.label)).toEqual(['All kinds', 'Status change (1)', 'Tool call (2)']);
    const attempts = [attempt('att_2', 2), attempt('att_1', 1)];
    expect(attemptOptions(events, attempts).map((option) => [option.value, option.label])).toEqual([
      ['', 'All attempts'],
      ['att_1', 'Attempt 1 (1)'],
      ['att_2', 'Attempt 2 (1)']
    ]);
  });
});

group('describe', () => {
  const words = (kind: string, payload: unknown) => describe({ id: 'x', ts: '2026-10-10T00:00:00Z', kind, payload });

  it('puts every kind into words', () => {
    expect(words('transition', { from: 'human_review', to: 'approved', evidence: 'sha ae5d4418f1f5c54e02c2ced82de2a5b66ba65753, approver octocat' })).toMatchObject({
      label: 'Waiting for review → Approved',
      detail: 'sha ae5d441, approver octocat',
      tone: 'green'
    });
    expect(words('transition', { from: 'in_progress', to: 'blocked', evidence: 'attempt_cap' })).toMatchObject({ detail: 'Out of attempts', tone: 'red' });
    expect(words('transition_rejected', { from: 'pr_open', to: 'done', evidence: 'illegal' })).toMatchObject({ tone: 'amber' });
    expect(words('model_call', { role: 'implementer', modelId: 'm', stopReason: 'tool_use', inputTokens: 1616, outputTokens: 74 })).toMatchObject({
      label: 'Model call',
      detail: 'm · tool use',
      meta: '1,616 in / 74 out'
    });
    expect(words('model_call', { role: 'reviewer' }).label).toBe('Reviewer model');
    expect(words('tool_call', { name: 'edit_file', ok: true, summary: 'wrote src/a.ts' })).toMatchObject({ label: 'Edit file', detail: 'src/a.ts' });
    expect(words('tool_call', { name: 'search_files', ok: true, summary: 'a:1: x\nb:2: y\n' }).detail).toBe('2 matches');
    expect(words('tool_call', { name: 'read_file', ok: true, summary: 'one\ntwo\nthree' }).detail).toBe('3 lines');
    expect(words('tool_call', { name: 'verdict', ok: true, summary: 'recommend_approve' })).toMatchObject({ detail: 'Recommend approve', tone: 'green' });
    expect(words('tool_call', { name: 'list_files', ok: false, rejected: true, summary: 'not in profile' })).toMatchObject({ tone: 'amber', detail: 'Refused: not in profile' });
    expect(words('tool_call', { name: 'conventions_context', ok: true, summary: '2 rules and skills', items: [{ kind: 'rule', id: 'r1', version: 3, hash: 'abcdef0123456789' }] })).toMatchObject({
      label: 'Rules and skills',
      text: 'rule r1 v3 abcdef012345'
    });
    expect(words('job_error', { message: 'git exit 125: pax\u0000\u0000header\nmore' })).toMatchObject({ label: 'Error', detail: 'git exit 125: paxheader', tone: 'red' });
    expect(words('reviewers_requested', { reviewers: ['a', 'b'], ok: true }).detail).toBe('@a, @b');
    expect(words('health', { passed: false, rollbackExit: 0 }).tone).toBe('red');
    expect(words('verify', { exitCode: 0 }).detail).toBe('exit 0');
    expect(words('budget_stop', {}).tone).toBe('red');
  });

  it('says how long saved instructions are, or that they were cleared', () => {
    const saved = (length: number) => describe({ id: 'e', ts: '', kind: 'instructions_saved', payload: { length } });
    expect(saved(1200)).toMatchObject({ label: 'Instructions saved', detail: '1,200 characters' });
    expect(saved(0).detail).toBe('Cleared');
  });

  it('shows what each tool was asked for and what the model said', () => {
    const tool = (name: string, args: unknown, summary = '') => words('tool_call', { name, ok: true, summary, args });
    expect(tool('read_file', { path: 'src/x.ts' }, 'a\nb\nc')).toMatchObject({ label: 'Read file', detail: 'src/x.ts · 3 lines' });
    expect(tool('read_linked_file', { project: 'proj_api', path: 'go.mod' }, 'module x')).toMatchObject({ detail: 'proj_api: go.mod · 1 line' });
    expect(tool('list_files', { path: 'src' }, 'a\nb')).toMatchObject({ detail: 'src · 2 paths' });
    expect(tool('search_files', { text: 'useSession' }, '')).toMatchObject({ detail: '"useSession" · No matches' });
    expect(tool('edit_file', { path: 'src/a.ts', bytes: 300, lines: 12 }, 'wrote src/a.ts')).toMatchObject({ detail: 'src/a.ts · 12 lines', text: '' });
    expect(tool('run_local_check', { command: 'npm test' }, 'exit 0\nok')).toMatchObject({ detail: 'exit 0 · npm test', tone: 'green' });
    expect(tool('commit', { message: 'Fix login\n\nUses the session.' }, 'committed')).toMatchObject({ detail: 'Fix login', text: 'Fix login\n\nUses the session.' });
    expect(tool('comment_pr', { body: 'Looks fine.' }, 'commented')).toMatchObject({ detail: 'Looks fine.', text: 'Looks fine.' });
    expect(tool('verdict', { body: 'Meets REQ-001.' }, 'recommend_approve')).toMatchObject({ detail: 'Recommend approve', text: 'Meets REQ-001.' });
    expect(tool('read_file', { path: 'src/x.ts', offset: 10, limit: 3 }, '10|a\n11|b\n12|c\nlines 10-12 of 2500\nmore: read from line 13')).toMatchObject({
      detail: 'src/x.ts from line 10 · lines 10-12 of 2,500'
    });
    expect(tool('replace_in_file', { path: 'src/a.ts', oldBytes: 10, newBytes: 12 }, 'replaced 2 occurrences in src/a.ts')).toMatchObject({
      label: 'Replace in file',
      detail: 'src/a.ts · 2 occurrences',
      text: ''
    });
    expect(tool('move_file', { from: 'a.ts', to: 'b/a.ts' }, 'moved a.ts to b/a.ts')).toMatchObject({ label: 'Move file', detail: 'a.ts -> b/a.ts' });
    expect(tool('delete_file', { path: 'old.ts' }, 'deleted old.ts')).toMatchObject({ label: 'Delete file', detail: 'old.ts' });
    expect(tool('search_files', { text: 'login', glob: '*.ts', path: 'src' }, 'src/a.ts:1: login')).toMatchObject({ detail: '"login" in src *.ts · 1 match' });
    expect(tool('list_files', { path: '.', glob: '**/*.go' }, 'a.go')).toMatchObject({ detail: '. **/*.go · 1 path' });
    expect(tool('run_command', { argv: ['go', 'test', './...'] }, 'exit 1\nFAIL')).toMatchObject({ label: 'Run command', detail: 'exit 1 · go test ./...', tone: 'red' });
    expect(words('tool_call', { name: 'read_file', ok: false, summary: 'error: open x: no such file', args: { path: 'x' } })).toMatchObject({ tone: 'red' });
    expect(words('model_call', { role: 'implementer', modelId: 'm', text: '\nI will read the login page first.\nThen edit it.' })).toMatchObject({
      detail: 'I will read the login page first.',
      text: 'I will read the login page first.\nThen edit it.'
    });
  });

  it('reads an unknown kind and a missing payload as words', () => {
    expect(words('brand_new_kind', null)).toMatchObject({ label: 'Brand new kind', detail: '', tone: 'default' });
    expect(words('tool_call', 'not an object')).toMatchObject({ label: '' });
  });
});

group('mergeEvents', () => {
  it('keeps one row per id and orders by time, then insert order', () => {
    const shown: TraceEvent[] = [
      { id: 'a', seq: 1, ts: '2026-10-12T10:00:00Z', kind: 'transition', payload: {} },
      { id: 'b', seq: 2, ts: '2026-10-12T10:00:05Z', kind: 'model_call', payload: {} }
    ];
    const polled: TraceEvent[] = [
      { id: 'b', seq: 2, ts: '2026-10-12T10:00:05Z', kind: 'model_call', payload: {} },
      { id: 'd', seq: 4, ts: '2026-10-12T10:00:05Z', kind: 'tool_call', payload: {} },
      { id: 'c', seq: 3, ts: '2026-10-12T10:00:05Z', kind: 'tool_call', payload: {} },
      { id: 'e', seq: 5, ts: '2026-10-12T10:00:09Z', kind: 'tool_call', payload: {} }
    ];
    expect(mergeEvents(shown, polled).map((event) => event.id)).toEqual(['a', 'b', 'c', 'd', 'e']);
    expect(mergeEvents(shown, [])).toHaveLength(2);
  });
});

group('cleanText', () => {
  it('strips control characters and reports the size', () => {
    const raw = `pax${'\u0000'.repeat(100)}header\n\tline`;
    const clean = cleanText(raw, 4000);
    expect(clean.full).toBe('paxheader\n\tline');
    expect(clean.binary).toBe(true);
    expect(clean.bytes).toBe(raw.length);
  });

  it('cuts long text at the cap', () => {
    const clean = cleanText('x'.repeat(5000), 4000);
    expect(clean.cut).toBe(true);
    expect(clean.text.length).toBe(4000);
    expect(clean.full.length).toBe(5000);
    expect(clean.binary).toBe(false);
  });
});

group('buildTimeline', () => {
  const at = (s: number) => `2026-10-10T00:00:${String(s).padStart(2, '0')}Z`;
  const timeline: TraceEvent[] = [
    { id: 't1', ts: at(0), kind: 'transition', payload: { from: 'ready', to: 'in_progress' } },
    { id: 'm1', ts: at(1), attemptId: 'att_1', kind: 'model_call', payload: { role: 'implementer', inputTokens: 100, outputTokens: 10 } },
    { id: 'c1', ts: at(2), attemptId: 'att_1', kind: 'tool_call', payload: { name: 'edit_file', ok: true, summary: 'wrote a' } },
    { id: 't2', ts: at(3), kind: 'transition', payload: { from: 'in_progress', to: 'self_check' } },
    { id: 't3', ts: at(4), kind: 'transition', payload: { from: 'pr_open', to: 'agent_review' } },
    { id: 'r1', ts: at(5), attemptId: 'att_1', kind: 'model_call', payload: { role: 'reviewer', inputTokens: 50, outputTokens: 5 } },
    { id: 'r2', ts: at(6), attemptId: 'att_1', kind: 'tool_call', payload: { name: 'verdict', ok: true, summary: 'request_changes' } },
    { id: 'e1', ts: at(7), kind: 'job_error', payload: { message: 'boom' } }
  ];
  const attempts = [attempt('att_1', 1, { outcome: 'checks_passed', endedAt: at(3) })];

  it('splits implement and review blocks of one attempt, with milestones between', () => {
    const items = buildTimeline(timeline, attempts);
    expect(items.map((item) => (item.type === 'block' ? `${item.review ? 'review' : 'attempt'}:${item.events.length}` : item.event.id))).toEqual([
      't1',
      'attempt:2',
      't2',
      't3',
      'review:2',
      'e1'
    ]);
    const [, implement, , , review] = items;
    if (implement.type !== 'block' || review.type !== 'block') throw new Error('expected blocks');
    expect(implement).toMatchObject({ models: 1, tools: 1, inputTokens: 100, outputTokens: 10, problems: 0, continued: false, start: at(1), end: at(2) });
    expect(implement.attempt?.n).toBe(1);
    expect(review).toMatchObject({ review: true, models: 1, problems: 1, continued: false });
  });

  it('marks a second run of the same attempt and role as continued', () => {
    const items = buildTimeline([timeline[1], timeline[3], timeline[2]], attempts);
    expect(items.map((item) => (item.type === 'block' ? item.continued : 'milestone'))).toEqual([false, 'milestone', true]);
  });

  it('summarizes attempts, tokens, and errors', () => {
    expect(summarize(timeline, attempts)).toEqual({ attempts: 1, events: 8, tokens: 165, errors: 1, first: at(0), last: at(7) });
  });

  it('labels outcomes, and an attempt with no end as running', () => {
    expect(outcomeLabel({ outcome: 'checks_passed', endedAt: 'x' })).toBe('Checks passed');
    expect(outcomeLabel({ outcome: '', endedAt: '' })).toBe('Running');
  });
});
