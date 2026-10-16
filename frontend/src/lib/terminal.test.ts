import { expect, test } from 'vitest';
import { clipLines, foldLines, terminalEntry, terminalLines } from './terminal';
import type { TraceAttempt, TraceEvent } from './types';

function event(kind: string, payload: unknown, extra: Partial<TraceEvent> = {}): TraceEvent {
  return { id: `e_${kind}`, seq: 1, ts: '2026-10-12T10:00:01Z', attemptId: 'att_1', kind, payload, ...extra };
}

const attempt: TraceAttempt = {
  id: 'att_1',
  n: 2,
  taskClass: 'implement',
  modelId: 'model-a',
  fallbackModelId: '',
  inputTokens: 0,
  outputTokens: 0,
  maxTurns: 12,
  commitSha: '',
  reviewModelId: '',
  reviewVerdict: '',
  reviewIndependent: null,
  startedAt: '2026-10-12T10:00:00Z',
  endedAt: '',
  outcome: ''
};

test('a model call shows role, model, tokens, and stop reason over the reply', () => {
  const entry = terminalEntry(
    event('model_call', { role: 'implementer', modelId: 'model-a', inputTokens: 1500, outputTokens: 20, stopReason: 'tool_use', text: 'Reading.\n' })
  );
  expect(entry.tone).toBe('model');
  expect(entry.head).toBe('model  implementer  model-a  1.5k in / 20 out  stop tool_use');
  expect(entry.body).toBe('Reading.');
});

test('a tool call shows the command and its full output', () => {
  const output = Array.from({ length: 500 }, (_, i) => `line ${i}`).join('\n');
  const entry = terminalEntry(event('tool_call', { name: 'read_file', ok: true, summary: output, args: { path: 'src/a.ts' } }));
  expect(entry.tone).toBe('tool');
  expect(entry.head).toBe('$ read_file  src/a.ts');
  expect(entry.body).toBe(output);
});

test('a local check is green on exit 0 and red otherwise', () => {
  const pass = terminalEntry(event('tool_call', { name: 'run_local_check', ok: true, summary: 'exit 0\nok', args: { command: 'npm test' } }));
  expect(pass.tone).toBe('ok');
  expect(pass.head).toBe('$ run_local_check  npm test');
  const fail = terminalEntry(event('tool_call', { name: 'run_local_check', ok: true, summary: 'exit 1\n\u001b[31mFAIL\u001b[0m a.test.ts', args: { command: 'npm test' } }));
  expect(fail.tone).toBe('error');
  expect(fail.body).toBe('exit 1\nFAIL a.test.ts');
});

test('the new file tools and run_command show their targets', () => {
  const replace = terminalEntry(event('tool_call', { name: 'replace_in_file', ok: true, summary: 'replaced 1 occurrence in a.ts', args: { path: 'a.ts', oldBytes: 3, newBytes: 4 } }));
  expect(replace.head).toBe('$ replace_in_file  a.ts');
  expect(replace.body).toBe('replaced 1 occurrence in a.ts');
  expect(terminalEntry(event('tool_call', { name: 'move_file', ok: true, summary: 'moved', args: { from: 'a', to: 'b' } })).head).toBe('$ move_file  a -> b');
  expect(terminalEntry(event('tool_call', { name: 'read_file', ok: true, summary: '', args: { path: 'a.ts', offset: 40 } })).head).toBe('$ read_file  a.ts from line 40');
  const command = terminalEntry(event('tool_call', { name: 'run_command', ok: true, summary: 'exit 0\nok', args: { argv: ['go', 'vet', './...'] } }));
  expect(command).toMatchObject({ head: '$ run_command  go vet ./...', tone: 'ok' });
  const failed = terminalEntry(event('tool_call', { name: 'replace_in_file', ok: false, summary: 'error: old text not found in a.ts', args: { path: 'a.ts' } }));
  expect(failed.tone).toBe('error');
});

test('edit_file and replace_in_file with old/new build a terminal edit card', () => {
  const edit = terminalEntry(
    event('tool_call', {
      name: 'edit_file',
      ok: true,
      summary: 'wrote src/a.ts',
      args: { path: 'src/a.ts', bytes: 12, lines: 1, old: 'hello\n', new: 'hello world\n' }
    })
  );
  expect(edit.edit).toEqual({ path: 'src/a.ts', old: 'hello\n', new: 'hello world\n', name: 'edit_file' });
  expect(edit.body).toBe('');
  const replace = terminalEntry(
    event('tool_call', {
      name: 'replace_in_file',
      ok: true,
      summary: 'replaced 1 occurrence in a.ts',
      args: { path: 'a.ts', oldBytes: 5, newBytes: 3, old: 'hello', new: 'hi' }
    })
  );
  expect(replace.edit).toEqual({ path: 'a.ts', old: 'hello', new: 'hi', name: 'replace_in_file' });
  expect(replace.body).toBe('');
});

test('a failed or refused tool uses the error tone', () => {
  expect(terminalEntry(event('tool_call', { name: 'read_file', ok: false, summary: 'no such file', args: { path: 'x' } })).tone).toBe('error');
  const refused = terminalEntry(event('tool_call', { name: 'edit_file', ok: false, rejected: true, summary: 'outside the worktree', args: { path: '../x' } }));
  expect(refused.tone).toBe('error');
  expect(refused.head).toBe('$ edit_file  ../x  (refused)');
});

test('a commit shows its subject and a PR comment shows its body', () => {
  const commit = terminalEntry(event('tool_call', { name: 'commit', ok: true, summary: 'abc123', args: { message: 'Fix login\n\nDetails' } }));
  expect(commit.head).toBe('$ commit  Fix login');
  const comment = terminalEntry(event('tool_call', { name: 'comment_pr', ok: true, summary: 'posted', args: { body: 'Looks good' } }));
  expect(comment.head).toBe('$ comment_pr');
  expect(comment.body).toBe('posted\nLooks good');
});

test('a transition shows both statuses and a move to blocked is red', () => {
  const moved = terminalEntry(event('transition', { from: 'ready', to: 'in_progress', evidence: 'recorded' }));
  expect(moved).toMatchObject({ tone: 'transition', head: 'transition  ready -> in_progress', body: '' });
  const blocked = terminalEntry(event('transition', { from: 'changes_requested', to: 'blocked', evidence: 'attempt_cap' }));
  expect(blocked).toMatchObject({ tone: 'error', head: 'transition  changes_requested -> blocked  attempt_cap' });
});

test('a job error shows the whole message', () => {
  const entry = terminalEntry(event('job_error', { message: 'provider timeout\nretry later' }));
  expect(entry).toMatchObject({ tone: 'error', head: '! job_error', body: 'provider timeout\nretry later' });
});

test('a known kind uses its description and an unknown kind shows its payload as JSON', () => {
  const deploy = terminalEntry(event('deploy', { exitCode: 1 }));
  expect(deploy.tone).toBe('error');
  expect(deploy.head).toBe('deploy  exit 1');
  const unknown = terminalEntry(event('mystery', { a: 1 }));
  expect(unknown).toMatchObject({ tone: 'muted', head: 'mystery', body: '{"a":1}' });
  expect(terminalEntry(event('mystery', {})).body).toBe('');
});

test('a line marks where each attempt begins', () => {
  const entries = terminalLines(
    [
      event('model_call', { role: 'implementer', text: 'a' }, { id: 'e1' }),
      event('tool_call', { name: 'read_file', ok: true, summary: '', args: { path: 'a' } }, { id: 'e2' }),
      event('model_call', { role: 'implementer', text: 'b' }, { id: 'e3', attemptId: 'att_9' })
    ],
    [attempt]
  );
  expect(entries.map((entry) => entry.head)).toEqual([
    '--- attempt 2  implement  model-a  ---',
    expect.stringMatching(/^model/),
    '$ read_file  a',
    '--- att_9 ---',
    expect.stringMatching(/^model/)
  ]);
  expect(entries[0]).toMatchObject({ id: 'attempt-e1', tone: 'muted' });
});

test('clipLines folds output past 200 lines', () => {
  expect(foldLines).toBe(200);
  expect(clipLines('')).toEqual({ shown: '', hidden: 0 });
  const exact = Array.from({ length: 200 }, (_, i) => String(i)).join('\n');
  expect(clipLines(exact)).toEqual({ shown: exact, hidden: 0 });
  const long = Array.from({ length: 250 }, (_, i) => String(i)).join('\n');
  const clipped = clipLines(long);
  expect(clipped.hidden).toBe(50);
  expect(clipped.shown.split('\n')).toHaveLength(200);
  expect(clipped.shown.endsWith('\n199')).toBe(true);
});
