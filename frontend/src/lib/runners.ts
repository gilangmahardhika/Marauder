import { matchText, type Option } from '$lib/filter';
import type { AgentKind, Runner, RunnerStatus } from '$lib/types';

export const statusValues = ['idle', 'busy', 'draining', 'offline'] as const satisfies readonly RunnerStatus[];

const statusLabels: Record<RunnerStatus, string> = {
  idle: 'Idle',
  busy: 'Busy',
  draining: 'Draining',
  offline: 'Offline'
};

export function statusLabel(status: RunnerStatus) {
  return statusLabels[status] ?? status;
}

export function statusTone(status: RunnerStatus): 'default' | 'green' | 'amber' | 'sky' {
  if (status === 'idle') return 'green';
  if (status === 'busy') return 'sky';
  if (status === 'draining') return 'amber';
  return 'default';
}

export function statusOptions(runners: Runner[]): Option[] {
  return [
    { value: '', label: 'All statuses' },
    ...statusValues.map((value) => ({
      value,
      label: statusLabels[value],
      count: runners.filter((runner) => runner.status === value).length
    }))
  ];
}

export function filterRunners(runners: Runner[], filter: { q: string; status: string }) {
  return runners.filter(
    (runner) => (filter.status === '' || runner.status === filter.status) && matchText([runner.name, ...runner.labels], filter.q)
  );
}

const actionLabels: Record<string, string> = { drain: 'Drain', resume: 'Resume', remove: 'Remove' };

export function actionLabel(action: string) {
  return actionLabels[action] ?? action;
}

/** "just now", "45s ago", "3m ago", "2h ago", or "never". */
export function since(ts: string | null, now: number) {
  if (!ts) return 'never';
  const at = Date.parse(ts);
  if (Number.isNaN(at)) return ts;
  const seconds = Math.max(0, Math.round((now - at) / 1000));
  if (seconds < 5) return 'just now';
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  return `${Math.floor(minutes / 60)}h ago`;
}

/** True when jobs go to runners and none can take one. */
export function waitingForRunners(executor: string, runners: Runner[]) {
  return executor === 'runners' && !runners.some((runner) => runner.status === 'idle' || runner.status === 'busy');
}

const agentLabels: Record<string, string> = { builtin: 'Built-in', claude_code: 'Claude Code' };

export function agentLabel(agent: string) {
  return agentLabels[agent] ?? agent;
}

/** True when jobs go to runners with Claude Code and no online runner has it. */
export function waitingForClaude(executor: string, agent: AgentKind, runners: Runner[]) {
  return (
    executor === 'runners' &&
    agent === 'claude_code' &&
    !runners.some((runner) => (runner.status === 'idle' || runner.status === 'busy') && (runner.agents ?? []).includes('claude_code'))
  );
}

function quote(value: string) {
  return /^[A-Za-z0-9._:/=@-]+$/.test(value) ? value : `'${value.replaceAll("'", "'\\''")}'`;
}

/** The one-line install command for a new runner. */
export function installCommand(api: string, token: string, name: string, slots: number) {
  const origin = api.replace(/\/+$/, '');
  const args = ['--url', quote(origin), '--token', quote(token)];
  if (name.trim()) args.push('--name', quote(name.trim()));
  args.push('--slots', String(slots));
  return `curl -fsSL ${quote(`${origin}/runner/install.sh`)} | sudo bash -s -- ${args.join(' ')}`;
}
