import { matchText, type Option } from './filter';
import { count } from './format';
import { reasonLabel, sentence, statusLabel } from './status';
import type { TraceAttempt, TraceEvent } from './types';

/** `kind` and `attempt` are empty for every event. */
export type TraceFilter = { q: string; kind: string; attempt: string };

export type Tone = 'default' | 'green' | 'amber' | 'sky' | 'red';

const control = /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/g;

/** One event in words: a label, a short detail, right-hand meta, and the long text behind it. */
export type Described = { label: string; detail: string; meta: string; tone: Tone; text: string };

const kinds: Record<string, string> = {
  transition: 'Status change',
  transition_rejected: 'Refused status change',
  model_call: 'Model call',
  tool_call: 'Tool call',
  job_error: 'Error',
  reviewers_requested: 'Reviewers requested',
  route: 'Model route',
  gate: 'Gate',
  deploy: 'Deploy',
  merge: 'Squash and merge',
  wont_fix: "Won't fix",
  health: 'Health check',
  verify: 'Dev check',
  budget_stop: 'Budget stop',
  config_changed: 'Settings changed',
  assertion_failed: 'Harness assertion',
  stacks_detected: 'Stacks detected',
  instructions_saved: 'Instructions saved',
  context_changed: 'Context changed',
  review_failed: 'AI review gave no verdict'
};

const tools: Record<string, string> = {
  conventions_context: 'Rules and skills',
  linked_context: 'Linked projects',
  mcp_context: 'MCP servers',
  list_files: 'List files',
  search_files: 'Search files',
  read_file: 'Read file',
  read_linked_file: 'Read linked file',
  replace_in_file: 'Replace in file',
  edit_file: 'Edit file',
  delete_file: 'Delete file',
  move_file: 'Move file',
  todo_write: 'Todo list',
  repo_context: 'Repository notes',
  run_local_check: 'Run local check',
  run_command: 'Run command',
  commit: 'Commit',
  read_diff: 'Read diff',
  comment_pr: 'Comment on pull request',
  verdict: 'Verdict',
  create_card: 'Create card'
};

const outcomes: Record<string, string> = {
  checks_passed: 'Checks passed',
  checks_failed: 'Checks failed',
  no_commit: 'No commit',
  no_change: 'No new change',
  budget: 'Out of budget',
  error: 'Error',
  cancelled: 'Cancelled'
};

const verdicts: Record<string, string> = {
  recommend_approve: 'Recommend approve',
  request_changes: 'Request changes',
  failed: 'Failed, no verdict'
};

export function kindLabel(kind: string) {
  return kinds[kind] ?? sentence(kind);
}

const mcpName = /^mcp_([a-z][a-z0-9]{1,15})_([A-Za-z0-9_-]{1,40})$/;

/** `mcp_<slug>_<tool>` reads as "MCP: slug / tool". */
export function toolLabel(name: string) {
  const mcp = mcpName.exec(name);
  if (mcp) return `MCP: ${mcp[1]} / ${mcp[2]}`;
  return tools[name] ?? sentence(name);
}

export function verdictLabel(verdict: string) {
  return verdicts[verdict] ?? sentence(verdict);
}

export function verdictTone(verdict: string): Tone {
  if (verdict === 'recommend_approve') return 'green';
  return verdict === 'failed' ? 'red' : 'amber';
}

/** An attempt with no end is still running. */
export function outcomeLabel(attempt: Pick<TraceAttempt, 'outcome' | 'endedAt'>) {
  if (attempt.outcome) return outcomes[attempt.outcome] ?? sentence(attempt.outcome);
  return attempt.endedAt ? 'Ended' : 'Running';
}

export function outcomeTone(attempt: Pick<TraceAttempt, 'outcome' | 'endedAt'>): Tone {
  switch (attempt.outcome) {
    case 'checks_passed':
      return 'green';
    case 'checks_failed':
    case 'error':
      return 'red';
    case 'no_commit':
    case 'budget':
      return 'amber';
    case '':
      return attempt.endedAt ? 'default' : 'sky';
    default:
      return 'default';
  }
}

function record(payload: unknown): Record<string, unknown> {
  return payload && typeof payload === 'object' && !Array.isArray(payload) ? (payload as Record<string, unknown>) : {};
}

function str(value: unknown) {
  return typeof value === 'string' ? value : value === undefined || value === null ? '' : String(value);
}

function num(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function firstLine(text: string, max = 160) {
  const line = text.trimStart().split('\n', 1)[0] ?? '';
  return line.length > max ? `${line.slice(0, max - 1)}…` : line;
}

function lineCount(text: string) {
  const trimmed = text.replace(/\n+$/, '');
  return trimmed === '' ? 0 : trimmed.split('\n').length;
}

function capitalized(text: string) {
  return text.charAt(0).toUpperCase() + text.slice(1);
}

function plural(n: number, one: string, many: string) {
  return `${count(n)} ${n === 1 ? one : many}`;
}

/** Full SHAs read as their first 7 characters. */
function shortShas(text: string) {
  return text.replace(/\b([0-9a-f]{7})[0-9a-f]{33}\b/g, '$1');
}

function evidence(text: string) {
  if (text === '' || text === 'recorded') return '';
  if (/^[a-z_]+$/.test(text)) return reasonLabel(text);
  return shortShas(text);
}

/** What the tool was asked for, from `args`, in a few words. */
export function target(name: string, args: Record<string, unknown>) {
  switch (name) {
    case 'list_files':
      return [str(args.path), str(args.glob)].filter(Boolean).join(' ');
    case 'edit_file':
    case 'replace_in_file':
    case 'delete_file':
      return str(args.path);
    case 'read_file':
      return str(args.path) + fromLine(args.offset);
    case 'read_linked_file':
      return [str(args.project), str(args.path)].filter(Boolean).join(': ') + fromLine(args.offset);
    case 'move_file':
      return args.from === undefined ? '' : `${str(args.from)} -> ${str(args.to)}`;
    case 'search_files': {
      if (args.text === undefined) return '';
      const where = [str(args.path), str(args.glob)].filter(Boolean).join(' ');
      return `"${firstLine(str(args.text), 80)}"` + (where ? ` in ${where}` : '');
    }
    case 'run_local_check':
      return firstLine(str(args.command), 80);
    case 'run_command':
      return Array.isArray(args.argv) ? firstLine(args.argv.map(str).join(' '), 80) : '';
    default:
      return '';
  }
}

function fromLine(offset: unknown) {
  return num(offset) > 1 ? ` from line ${num(offset)}` : '';
}

/** "lines A-B of T" from the end of a read result, or a count of its lines. */
function readSize(summary: string) {
  const range = /lines (\d+)-(\d+) of (\d+)/.exec(summary);
  if (range) return `lines ${range[1]}-${range[2]} of ${count(Number(range[3]))}`;
  if (/lines 0 of 0/.test(summary)) return 'Empty file';
  return plural(lineCount(summary), 'line', 'lines');
}

function joined(...parts: string[]) {
  return parts.filter(Boolean).join(' · ');
}

function describeTool(p: Record<string, unknown>): Described {
  const name = str(p.name);
  const summary = str(p.summary);
  const label = toolLabel(name);
  const args = record(p.args);
  const on = target(name, args);
  if (p.rejected === true) {
    return { label, detail: joined(on, `Refused: ${firstLine(summary)}`), meta: '', tone: 'amber', text: summary };
  }
  if (p.ok === false) {
    return { label, detail: joined(on, firstLine(summary)), meta: '', tone: 'red', text: summary };
  }
  switch (name) {
    case 'conventions_context': {
      const items = Array.isArray(p.items) ? p.items.map(record) : [];
      const text = items.map((item) => `${str(item.kind)} ${str(item.id)} v${str(item.version)} ${str(item.hash).slice(0, 12)}`).join('\n');
      return { label, detail: summary, meta: '', tone: 'default', text };
    }
    case 'mcp_context': {
      const servers = Array.isArray(p.servers) ? p.servers.map(record) : [];
      const text = servers
        .map((server) => {
          const where = str(server.database) ? `${str(server.host)}/${str(server.database)}` : str(server.host) || str(server.image);
          const listed = Array.isArray(server.tools) ? server.tools.map(record).map((tool) => `${str(tool.name)} ${str(tool.schemaSha256).slice(0, 12)}`) : [];
          const error = str(server.error);
          return `${str(server.slug)} ${str(server.transport)} ${where}: ${error || listed.join(', ') || 'no tools'}`;
        })
        .join('\n');
      const failed = servers.some((server) => str(server.error) !== '');
      const missing = Array.isArray(p.missing) ? p.missing.map(str).filter(Boolean) : [];
      const attached = Array.isArray(p.attached) ? p.attached.map(str).filter(Boolean) : [];
      const detached = Array.isArray(p.detached) ? p.detached.map(str).filter(Boolean) : [];
      const lines = [
        text,
        ...missing.map((slug) => `not a server: ${slug}`),
        attached.length > 0 ? `attached on the card: ${attached.join(', ')}` : '',
        detached.length > 0 ? `detached on the card: ${detached.join(', ')}` : ''
      ]
        .filter(Boolean)
        .join('\n');
      const scope = str(p.scope);
      const detail =
        scope === 'none'
          ? `No MCP servers${str(p.reason) ? `: ${str(p.reason)}` : ''}`
          : scope === 'mentioned'
            ? `${summary}, from the requirements' mentions`
            : scope === 'card'
              ? `${summary}, changed on the card`
              : summary;
      const warn = failed || scope === 'none' || missing.length > 0;
      return { label, detail, meta: '', tone: warn ? 'amber' : 'default', text: lines };
    }
    case 'linked_context': {
      const links = Array.isArray(p.links) ? p.links.map(record) : [];
      return { label, detail: summary, meta: '', tone: 'default', text: links.map((link) => `${str(link.projectId)} ${str(link.sha)}`).join('\n') };
    }
    case 'list_files':
      return { label, detail: joined(on, plural(lineCount(summary), 'path', 'paths')), meta: '', tone: 'default', text: summary };
    case 'search_files': {
      const n = lineCount(summary);
      return { label, detail: joined(on, n === 0 ? 'No matches' : plural(n, 'match', 'matches')), meta: '', tone: 'default', text: summary };
    }
    case 'read_file':
    case 'read_linked_file':
      return { label, detail: joined(on, readSize(summary)), meta: '', tone: 'default', text: summary };
    case 'edit_file': {
      const size = args.lines === undefined ? '' : plural(num(args.lines), 'line', 'lines');
      return { label, detail: joined(on || summary.replace(/^wrote /, ''), size), meta: '', tone: 'default', text: '' };
    }
    case 'replace_in_file': {
      const n = /replaced (\d+) occurrence/.exec(summary);
      return { label, detail: joined(on, n ? plural(Number(n[1]), 'occurrence', 'occurrences') : ''), meta: '', tone: 'default', text: '' };
    }
    case 'delete_file':
    case 'move_file':
      return { label, detail: on || firstLine(summary), meta: '', tone: 'default', text: '' };
    case 'todo_write':
    case 'repo_context':
      return { label, detail: firstLine(summary), meta: '', tone: 'default', text: summary };
    case 'run_command':
    case 'run_local_check': {
      const head = firstLine(summary);
      return { label, detail: joined(head, on), meta: '', tone: /^exit 0\b/.test(head) ? 'green' : 'red', text: summary };
    }
    case 'read_diff': {
      const files = summary.split('\n').filter((line) => line.startsWith('diff --git ')).length;
      return { label, detail: plural(files, 'file', 'files'), meta: '', tone: 'default', text: summary };
    }
    case 'commit': {
      const message = str(args.message);
      return { label, detail: message ? firstLine(message) : firstLine(summary), meta: '', tone: 'default', text: lineCount(message) > 1 ? message : '' };
    }
    case 'comment_pr': {
      const body = str(args.body);
      return { label, detail: body ? firstLine(body) : firstLine(summary), meta: '', tone: 'default', text: body };
    }
    case 'verdict':
      return {
        label,
        detail: verdictLabel(summary),
        meta: '',
        tone: summary === 'recommend_approve' ? 'green' : summary === 'request_changes' ? 'amber' : 'default',
        text: str(args.body)
      };
    default:
      if (mcpName.test(name)) {
        return { label, detail: plural(num(p.bytes), 'byte', 'bytes'), meta: '', tone: 'default', text: summary };
      }
      return { label, detail: firstLine(summary), meta: '', tone: 'default', text: lineCount(summary) > 1 || summary.length > 160 ? summary : '' };
  }
}

/** Puts one event into words. Every kind has a template, and an unknown kind reads as its name. */
export function describe(event: TraceEvent): Described {
  const p = record(event.payload);
  switch (event.kind) {
    case 'transition': {
      const to = str(p.to);
      const tone: Tone = to === 'blocked' ? 'red' : to === 'approved' || to === 'done' ? 'green' : 'default';
      return { label: `${statusLabel(str(p.from))} → ${statusLabel(to)}`, detail: evidence(str(p.evidence)), meta: '', tone, text: '' };
    }
    case 'transition_rejected':
      return {
        label: 'Refused status change',
        detail: `${statusLabel(str(p.from))} → ${statusLabel(str(p.to))}: ${sentence(str(p.evidence))}`,
        meta: '',
        tone: 'amber',
        text: ''
      };
    case 'model_call': {
      const role = str(p.role);
      const stop = str(p.stopReason);
      const reply = str(p.text).trim();
      return {
        label: role === 'reviewer' ? 'Reviewer model' : role === 'planner' ? 'Planner model' : 'Model call',
        detail: reply ? firstLine(reply) : joined(str(p.modelId), stop ? sentence(stop).toLowerCase() : ''),
        meta: `${count(num(p.inputTokens))} in / ${count(num(p.outputTokens))} out`,
        tone: 'default',
        text: reply
      };
    }
    case 'tool_call':
      return describeTool(p);
    case 'job_error': {
      const message = str(p.message);
      return { label: 'Error', detail: firstLine(message.replace(control, '')), meta: '', tone: 'red', text: message };
    }
    case 'reviewers_requested': {
      const reviewers = Array.isArray(p.reviewers) ? p.reviewers.map(str) : [];
      const waiting = Array.isArray(p.waiting) ? p.waiting.map(str) : [];
      const ok = p.ok !== false;
      const asked = reviewers.map((login) => `@${login}`).join(', ');
      const pending = waiting.length > 0 ? `waiting for GitHub: ${waiting.join(', ')}` : '';
      return {
        label: reviewers.length === 0 && waiting.length > 0 ? 'Reviewers waiting' : 'Reviewers requested',
        detail: ok ? [asked, pending].filter(Boolean).join('; ') : firstLine(str(p.message)),
        meta: '',
        tone: ok ? 'default' : 'amber',
        text: ok ? '' : str(p.message)
      };
    }
    case 'deploy': {
      const exit = num(p.exitCode);
      return { label: 'Deploy', detail: [`exit ${exit}`, shortShas(str(p.requestedSha))].filter(Boolean).join(' · '), meta: '', tone: exit === 0 ? 'green' : 'red', text: '' };
    }
    case 'wont_fix':
      return {
        label: "Won't fix",
        detail: p.prClosed === true ? 'Pull request closed' : '',
        meta: str(p.login) ? `@${str(p.login)}` : '',
        tone: 'default',
        text: str(p.reason)
      };
    case 'health':
      return p.passed === true
        ? { label: 'Health check', detail: `Passed (${num(p.status)})`, meta: '', tone: 'green', text: '' }
        : { label: 'Health check', detail: 'Failed, rolled back', meta: '', tone: 'red', text: '' };
    case 'verify': {
      const exit = num(p.exitCode);
      return { label: 'Dev check', detail: `exit ${exit}`, meta: '', tone: exit === 0 ? 'green' : 'red', text: '' };
    }
    case 'review_failed':
      return { label: 'AI review gave no verdict', detail: shortShas(str(p.sha)), meta: '', tone: 'amber', text: '' };
    case 'route':
      return { label: 'Model route', detail: str(p.modelId), meta: str(p.taskClass), tone: 'default', text: '' };
    case 'config_changed': {
      const fields = Array.isArray(p.fields) ? p.fields.map(str).join(', ') : '';
      return { label: 'Settings changed', detail: [str(p.section), fields].filter(Boolean).join(': '), meta: '', tone: 'default', text: '' };
    }
    case 'assertion_failed':
      if (p.name === 'mcp_changed') {
        return { label: 'Harness assertion failed', detail: `MCP tool changed: ${str(p.server)} / ${str(p.tool)}`, meta: '', tone: 'red', text: '' };
      }
      return { label: 'Harness assertion failed', detail: [str(p.name), str(p.detail)].filter(Boolean).join(': '), meta: '', tone: 'red', text: '' };
    case 'budget_stop':
      return { label: 'Budget stop', detail: '', meta: '', tone: 'red', text: '' };
    case 'context_compacted':
      return {
        label: 'Context compacted',
        detail: `at ${count(num(p.inputTokens))} tokens, ${plural(num(p.turnsDropped), 'turn', 'turns')} summarized`,
        meta: num(p.n) > 1 ? `#${num(p.n)}` : '',
        tone: 'default',
        text: ''
      };
    case 'handoff_saved': {
      const left = num(p.left);
      return {
        label: 'Handoff saved',
        detail: [
          plural(num(p.files), 'uncommitted file', 'uncommitted files'),
          str(p.source) === 'model' ? 'summary by the model' : 'summary by the harness',
          left > 0 ? `${plural(left, 'file', 'files')} too large to keep` : ''
        ]
          .filter(Boolean)
          .join(' · '),
        meta: '',
        tone: 'default',
        text: ''
      };
    }
    case 'handoff_applied':
      return p.ok === false
        ? { label: 'Earlier work could not be re-applied', detail: capitalized(str(p.reason)), meta: '', tone: 'amber', text: '' }
        : {
            label: 'Continued from the last attempt',
            detail: [plural(num(p.files), 'file', 'files'), plural(num(p.todos), 'todo', 'todos')].join(' · '),
            meta: '',
            tone: 'default',
            text: ''
          };
    case 'agent_run': {
      const exit = num(p.exitCode);
      return {
        label: 'Claude Code',
        detail: [str(p.modelId), plural(num(p.turns), 'turn', 'turns'), exit !== 0 ? `exited ${exit}` : '']
          .filter(Boolean)
          .join(' · '),
        meta: str(p.version),
        tone: exit !== 0 ? 'red' : 'default',
        text: ''
      };
    }
    case 'context_changed': {
      const what = ({ skill: 'skill', rule: 'rule', mcp: 'MCP server' } as Record<string, string>)[str(p.kind)] ?? str(p.kind);
      const verb = ({ attach: 'Attached', detach: 'Detached', inherit: 'Restored' } as Record<string, string>)[str(p.mode)] ?? str(p.mode);
      return { label: 'Context changed', detail: `${verb} ${what} ${str(p.itemId)}`.trim(), meta: '', tone: 'default', text: '' };
    }
    case 'instructions_saved': {
      const length = num(p.length);
      return {
        label: 'Instructions saved',
        detail: length > 0 ? `${count(length)} characters` : 'Cleared',
        meta: '',
        tone: 'default',
        text: ''
      };
    }
    default:
      return { label: kindLabel(event.kind), detail: '', meta: '', tone: 'default', text: '' };
  }
}

/**
 * Adds polled events to the ones already shown. An event read twice keeps one
 * row; the order is by time, then by insert order within a second.
 */
export function mergeEvents(current: TraceEvent[], incoming: TraceEvent[]): TraceEvent[] {
  const byId = new Map(current.map((event) => [event.id, event]));
  for (const event of incoming) byId.set(event.id, event);
  return [...byId.values()].sort((a, b) => (a.ts < b.ts ? -1 : a.ts > b.ts ? 1 : (a.seq ?? 0) - (b.seq ?? 0)));
}

/** A problem is an event shown in red or amber. */
export function isProblem(event: TraceEvent) {
  const tone = describe(event).tone;
  return tone === 'red' || tone === 'amber';
}

export type CleanText = { text: string; full: string; cut: boolean; binary: boolean; bytes: number };

/**
 * Strips NUL and other control characters, keeps newlines and tabs, and cuts
 * the text at `cap` characters. Text that was mostly NUL padding, such as a
 * tar archive, is marked binary.
 */
export function cleanText(raw: string, cap = 4000): CleanText {
  const bytes = new TextEncoder().encode(raw).length;
  const full = raw.replace(control, '');
  const removed = raw.length - full.length;
  return { text: full.length > cap ? full.slice(0, cap) : full, full, cut: full.length > cap, binary: removed > 64, bytes };
}

export type Block = {
  type: 'block';
  key: string;
  attemptId: string;
  attempt: TraceAttempt | null;
  review: boolean;
  continued: boolean;
  events: TraceEvent[];
  models: number;
  tools: number;
  problems: number;
  inputTokens: number;
  outputTokens: number;
  start: string;
  end: string;
};

export type Milestone = { type: 'event'; key: string; event: TraceEvent };

export type TimelineItem = Block | Milestone;

function isReview(events: TraceEvent[]) {
  return events.some((event) => {
    const p = record(event.payload);
    if (event.kind === 'model_call') return p.role === 'reviewer';
    return event.kind === 'tool_call' && ['read_diff', 'verdict', 'comment_pr'].includes(str(p.name));
  });
}

/**
 * Groups events in their order. Events without an attempt are milestones. A
 * run of events with one attempt is a block. The reviewer's events carry the
 * implement attempt's id, so they form their own block marked `review`.
 */
export function buildTimeline(events: TraceEvent[], attempts: TraceAttempt[]): TimelineItem[] {
  const byId = new Map(attempts.map((attempt) => [attempt.id, attempt]));
  const items: TimelineItem[] = [];
  for (const event of events) {
    const last = items.at(-1);
    if (!event.attemptId) {
      items.push({ type: 'event', key: event.id, event });
    } else if (last?.type === 'block' && last.attemptId === event.attemptId) {
      last.events.push(event);
    } else {
      items.push({
        type: 'block',
        key: event.id,
        attemptId: event.attemptId,
        attempt: byId.get(event.attemptId) ?? null,
        review: false,
        continued: false,
        events: [event],
        models: 0,
        tools: 0,
        problems: 0,
        inputTokens: 0,
        outputTokens: 0,
        start: event.ts,
        end: event.ts
      });
    }
  }
  const seen = new Set<string>();
  for (const item of items) {
    if (item.type !== 'block') continue;
    item.review = isReview(item.events);
    const id = `${item.attemptId}:${item.review}`;
    item.continued = seen.has(id);
    seen.add(id);
    item.start = item.events[0].ts;
    item.end = item.events[item.events.length - 1].ts;
    for (const event of item.events) {
      const p = record(event.payload);
      if (event.kind === 'model_call') {
        item.models += 1;
        item.inputTokens += num(p.inputTokens);
        item.outputTokens += num(p.outputTokens);
      }
      if (event.kind === 'tool_call') item.tools += 1;
      if (isProblem(event)) item.problems += 1;
    }
  }
  return items;
}

export type TraceSummary = { attempts: number; events: number; tokens: number; errors: number; first: string; last: string };

export function summarize(events: TraceEvent[], attempts: TraceAttempt[]): TraceSummary {
  let tokens = 0;
  let errors = 0;
  for (const event of events) {
    const p = record(event.payload);
    if (event.kind === 'model_call') tokens += num(p.inputTokens) + num(p.outputTokens);
    if (event.kind === 'job_error') errors += 1;
  }
  const ids = new Set(events.flatMap((event) => (event.attemptId ? [event.attemptId] : [])));
  return {
    attempts: Math.max(attempts.length, ids.size),
    events: events.length,
    tokens,
    errors,
    first: events[0]?.ts ?? '',
    last: events.at(-1)?.ts ?? ''
  };
}

function counted(values: string[], all: string, label: (value: string) => string, order?: (a: string, b: string) => number): Option[] {
  const counts = new Map<string, number>();
  for (const value of values) counts.set(value, (counts.get(value) ?? 0) + 1);
  const keys = [...counts.keys()].sort(order);
  return [{ value: '', label: all }, ...keys.map((value) => ({ value, label: `${label(value)} (${counts.get(value)})` }))];
}

export function kindOptions(events: TraceEvent[]) {
  return counted(
    events.map((event) => event.kind),
    'All kinds',
    kindLabel,
    (a, b) => kindLabel(a).localeCompare(kindLabel(b))
  );
}

export function attemptOptions(events: TraceEvent[], attempts: TraceAttempt[] = []) {
  const numbers = new Map(attempts.map((attempt) => [attempt.id, attempt.n]));
  const label = (id: string) => (numbers.has(id) ? `Attempt ${numbers.get(id)}` : id);
  return counted(
    events.flatMap((event) => (event.attemptId ? [event.attemptId] : [])),
    'All attempts',
    label,
    (a, b) => (numbers.get(a) ?? Infinity) - (numbers.get(b) ?? Infinity) || a.localeCompare(b)
  );
}

export function filterEvents(events: TraceEvent[], filter: TraceFilter) {
  return events.filter((event) => {
    if (filter.kind !== '' && event.kind !== filter.kind) return false;
    if (filter.attempt !== '' && event.attemptId !== filter.attempt) return false;
    if (filter.q.trim() === '') return true;
    const words = describe(event);
    return matchText([event.kind, kindLabel(event.kind), event.attemptId, words.label, words.detail, JSON.stringify(event.payload)], filter.q);
  });
}
