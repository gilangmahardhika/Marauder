import { tokens } from './format';
import { cleanText, describe, target } from './trace';
import type { TraceAttempt, TraceEvent } from './types';

export type TerminalTone = 'model' | 'tool' | 'ok' | 'error' | 'transition' | 'muted';

/** Capped edit snippets from a tool_call for a terminal edit card (RFC-132). */
export type TerminalEdit = { path: string; old: string; new: string; name: string };

/** One event as terminal output: a one-line head and the text under it. */
export type TerminalEntry = {
  id: string;
  ts: string;
  tone: TerminalTone;
  head: string;
  body: string;
  edit?: TerminalEdit;
};

/** Live thinking text for the current turn (shown only, never stored). */
export type PendingThinking = { attemptId: string; turn: number; text: string; since: string };

/** Output past this many lines is folded until a person asks for all of it. */
export const foldLines = 200;

/** How many of the newest events the terminal renders before "Show earlier events". */
export const pageEvents = 300;

function record(payload: unknown): Record<string, unknown> {
  return payload && typeof payload === 'object' && !Array.isArray(payload) ? (payload as Record<string, unknown>) : {};
}

function str(value: unknown) {
  return typeof value === 'string' ? value : value === undefined || value === null ? '' : String(value);
}

function num(value: unknown) {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

const ansi = /\u001b\[[0-9;?]*[A-Za-z]/g;

function clean(text: string) {
  return cleanText(text.replace(ansi, ''), Infinity).full.replace(/\s+$/, '');
}

function words(...parts: string[]) {
  return parts.filter(Boolean).join('  ');
}

function argText(name: string, args: Record<string, unknown>) {
  if (name === 'commit') return str(args.message).split('\n', 1)[0] ?? '';
  if (name === 'comment_pr') return '';
  return target(name, args);
}

function editFromArgs(name: string, args: Record<string, unknown>): TerminalEdit | undefined {
  if (name !== 'edit_file' && name !== 'replace_in_file') return undefined;
  const path = str(args.path);
  const next = str(args.new);
  const prev = str(args.old);
  if (!path || (next === '' && prev === '')) return undefined;
  return { path, old: prev, new: next, name };
}

function toolEntry(event: TraceEvent, p: Record<string, unknown>): TerminalEntry {
  const name = str(p.name);
  const args = record(p.args);
  const summary = str(p.summary);
  const failed = p.ok === false || p.rejected === true;
  const check = name === 'run_local_check' || name === 'run_command';
  const edit = !failed ? editFromArgs(name, args) : undefined;
  let body = summary;
  if (edit) body = '';
  else if (name === 'edit_file') body = summary + (args.lines === undefined ? '' : ` (${num(args.lines)} lines)`);
  else if (name === 'comment_pr' || name === 'verdict') body = [summary, str(args.body)].filter(Boolean).join('\n');
  else if (name === 'conventions_context' || name === 'linked_context') body = describe(event).text || summary;
  const tone: TerminalTone = failed ? 'error' : check ? (/^exit 0\b/.test(summary) ? 'ok' : 'error') : 'tool';
  return {
    id: event.id,
    ts: event.ts,
    tone,
    head: words(`$ ${name}`, argText(name, args), p.rejected === true ? '(refused)' : ''),
    body: clean(body),
    edit
  };
}

/** Turns one event into terminal output. Every kind has a head; an unknown kind shows its payload as JSON. */
export function terminalEntry(event: TraceEvent): TerminalEntry {
  const p = record(event.payload);
  const base = { id: event.id, ts: event.ts };
  switch (event.kind) {
    case 'model_call': {
      const stop = str(p.stopReason);
      return {
        ...base,
        tone: 'model',
        head: words(
          'model',
          str(p.role),
          str(p.modelId),
          `${tokens(num(p.inputTokens))} in / ${tokens(num(p.outputTokens))} out`,
          stop ? `stop ${stop}` : ''
        ),
        body: clean(str(p.text))
      };
    }
    case 'tool_call':
      return toolEntry(event, p);
    case 'transition': {
      const to = str(p.to);
      const evidence = str(p.evidence);
      return {
        ...base,
        tone: to === 'blocked' ? 'error' : 'transition',
        head: words('transition', `${str(p.from)} -> ${to}`, evidence === 'recorded' ? '' : evidence),
        body: ''
      };
    }
    case 'job_error':
      return { ...base, tone: 'error', head: '! job_error', body: clean(str(p.message)) };
    default: {
      const said = describe(event);
      const known = said.detail !== '' || said.tone !== 'default';
      const tone: TerminalTone = said.tone === 'red' || said.tone === 'amber' ? 'error' : said.tone === 'green' ? 'ok' : 'muted';
      const json = event.payload === null || event.payload === undefined ? '' : JSON.stringify(event.payload);
      return {
        ...base,
        tone,
        head: words(event.kind, known ? said.detail : ''),
        body: known ? clean(said.text) : clean(json === '{}' ? '' : json)
      };
    }
  }
}

/**
 * The events as terminal output, oldest first. A line marks where the events
 * of another attempt begin.
 */
export function terminalLines(events: TraceEvent[], attempts: TraceAttempt[] = []): TerminalEntry[] {
  const byId = new Map(attempts.map((attempt) => [attempt.id, attempt]));
  const out: TerminalEntry[] = [];
  let attemptId = '';
  for (const event of events) {
    if (event.attemptId && event.attemptId !== attemptId) {
      const attempt = byId.get(event.attemptId);
      out.push({
        id: `attempt-${event.id}`,
        ts: event.ts,
        tone: 'muted',
        head: attempt ? words(`--- attempt ${attempt.n}`, attempt.taskClass, attempt.modelId, '---') : `--- ${event.attemptId} ---`,
        body: ''
      });
    }
    attemptId = event.attemptId ?? attemptId;
    out.push(terminalEntry(event));
  }
  return out;
}

/** The model reply being written, from the trace stream. It is shown only, never stored (RFC-088). */
export type PendingReply = { attemptId: string; turn: number; role: string; modelId: string; text: string; since: string };

/** The reply being written as the terminal's last entry. Its body is empty until the first text arrives. */
export function pendingEntry(pending: PendingReply): TerminalEntry {
  return {
    id: `pending-${pending.attemptId}-${pending.role}-${pending.turn}`,
    ts: pending.since,
    tone: 'model',
    head: words('model', pending.role, pending.modelId, 'writing…'),
    body: clean(pending.text)
  };
}

/** The first `max` lines of text, and how many lines are left out. */
export function clipLines(text: string, max = foldLines): { shown: string; hidden: number } {
  if (text === '') return { shown: '', hidden: 0 };
  const lines = text.split('\n');
  if (lines.length <= max) return { shown: text, hidden: 0 };
  return { shown: lines.slice(0, max).join('\n'), hidden: lines.length - max };
}
