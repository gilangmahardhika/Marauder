import { getNotifications, readNotifications } from '$lib/api';
import { day } from '$lib/format';
import { session } from '$lib/session';
import type { AppNotification } from '$lib/types';
import type { IconName } from '$lib/ui/icons';

/** The viewer's newest notifications and unread count, shared by the bell and the notifications page. */
export const inbox = $state({ items: [] as AppNotification[], unread: 0, failed: false });

/** Only a path on this site is followed. */
export function notePath(url: string) {
  return url.startsWith('/') && !url.startsWith('//') ? url : '/';
}

/** Reads the newest notifications, or only unread ones; the full list also replaces `inbox.items`. */
export async function fetchInbox(unread = false) {
  const body = await getNotifications(unread);
  inbox.unread = body.unread;
  if (!unread) {
    inbox.items = body.notifications;
    inbox.failed = false;
  }
  return body.notifications;
}

export async function loadInbox() {
  try {
    await fetchInbox();
  } catch {
    inbox.failed = true;
  }
}

/** A `notification` frame: the row joins the top of the list. */
export function receive(note: AppNotification, unread: number) {
  inbox.items = [note, ...inbox.items.filter((item) => item.id !== note.id)].slice(0, 50);
  inbox.unread = unread;
}

/** Marks the unread notes read with one request. */
export async function markManyRead(notes: AppNotification[]) {
  const unread = notes.filter((note) => note.readAt === null);
  if (unread.length === 0) return;
  const now = new Date().toISOString();
  const ids = unread.map((note) => note.id);
  for (const note of unread) note.readAt = now;
  for (const item of inbox.items) if (ids.includes(item.id)) item.readAt ??= now;
  inbox.unread = Math.max(0, inbox.unread - ids.length);
  try {
    inbox.unread = (await readNotifications({ ids })).unread;
  } catch {
    /* The next read of the list corrects the count. */
  }
}

/** A card in another project loads the page fresh, so the whole shell switches to that project. `also` are repeats shown in the same row. */
export async function follow(event: MouseEvent, note: AppNotification, also: AppNotification[] = []) {
  const notes = [note, ...also];
  if (note.projectId === session.project()) {
    void markManyRead(notes);
    return;
  }
  event.preventDefault();
  await markManyRead(notes);
  window.location.assign(notePath(note.url));
}

/** Marks every notification read, in the inbox and in any other list on screen. */
export async function markAllRead(lists: AppNotification[][] = []) {
  const now = new Date().toISOString();
  for (const list of [inbox.items, ...lists]) for (const item of list) item.readAt ??= now;
  inbox.unread = 0;
  try {
    inbox.unread = (await readNotifications({ all: true })).unread;
  } catch {
    void loadInbox();
  }
}

export type KindLook = { icon: IconName; chip: string };

const neutral: KindLook = { icon: 'bell', chip: 'bg-ground text-muted' };

const looks: Record<string, KindLook> = {
  review: { icon: 'eye', chip: 'bg-sky-50 text-sky-700' },
  question: { icon: 'message', chip: 'bg-sky-50 text-sky-700' },
  ship: { icon: 'upload', chip: 'bg-emerald-50 text-emerald-700' },
  approved: { icon: 'check', chip: 'bg-emerald-50 text-emerald-700' },
  done: { icon: 'check', chip: 'bg-emerald-50 text-emerald-700' },
  changes_requested: { icon: 'pencil', chip: 'bg-amber-50 text-amber-700' },
  review_failed: { icon: 'alert', chip: 'bg-amber-50 text-amber-700' },
  blocked: { icon: 'alert', chip: 'bg-red-50 text-danger' },
  owner_github: { icon: 'alert', chip: 'bg-red-50 text-danger' },
  reopened: { icon: 'refresh', chip: neutral.chip },
  wont_fix: { icon: 'ban', chip: neutral.chip }
};

export function kindLook(kind: string): KindLook {
  return looks[kind] ?? neutral;
}

export type DayGroup = { key: string; label: string; items: AppNotification[] };

/** Groups rows by local day in list order: "Today", "Yesterday", else the date. */
export function dayGroups(items: AppNotification[], now = Date.now()): DayGroup[] {
  const today = new Date(now);
  const yesterday = new Date(now);
  yesterday.setDate(yesterday.getDate() - 1);
  const groups = new Map<string, DayGroup>();
  for (const item of items) {
    const date = new Date(item.createdAt);
    const key = Number.isNaN(date.getTime()) ? item.createdAt : date.toDateString();
    let group = groups.get(key);
    if (!group) {
      const label = key === today.toDateString() ? 'Today' : key === yesterday.toDateString() ? 'Yesterday' : day(item.createdAt);
      group = { key, label, items: [] };
      groups.set(key, group);
    }
    group.items.push(item);
  }
  return [...groups.values()];
}

/** One row per card and kind; `note` is the newest and `notes` holds it and its repeats. */
export type NoteRow = { note: AppNotification; notes: AppNotification[]; count: number; unread: boolean };

/** Merges notes of the same card and kind, keeping list order by the newest of each. */
export function collapseRepeats(items: AppNotification[]): NoteRow[] {
  const rows = new Map<string, NoteRow>();
  for (const item of items) {
    const key = `${item.cardId}\u0000${item.kind}`;
    const row = rows.get(key);
    if (row) {
      row.notes.push(item);
      row.count += 1;
      row.unread ||= item.readAt === null;
    } else {
      rows.set(key, { note: item, notes: [item], count: 1, unread: item.readAt === null });
    }
  }
  return [...rows.values()];
}

export type Facet = { value: string; label: string; count: number };

/** The project name ends the body, after the last " · ". */
function projectLabel(note: AppNotification) {
  const at = note.body.lastIndexOf(' · ');
  return at === -1 ? note.projectId : note.body.slice(at + 3).trim() || note.projectId;
}

function count(items: AppNotification[], key: (item: AppNotification) => string, label: (item: AppNotification) => string): Facet[] {
  const facets = new Map<string, Facet>();
  for (const item of items) {
    const value = key(item);
    const facet = facets.get(value);
    if (facet) facet.count += 1;
    else facets.set(value, { value, label: label(item), count: 1 });
  }
  return [...facets.values()].sort((a, b) => b.count - a.count || a.label.localeCompare(b.label));
}

/** Counts by kind, labelled by the newest title of that kind, and by project. */
export function facets(items: AppNotification[]) {
  return {
    kinds: count(items, (item) => item.kind, (item) => item.title),
    projects: count(items, (item) => item.projectId, projectLabel)
  };
}
