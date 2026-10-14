export type ToastTone = 'success' | 'error' | 'info';

export type ToastItem = {
  id: number;
  tone: ToastTone;
  text: string;
  href?: string;
  label?: string;
};

const life: Record<ToastTone, number> = { success: 5000, info: 5000, error: 8000 };
const limit = 3;

const store = $state({ items: [] as ToastItem[] });
const timers = new Map<number, ReturnType<typeof setTimeout>>();
let nextId = 1;

function stop(id: number) {
  clearTimeout(timers.get(id));
  timers.delete(id);
}

function arm(item: ToastItem) {
  stop(item.id);
  timers.set(
    item.id,
    setTimeout(() => dismiss(item.id), life[item.tone])
  );
}

function dismiss(id: number) {
  stop(id);
  store.items = store.items.filter((item) => item.id !== id);
}

function push(tone: ToastTone, text: string, link: { href?: string; label?: string } = {}) {
  const trimmed = text.trim();
  if (trimmed === '') return 0;
  const newest = store.items.at(-1);
  if (newest && newest.tone === tone && newest.text === trimmed) {
    arm(newest);
    return newest.id;
  }
  const item: ToastItem = { id: nextId++, tone, text: trimmed, ...link };
  store.items = [...store.items, item];
  while (store.items.length > limit) dismiss(store.items[0].id);
  arm(item);
  return item.id;
}

/** Outcomes of an action, shown by the root layout's Toaster (REQ-096). */
export const toast = {
  get items() {
    return store.items;
  },
  success: (text: string) => push('success', text),
  error: (text: string) => push('error', text),
  info: (text: string, link?: { href?: string; label?: string }) => push('info', text, link),
  dismiss,
  pause: stop,
  resume(id: number) {
    const item = store.items.find((entry) => entry.id === id);
    if (item) arm(item);
  },
  clear() {
    for (const id of timers.keys()) stop(id);
    store.items = [];
  }
};
