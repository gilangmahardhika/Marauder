export const pageSize = 50;

export type Option<T extends string = string> = { value: T; label: string; count?: number };

export function matchText(parts: (string | null | undefined)[], q: string) {
  const needle = q.trim().toLowerCase();
  return needle === '' || parts.some((part) => (part ?? '').toLowerCase().includes(needle));
}

export function withParams(url: URL, updates: Record<string, string>) {
  const next = new URL(url);
  for (const [key, value] of Object.entries(updates)) {
    if (value === '') next.searchParams.delete(key);
    else next.searchParams.set(key, value);
  }
  return next;
}

export function pickParam<T extends string>(value: string | null, allowed: readonly T[], fallback: T): T {
  return value !== null && (allowed as readonly string[]).includes(value) ? (value as T) : fallback;
}
