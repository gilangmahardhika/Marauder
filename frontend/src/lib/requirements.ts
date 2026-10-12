import { listItems } from './markdown';
import { displayName } from './people';
import type { Reviewer } from './types';
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

/** Only a Lead Developer or an admin may turn Self review on or off (RFC-070). */
export function canSetSelfReview(role: string, admin: boolean): boolean {
  return admin || role === 'lead_developer';
}

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
