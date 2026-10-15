const clockFormat = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
const dayFormat = new Intl.DateTimeFormat(undefined, { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
const stampFormat = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', hour12: false });
const countFormat = new Intl.NumberFormat();

function parse(ts: string) {
  const date = new Date(ts);
  return Number.isNaN(date.getTime()) ? null : date;
}

/** Local time of day, such as "13:07:55". An unreadable value is returned as is. */
export function clock(ts: string) {
  const date = parse(ts);
  return date ? clockFormat.format(date) : ts;
}

/** Local calendar day, such as "Sun, 11 Oct 2026". */
export function day(ts: string) {
  const date = parse(ts);
  return date ? dayFormat.format(date) : ts;
}

/** Local day and minute, such as "11 Oct, 12:04". */
export function stamp(ts: string) {
  const date = parse(ts);
  return date ? stampFormat.format(date) : ts;
}

/** "Just now", "5m ago", "3h ago", "2d ago", then `stamp` after a week. */
export function ago(ts: string, now = Date.now()) {
  const date = parse(ts);
  if (!date) return ts;
  const minutes = Math.floor((now - date.getTime()) / 60_000);
  if (minutes < 1) return 'Just now';
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}d ago`;
  return stampFormat.format(date);
}

/** Milliseconds between two timestamps, or null when either is unreadable. */
export function between(start: string, end: string) {
  const a = parse(start);
  const b = parse(end);
  return a && b ? b.getTime() - a.getTime() : null;
}

/** "0s", "45s", "1m 8s", "2h 5m". */
export function duration(ms: number) {
  const seconds = Math.max(0, Math.round(ms / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}

/** "950", "1.6k", "290k", "1.2M". */
export function tokens(n: number) {
  if (n < 1000) return String(n);
  if (n < 10_000) return `${(n / 1000).toFixed(1).replace(/\.0$/, '')}k`;
  if (n < 1_000_000) return `${Math.round(n / 1000)}k`;
  return `${(n / 1_000_000).toFixed(1).replace(/\.0$/, '')}M`;
}

/** "1,616". */
export function count(n: number) {
  return countFormat.format(n);
}

/** "512 B", "23 KB", "1.2 MB". */
export function bytes(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1).replace(/\.0$/, '')} MB`;
}
