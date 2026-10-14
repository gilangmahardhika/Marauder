import { expect, test } from 'vitest';
import { agentLabel, filterRunners, installCommand, since, statusOptions, waitingForClaude, waitingForRunners } from '$lib/runners';
import type { Runner } from '$lib/types';

function runner(over: Partial<Runner>): Runner {
  return {
    id: 'rnr_a',
    name: 'agent-1',
    labels: ['podman'],
    slots: 2,
    busy: 0,
    status: 'idle',
    version: '0.1.0',
    protocol: 1,
    updateRequired: false,
    heartbeatAt: '2026-10-14T05:00:00Z',
    leases: [],
    actions: ['drain', 'remove'],
    agents: ['builtin'],
    ...over
  };
}

test('filters by name or label and by status', () => {
  const runners = [runner({}), runner({ id: 'rnr_b', name: 'gpu-box', labels: ['cpu=16'], status: 'offline' })];
  expect(filterRunners(runners, { q: 'cpu', status: '' }).map((r) => r.id)).toEqual(['rnr_b']);
  expect(filterRunners(runners, { q: '', status: 'idle' }).map((r) => r.id)).toEqual(['rnr_a']);
  expect(statusOptions(runners).map((o) => `${o.value}:${o.count ?? ''}`)).toEqual([':', 'idle:1', 'busy:0', 'draining:0', 'offline:1']);
});

test('says how long ago a heartbeat was', () => {
  const now = Date.parse('2026-10-14T05:02:30Z');
  expect(since(null, now)).toBe('never');
  expect(since('2026-10-14T05:02:28Z', now)).toBe('just now');
  expect(since('2026-10-14T05:02:00Z', now)).toBe('30s ago');
  expect(since('2026-10-14T05:00:00Z', now)).toBe('2m ago');
});

test('warns only when runners take jobs and none can', () => {
  expect(waitingForRunners('local', [])).toBe(false);
  expect(waitingForRunners('runners', [runner({ status: 'offline' })])).toBe(true);
  expect(waitingForRunners('runners', [runner({ status: 'busy' })])).toBe(false);
});

test('builds the install command and quotes odd names', () => {
  expect(installCommand('https://orch.example.com/', 'ab12', "Bob's box", 4)).toBe(
    "curl -fsSL https://orch.example.com/runner/install.sh | sudo bash -s -- --url https://orch.example.com --token ab12 --name 'Bob'\\''s box' --slots 4"
  );
  expect(installCommand('http://127.0.0.1:8787', 'ab12', '', 1)).not.toContain('--name');
});

test('waitingForClaude needs an online runner with Claude Code', () => {
  expect(waitingForClaude('runners', 'builtin', [])).toBe(false);
  expect(waitingForClaude('local', 'claude_code', [])).toBe(false);
  expect(waitingForClaude('runners', 'claude_code', [runner({ status: 'idle', agents: ['builtin'] })])).toBe(true);
  expect(waitingForClaude('runners', 'claude_code', [runner({ status: 'offline', agents: ['builtin', 'claude_code'] })])).toBe(true);
  expect(waitingForClaude('runners', 'claude_code', [runner({ status: 'busy', agents: ['builtin', 'claude_code'] })])).toBe(false);
  expect(agentLabel('claude_code')).toBe('Claude Code');
  expect(agentLabel('builtin')).toBe('Built-in');
});
