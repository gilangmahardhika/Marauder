import type { Option } from './filter';
import { listItems } from './markdown';
import { displayName } from './people';
import type { AIReviewMode, MCPServerMention, Reviewer } from './types';
import type { SearchOption } from './ui/SearchSelect.svelte';

const LIST_MARKER = /^(?:[-*+]|\d{1,9}[.)])(?:[ \t]+\[[ xX]\])?(?:[ \t]+|$)/;

/**
 * Turns the acceptance textarea into checks. When the text has a markdown list, only its items are checks,
 * as ingest reads an Acceptance section, and skipped counts the other lines. Otherwise each non-blank line
 * is a check. A leading list or checkbox marker is dropped, since the pull request writes each line as `- [ ] `.
 */
export function acceptanceChecks(text: string): { lines: string[]; skipped: number } {
  const list = listItems(text);
  if (list) return { lines: list.items, skipped: list.skipped };
  const lines = text
    .split(/\r\n?|\n/)
    .map((line) => line.trim().replace(LIST_MARKER, '').trim())
    .filter((line) => line !== '');
  return { lines, skipped: 0 };
}

export function toLines(text: string): string[] {
  return acceptanceChecks(text).lines;
}

/** Saved checks load as a list, so adding a list item later keeps them. */
export function fromLines(lines: string[]): string {
  return lines.map((line) => `- ${line}`).join('\n');
}

/** Only a Lead Developer or an admin may turn Self review on or off (RFC-070), or change the AI review mode (RFC-072). */
export function canSetSelfReview(role: string, admin: boolean): boolean {
  return admin || role === 'lead_developer';
}

export const reviewModes: Option<AIReviewMode>[] = [
  { value: 'continue', label: 'Run the next attempt' },
  { value: 'manual', label: 'A person reviews' }
];

export const reviewModeHints: Record<AIReviewMode, string> = {
  continue: 'The card goes back with the review note, and the next attempt starts on its own.',
  manual: 'The card waits for its pull request checks, then a person reads the note and approves or asks for changes.'
};

/** Reviewer picker options. The creator opens the pull request, so they are left out. */
export function reviewerChoices(
  options: Reviewer[],
  creatorEmail: string | null,
  current: Reviewer[] = []
): SearchOption[] {
  const seen = new Set<string>();
  const out: SearchOption[] = [];
  for (const reviewer of [...current, ...options]) {
    if (seen.has(reviewer.id) || reviewer.email === creatorEmail) continue;
    seen.add(reviewer.id);
    out.push({
      value: reviewer.id,
      label: displayName(reviewer),
      detail: [reviewer.githubLogin, reviewer.email].filter(Boolean).join(' · ')
    });
  }
  return out;
}

const MENTION_BEFORE = '([{"\',;:';
const MENTION_AFTER = ')]}"\',;:!?';
const MAX_MENTIONS = 16;
const FENCE = /^ {0,3}(`{3,}|~{3,})(.*)$/;

function mentionEnds(text: string, end: number): boolean {
  const next = text[end];
  if (next === undefined || /\s/.test(next) || MENTION_AFTER.includes(next)) return true;
  return next === '.' && !/[\p{L}\p{N}]/u.test(text[end + 1] ?? '');
}

function mentionStarts(text: string, at: number): boolean {
  const prev = text[at - 1];
  return prev === undefined || /\s/.test(prev) || MENTION_BEFORE.includes(prev);
}

/** Each @slug in plain text, with its range, by the RFC-101 grammar. */
export function findMentions(text: string): { slug: string; start: number; end: number }[] {
  const out: { slug: string; start: number; end: number }[] = [];
  for (let i = 0; i < text.length; i++) {
    if (text[i] !== '@' || !mentionStarts(text, i)) continue;
    const match = /^[a-z][a-z0-9]*/.exec(text.slice(i + 1));
    if (!match) continue;
    const end = i + 1 + match[0].length;
    if (match[0].length >= 2 && match[0].length <= 16 && mentionEnds(text, end)) {
      out.push({ slug: match[0], start: i, end });
    }
    i = end - 1;
  }
  return out;
}

/**
 * The MCP servers a markdown text mentions (RFC-101), in order of first use, without repeats, at most 16.
 * Code, links, autolinks, and HTML tags are not read; emphasis marks are, as the harness reads the markdown tree.
 */
export function mentions(text: string): string[] {
  const kept: string[] = [];
  let fence = '';
  for (const line of text.split(/\r\n?|\n/)) {
    const mark = FENCE.exec(line);
    if (mark) {
      if (fence === '' && !(mark[1][0] === '`' && mark[2].includes('`'))) fence = mark[1];
      else if (fence !== '' && mark[1][0] === fence[0] && mark[1].length >= fence.length && mark[2].trim() === '') fence = '';
      continue;
    }
    if (fence !== '' || /^( {4}|\t)/.test(line)) continue;
    const plain = line
      .replace(/(`+)[\s\S]*?\1/g, '\0')
      .replace(/!?\[[^\]]*\]\([^)]*\)/g, '\0')
      .replace(/<[^>]*>/g, '\0')
      .replace(/[*~]+/g, '');
    for (const { slug } of findMentions(plain)) {
      if (!kept.includes(slug) && kept.length < MAX_MENTIONS) kept.push(slug);
    }
  }
  return kept;
}

/** The @ and the slug typed so far before the caret, or null when the caret is not in a mention. */
export function mentionAt(text: string, caret: number): { start: number; query: string } | null {
  let start = caret;
  while (start > 0 && /[a-z0-9]/.test(text[start - 1])) start--;
  if (start === 0 || text[start - 1] !== '@') return null;
  const query = text.slice(start, caret);
  if (query !== '' && !/^[a-z][a-z0-9]{0,15}$/.test(query)) return null;
  return mentionStarts(text, start - 1) ? { start: start - 1, query } : null;
}

/** The tribe servers whose slug starts with query, as listbox options. */
export function mentionChoices(servers: MCPServerMention[], query: string): SearchOption[] {
  return servers
    .filter((server) => server.slug.startsWith(query))
    .map((server) => ({
      value: server.slug,
      label: server.slug,
      detail: server.enabled ? `${server.toolCount} ${server.toolCount === 1 ? 'tool' : 'tools'}` : 'Disabled'
    }));
}

/** Whether a mention names an enabled server, a disabled one, or no server of the tribe. */
export function mentionState(slug: string, servers: MCPServerMention[]): 'known' | 'disabled' | 'unknown' {
  const server = servers.find((item) => item.slug === slug);
  return !server ? 'unknown' : server.enabled ? 'known' : 'disabled';
}
