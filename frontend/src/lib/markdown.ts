// A small markdown reader for requirement statements. It returns a token tree
// that Markdown.svelte renders as elements and text nodes; nothing here builds
// HTML. Anything outside the supported subset stays literal text.

export type Inline =
  | { type: 'text'; text: string }
  | { type: 'code'; text: string }
  | { type: 'strong'; children: Inline[] }
  | { type: 'em'; children: Inline[] }
  | { type: 'link'; href: string; children: Inline[] }
  | { type: 'br' };

export type ListItem = { children: Inline[]; sublists: List[] };
export type List = { type: 'list'; ordered: boolean; start: number; items: ListItem[] };

export type MdBlock =
  | { type: 'paragraph'; children: Inline[] }
  | { type: 'heading'; level: number; children: Inline[] }
  | List
  | { type: 'code'; lang: string; text: string }
  | { type: 'quote'; children: MdBlock[] }
  | { type: 'rule' };

export type Doc = { blocks: MdBlock[]; cut: boolean };

const MAX_DEPTH = 8;
const MAX_QUOTE = 4;
const SCAN_LIMIT = 2000;

const FENCE = /^ {0,3}(`{3,}|~{3,})(.*)$/;
const HEADING = /^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$/;
const RULE = /^ {0,3}([-*_])(?:[ \t]*\1){2,}[ \t]*$/;
const QUOTE = /^ {0,3}> ?(.*)$/;
const ITEM = /^( {0,3})([-*+]|\d{1,9}[.)])[ \t]+(.*)$/;
const ITEM_ANY = /^( *)([-*+]|\d{1,9}[.)])[ \t]+(.*)$/;
const PUNCT = /[!-/:-@[-`{-~]/;
const WORD = /[\p{L}\p{N}]/u;
const SPACE = /\s/;
const AUTOLINK = /<((?:https?:\/\/|mailto:)[^\s<>]*)>/iy;
const CONTROL = /[\u0000-\u0008\u000b-\u001f\u007f]/g;

/** Normalizes line endings, drops control characters except newline and tab, and cuts at cap characters. */
export function cleanMarkdown(raw: string, cap: number): { text: string; cut: boolean } {
  const text = raw.replace(/\r\n?/g, '\n').replace(CONTROL, '');
  return text.length > cap ? { text: text.slice(0, cap), cut: true } : { text, cut: false };
}

/** A link target is kept only under the client's link rule: GitHub or the local harness. */
export function safeHref(raw: string): string | null {
  const href = raw.trim();
  if (href === '' || /[\s\u0000-\u001f\u007f]/.test(href)) return null;
  if (href.startsWith('https://github.com/')) return href;
  if (/^http:\/\/127\.0\.0\.1(?::\d{1,5})?(?:[/?#]|$)/.test(href)) return href;
  return null;
}

export function parse(raw: string, cap = 20_000): Doc {
  const { text, cut } = cleanMarkdown(raw, cap);
  return { blocks: parseBlocks(text.replace(/\t/g, '    ').split('\n'), 0), cut };
}

function fenceOpen(line: string): { mark: string; lang: string } | null {
  const match = FENCE.exec(line);
  if (!match) return null;
  const [, mark, rest] = match;
  if (mark[0] === '`' && rest.includes('`')) return null;
  return { mark, lang: rest.trim().split(/\s+/)[0] ?? '' };
}

function fenceCloses(line: string, mark: string): boolean {
  return new RegExp(`^ {0,3}\\${mark[0]}{${mark.length},}[ \\t]*$`).test(line);
}

function startsBlock(line: string, depth: number): boolean {
  return (
    fenceOpen(line) !== null ||
    HEADING.test(line) ||
    RULE.test(line) ||
    (depth < MAX_QUOTE && QUOTE.test(line)) ||
    ITEM.test(line)
  );
}

function parseBlocks(lines: string[], depth: number): MdBlock[] {
  const out: MdBlock[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.trim() === '') {
      i++;
      continue;
    }
    const fence = fenceOpen(line);
    if (fence) {
      const body: string[] = [];
      i++;
      while (i < lines.length && !fenceCloses(lines[i], fence.mark)) body.push(lines[i++]);
      i++;
      out.push({ type: 'code', lang: fence.lang, text: body.join('\n') });
      continue;
    }
    const heading = HEADING.exec(line);
    if (heading) {
      out.push({ type: 'heading', level: heading[1].length, children: parseInline(heading[2] ?? '') });
      i++;
      continue;
    }
    if (RULE.test(line)) {
      out.push({ type: 'rule' });
      i++;
      continue;
    }
    if (depth < MAX_QUOTE && QUOTE.test(line)) {
      const inner: string[] = [];
      for (let match = QUOTE.exec(lines[i]); match && i < lines.length; match = QUOTE.exec(lines[++i] ?? '')) {
        inner.push(match[1]);
      }
      out.push({ type: 'quote', children: parseBlocks(inner, depth + 1) });
      continue;
    }
    if (ITEM.test(line)) {
      const [list, next] = parseList(lines, i);
      out.push(list);
      i = next;
      continue;
    }
    const para = [line];
    i++;
    while (i < lines.length && lines[i].trim() !== '' && !startsBlock(lines[i], depth)) para.push(lines[i++]);
    out.push({ type: 'paragraph', children: parseInline(para.map((part) => part.replace(/^ +/, '')).join('\n')) });
  }
  return out;
}

type ItemDraft = { text: string[]; sub: string[] };

function parseList(lines: string[], start: number): [List, number] {
  const first = ITEM_ANY.exec(lines[start])!;
  const base = first[1].length;
  const ordered = /\d/.test(first[2]);
  const family = ordered ? first[2].slice(-1) : first[2];
  const sameFamily = (marker: string) =>
    ordered ? /\d/.test(marker) && marker.slice(-1) === family : marker === family;
  const isTop = (line: string) => {
    const match = ITEM_ANY.exec(line);
    return match !== null && match[1].length <= base + 1 && sameFamily(match[2]) && !RULE.test(line);
  };
  const isNested = (line: string) => {
    const match = ITEM_ANY.exec(line);
    return match !== null && match[1].length >= base + 2;
  };

  const drafts: ItemDraft[] = [];
  let i = start;
  while (i < lines.length) {
    const line = lines[i];
    if (line.trim() === '') {
      const next = lines[i + 1];
      if (next !== undefined && (isTop(next) || isNested(next))) {
        i++;
        continue;
      }
      break;
    }
    const current = drafts[drafts.length - 1];
    if (isTop(line)) {
      drafts.push({ text: [ITEM_ANY.exec(line)![3]], sub: [] });
      i++;
      continue;
    }
    if (current && isNested(line)) {
      current.sub.push(line.replace(/^ +/, ''));
      i++;
      continue;
    }
    const indented = /^ /.test(line) && line.search(/\S/) >= base + 2;
    if (!current || (!indented && startsBlock(line, 0))) break;
    if (current.sub.length > 0) current.sub.push(line.trim());
    else current.text.push(line.trim());
    i++;
  }

  const items = drafts.map((draft) => ({
    children: parseInline(draft.text.join('\n')),
    sublists: sublists(draft.sub)
  }));
  return [{ type: 'list', ordered, start: ordered ? parseInt(first[2], 10) : 1, items }, i];
}

/** Items nested deeper than one level are flattened into the first nested level. */
function sublists(lines: string[]): List[] {
  const out: List[] = [];
  let i = 0;
  while (i < lines.length) {
    if (ITEM_ANY.test(lines[i]) && !RULE.test(lines[i])) {
      const flat = lines.slice(i).map((line) => line.replace(/^ +/, ''));
      const [list, used] = parseList(flat, 0);
      out.push({ ...list, items: list.items.map((item) => ({ ...item, sublists: [] })) });
      i += Math.max(used, 1);
    } else {
      i++;
    }
  }
  return out;
}

function runLength(src: string, at: number, char: string): number {
  let end = at;
  while (src[end] === char) end++;
  return end - at;
}

function codeClose(src: string, from: number, run: number): number {
  let j = from;
  while (j < src.length) {
    if (src[j] === '`') {
      const length = runLength(src, j, '`');
      if (length === run) return j;
      j += length;
    } else {
      j++;
    }
  }
  return -1;
}

function bracketClose(src: string, open: number): number {
  let depth = 0;
  const limit = Math.min(src.length, open + SCAN_LIMIT);
  for (let j = open; j < limit; j++) {
    const char = src[j];
    if (char === '\\') j++;
    else if (char === '[') depth++;
    else if (char === ']' && --depth === 0) return j;
  }
  return -1;
}

function parenClose(src: string, open: number): number {
  let depth = 0;
  const limit = Math.min(src.length, open + SCAN_LIMIT);
  for (let j = open; j < limit; j++) {
    const char = src[j];
    if (char === '\n') return -1;
    if (char === '(') depth++;
    else if (char === ')' && --depth === 0) return j;
  }
  return -1;
}

/** Finds where an emphasis run of `need` delimiters closes; -1 when it does not. */
function emphasisClose(src: string, from: number, char: string, need: number): number {
  let j = from;
  while (j < src.length) {
    const at = src[j];
    if (at === '\\') {
      j += 2;
      continue;
    }
    if (at === '`') {
      const run = runLength(src, j, '`');
      const close = codeClose(src, j + run, run);
      j = close >= 0 ? close + run : j + run;
      continue;
    }
    if (at === char) {
      const run = runLength(src, j, char);
      const before = src[j - 1] ?? ' ';
      const after = src[j + run] ?? ' ';
      const closes = !SPACE.test(before) && !(char === '_' && WORD.test(after));
      const fits = need === 1 ? run === 1 || run >= 3 : run >= need;
      if (closes && fits && j > from) return j + run - need;
      j += run;
      continue;
    }
    j++;
  }
  return -1;
}

export function parseInline(src: string, depth = 0): Inline[] {
  const out: Inline[] = [];
  const failed = new Set<string>();
  let buf = '';

  const flush = () => {
    if (buf === '') return;
    const last = out[out.length - 1];
    if (last?.type === 'text') last.text += buf;
    else out.push({ type: 'text', text: buf });
    buf = '';
  };
  const push = (node: Inline) => {
    flush();
    if (node.type === 'text') {
      buf = node.text;
      flush();
    } else {
      out.push(node);
    }
  };

  let i = 0;
  while (i < src.length) {
    const char = src[i];

    if (char === '\\') {
      const next = src[i + 1];
      if (next === '\n') {
        push({ type: 'br' });
        i += 2;
      } else if (next !== undefined && PUNCT.test(next)) {
        buf += next;
        i += 2;
      } else {
        buf += char;
        i++;
      }
      continue;
    }

    if (char === '`') {
      const run = runLength(src, i, '`');
      const close = failed.has(`code${run}`) ? -1 : codeClose(src, i + run, run);
      if (close < 0) {
        failed.add(`code${run}`);
        buf += src.slice(i, i + run);
        i += run;
        continue;
      }
      let code = src.slice(i + run, close).replace(/\n/g, ' ');
      if (code.length > 2 && code.startsWith(' ') && code.endsWith(' ') && code.trim() !== '') code = code.slice(1, -1);
      push({ type: 'code', text: code });
      i = close + run;
      continue;
    }

    if (char === '\n') {
      if (/ {2,}$/.test(buf)) {
        buf = buf.replace(/ +$/, '');
        push({ type: 'br' });
      } else {
        buf = buf.replace(/ +$/, '') + ' ';
      }
      i++;
      while (src[i] === ' ') i++;
      continue;
    }

    if (char === '<') {
      AUTOLINK.lastIndex = i;
      const match = AUTOLINK.exec(src);
      if (match) {
        const href = safeHref(match[1]);
        if (href) push({ type: 'link', href, children: [{ type: 'text', text: match[1] }] });
        else buf += match[1];
        i += match[0].length;
        continue;
      }
      buf += char;
      i++;
      continue;
    }

    if (char === '[' && depth < MAX_DEPTH) {
      const end = bracketClose(src, i);
      const close = end > 0 && src[end + 1] === '(' ? parenClose(src, end + 1) : -1;
      const url =
        close > 0
          ? (src
              .slice(end + 2, close)
              .trim()
              .split(/\s+/)[0] ?? '')
          : '';
      if (url !== '') {
        const label = parseInline(src.slice(i + 1, end), depth + 1);
        const href = safeHref(url);
        if (href) {
          push({ type: 'link', href, children: label });
        } else {
          label.forEach(push);
          buf += ` (${url})`;
        }
        i = close + 1;
        continue;
      }
      buf += char;
      i++;
      continue;
    }

    if ((char === '*' || char === '_') && depth < MAX_DEPTH) {
      const run = runLength(src, i, char);
      const before = src[i - 1] ?? ' ';
      const after = src[i + run] ?? ' ';
      const opens = !SPACE.test(after) && !(char === '_' && WORD.test(before));
      let matched = false;
      if (opens) {
        for (const need of run >= 3 ? [3, 2, 1] : run === 2 ? [2, 1] : [1]) {
          const key = `${char}${need}`;
          if (failed.has(key)) continue;
          const start = i + run;
          const close = emphasisClose(src, start, char, need);
          if (close < 0) {
            failed.add(key);
            continue;
          }
          buf += src.slice(i, i + run - need);
          const inner = parseInline(src.slice(start, close), depth + 1);
          const node: Inline =
            need === 3
              ? { type: 'strong', children: [{ type: 'em', children: inner }] }
              : need === 2
                ? { type: 'strong', children: inner }
                : { type: 'em', children: inner };
          push(node);
          i = close + need;
          matched = true;
          break;
        }
      }
      if (!matched) {
        buf += src.slice(i, i + run);
        i += run;
      }
      continue;
    }

    buf += char;
    i++;
  }
  flush();
  return out;
}

function inlineText(nodes: Inline[]): string {
  return nodes
    .map((node) => {
      if (node.type === 'text' || node.type === 'code') return node.text;
      if (node.type === 'br') return ' ';
      return inlineText(node.children);
    })
    .join('');
}

function listText(list: List): string {
  return list.items
    .map((item) => [inlineText(item.children), ...item.sublists.map(listText)].filter(Boolean).join(' · '))
    .join(' · ');
}

function blockText(block: MdBlock): string {
  switch (block.type) {
    case 'paragraph':
    case 'heading':
      return inlineText(block.children);
    case 'list':
      return listText(block);
    case 'code':
      return block.text;
    case 'quote':
      return block.children.map(blockText).join(' ');
    case 'rule':
      return '';
  }
}

/** The statement as one line of words, for clamped cells and search. */
export function plainText(raw: string, cap = 2000): string {
  return parse(raw, cap).blocks.map(blockText).filter(Boolean).join(' ').replace(/\s+/g, ' ').trim();
}

const TASK = /^\[[ xX]\](?:[ \t]+|$)/;
const EMPTY_ITEM = /^ *(?:[-*+]|\d{1,9}[.)])[ \t]*$/;
const LABEL = /^ {0,3}(\*\*|__).+\1:?[ \t]*$/;

/**
 * The items of every list in raw, each kept as its own inline markdown on one line, or null when raw has
 * no list item. A wrapped item joins with a space and a nested item is its own entry. Headings, labels,
 * prose, and fenced code are left out and counted in skipped; rules are left out silently.
 */
export function listItems(raw: string, cap = 20_000): { items: string[]; skipped: number } | null {
  const lines = cleanMarkdown(raw, cap).text.replace(/\t/g, '    ').split('\n');
  const items: string[] = [];
  let skipped = 0;
  let fence: string | null = null;
  let open = false;
  let gap = false;
  for (const line of lines) {
    if (fence !== null) {
      if (fenceCloses(line, fence)) fence = null;
      skipped++;
      continue;
    }
    if (line.trim() === '') {
      gap = true;
      continue;
    }
    const opened = fenceOpen(line);
    const item = opened || RULE.test(line) ? null : (ITEM_ANY.exec(line) ?? EMPTY_ITEM.exec(line));
    if (opened) {
      fence = opened.mark;
      skipped++;
      open = false;
    } else if (RULE.test(line)) {
      open = false;
    } else if (item) {
      const text = (item[3] ?? '').replace(TASK, '').trim();
      if (text !== '') items.push(text);
      open = text !== '';
    } else if (open && (gap ? /^ {2,}\S/.test(line) : !startsBlock(line, 0) && !LABEL.test(line))) {
      items[items.length - 1] += ' ' + line.trim();
    } else {
      skipped++;
      open = false;
    }
    gap = false;
  }
  return items.length > 0 ? { items, skipped } : null;
}
