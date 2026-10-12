export type DiffPart = { kind: 'same' | 'add' | 'del'; text: string };

/** Above this many LCS cells the changed middle shows as one removal and one addition. */
const MAX_CELLS = 2_000_000;

/** Word diff of two texts. Whitespace runs are tokens, so joining the parts of one side gives that side back. */
export function diffWords(before: string, after: string): DiffPart[] {
  return merge(diff(tokens(before), tokens(after)), '');
}

/** Line diff of two lists, such as acceptance lines. Each part is one line. */
export function diffLines(before: string[], after: string[]): DiffPart[] {
  return diff(before, after);
}

function tokens(text: string): string[] {
  return text.split(/(\s+)/).filter((token) => token !== '');
}

function diff(a: string[], b: string[]): DiffPart[] {
  let start = 0;
  while (start < a.length && start < b.length && a[start] === b[start]) start++;
  let endA = a.length;
  let endB = b.length;
  while (endA > start && endB > start && a[endA - 1] === b[endB - 1]) {
    endA--;
    endB--;
  }
  const head = a.slice(0, start).map((text): DiffPart => ({ kind: 'same', text }));
  const tail = a.slice(endA).map((text): DiffPart => ({ kind: 'same', text }));
  return [...head, ...middle(a.slice(start, endA), b.slice(start, endB)), ...tail];
}

function middle(a: string[], b: string[]): DiffPart[] {
  const n = a.length;
  const m = b.length;
  if (n === 0) return b.map((text) => ({ kind: 'add', text }));
  if (m === 0) return a.map((text) => ({ kind: 'del', text }));
  if ((n + 1) * (m + 1) > MAX_CELLS) {
    return [...a.map((text): DiffPart => ({ kind: 'del', text })), ...b.map((text): DiffPart => ({ kind: 'add', text }))];
  }
  const width = m + 1;
  const table = new Uint32Array((n + 1) * width);
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      table[i * width + j] =
        a[i] === b[j] ? table[(i + 1) * width + j + 1] + 1 : Math.max(table[(i + 1) * width + j], table[i * width + j + 1]);
    }
  }
  const out: DiffPart[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      out.push({ kind: 'same', text: a[i] });
      i++;
      j++;
    } else if (table[(i + 1) * width + j] >= table[i * width + j + 1]) {
      out.push({ kind: 'del', text: a[i++] });
    } else {
      out.push({ kind: 'add', text: b[j++] });
    }
  }
  while (i < n) out.push({ kind: 'del', text: a[i++] });
  while (j < m) out.push({ kind: 'add', text: b[j++] });
  return out;
}

/** Joins neighbouring parts of the same kind. */
function merge(parts: DiffPart[], separator: string): DiffPart[] {
  const out: DiffPart[] = [];
  for (const part of parts) {
    const last = out[out.length - 1];
    if (last && last.kind === part.kind) last.text += separator + part.text;
    else out.push({ ...part });
  }
  return out;
}

/** Counts added and removed parts, for a summary line. */
export function changeCount(parts: DiffPart[]): { added: number; removed: number } {
  return {
    added: parts.filter((part) => part.kind === 'add').length,
    removed: parts.filter((part) => part.kind === 'del').length
  };
}
