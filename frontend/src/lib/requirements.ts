import { displayName } from './people';
import type { Reviewer } from './types';
import type { SearchOption } from './ui/SearchSelect.svelte';

/** Splits a textarea into acceptance lines, dropping blank ones. */
export function toLines(text: string): string[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '');
}

export function fromLines(lines: string[]): string {
  return lines.join('\n');
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
