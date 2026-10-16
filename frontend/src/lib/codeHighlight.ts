/**
 * Lightweight syntax highlighting for terminal edit cards (RFC-132).
 * Small language set without an external highlighter package.
 */

export type HighlightLang = 'typescript' | 'javascript' | 'css' | 'html' | 'go' | 'json' | 'markdown' | 'svelte' | 'plaintext';

const extLang: Record<string, HighlightLang> = {
  ts: 'typescript',
  tsx: 'typescript',
  mts: 'typescript',
  cts: 'typescript',
  js: 'javascript',
  jsx: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  css: 'css',
  html: 'html',
  htm: 'html',
  go: 'go',
  json: 'json',
  md: 'markdown',
  markdown: 'markdown',
  svelte: 'svelte'
};

/** Language for a path from its extension. */
export function langFromPath(path: string): HighlightLang {
  const base = path.split(/[/\\]/).pop() ?? path;
  const dot = base.lastIndexOf('.');
  if (dot < 0) return 'plaintext';
  return extLang[base.slice(dot + 1).toLowerCase()] ?? 'plaintext';
}

type Kind = 'keyword' | 'string' | 'comment' | 'number' | 'type' | 'plain';

function setOf(...words: string[]) {
  return new Set(words);
}

const keywords: Record<HighlightLang, Set<string> | null> = {
  typescript: setOf(
    'abstract', 'as', 'async', 'await', 'break', 'case', 'catch', 'class', 'const', 'continue', 'debugger', 'default',
    'delete', 'do', 'else', 'enum', 'export', 'extends', 'false', 'finally', 'for', 'from', 'function', 'get', 'if',
    'implements', 'import', 'in', 'instanceof', 'interface', 'let', 'new', 'null', 'of', 'package', 'private',
    'protected', 'public', 'return', 'set', 'static', 'super', 'switch', 'this', 'throw', 'true', 'try', 'type',
    'typeof', 'undefined', 'var', 'void', 'while', 'with', 'yield'
  ),
  javascript: setOf(
    'async', 'await', 'break', 'case', 'catch', 'class', 'const', 'continue', 'debugger', 'default', 'delete', 'do',
    'else', 'export', 'extends', 'false', 'finally', 'for', 'from', 'function', 'if', 'import', 'in', 'instanceof',
    'let', 'new', 'null', 'of', 'return', 'static', 'super', 'switch', 'this', 'throw', 'true', 'try', 'typeof',
    'undefined', 'var', 'void', 'while', 'with', 'yield'
  ),
  go: setOf(
    'break', 'case', 'chan', 'const', 'continue', 'default', 'defer', 'else', 'fallthrough', 'for', 'func', 'go',
    'goto', 'if', 'import', 'interface', 'map', 'package', 'range', 'return', 'select', 'struct', 'switch', 'type',
    'var', 'true', 'false', 'nil', 'iota'
  ),
  css: setOf('and', 'not', 'only', 'or', 'important', 'from', 'to'),
  html: null,
  json: setOf('true', 'false', 'null'),
  markdown: null,
  svelte: setOf(
    'as', 'async', 'await', 'break', 'case', 'catch', 'class', 'const', 'continue', 'else', 'export', 'false', 'for',
    'from', 'function', 'if', 'import', 'let', 'new', 'null', 'of', 'return', 'true', 'try', 'type', 'typeof',
    'undefined', 'var', 'void', 'while'
  ),
  plaintext: null
};

const types = setOf(
  'string', 'number', 'boolean', 'any', 'unknown', 'never', 'object', 'symbol', 'bigint', 'Promise', 'Array',
  'Record', 'Map', 'Set', 'Error', 'HTMLElement', 'ParamMatcher'
);

/** Escapes text for safe HTML text nodes. */
export function escapeHtml(text: string): string {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function span(kind: Kind, text: string): string {
  if (kind === 'plain' || text === '') return escapeHtml(text);
  return `<span class="hl-${kind}">${escapeHtml(text)}</span>`;
}

/**
 * Highlights one line of source. Returns HTML with hl-* spans. Input must be a
 * single line (no newlines); the caller splits the file.
 */
export function highlightLine(line: string, lang: HighlightLang): string {
  if (lang === 'plaintext' || line === '') return escapeHtml(line);
  if (lang === 'html' || lang === 'svelte') return highlightMarkup(line, lang === 'svelte');
  if (lang === 'css') return highlightCss(line);
  if (lang === 'json') return highlightJson(line);
  if (lang === 'markdown') return highlightMarkdown(line);
  return highlightCode(line, lang);
}

function highlightCode(line: string, lang: HighlightLang): string {
  const out: string[] = [];
  let i = 0;
  const kw = keywords[lang];
  while (i < line.length) {
    if (line.startsWith('//', i)) {
      out.push(span('comment', line.slice(i)));
      break;
    }
    if (line.startsWith('/*', i)) {
      const end = line.indexOf('*/', i + 2);
      const stop = end >= 0 ? end + 2 : line.length;
      out.push(span('comment', line.slice(i, stop)));
      i = stop;
      continue;
    }
    const q = line[i];
    if (q === '"' || q === "'" || q === '`') {
      let j = i + 1;
      while (j < line.length) {
        if (line[j] === '\\') {
          j += 2;
          continue;
        }
        if (line[j] === q) {
          j++;
          break;
        }
        j++;
      }
      out.push(span('string', line.slice(i, j)));
      i = j;
      continue;
    }
    if (/[0-9]/.test(line[i]!) && (i === 0 || /[^\w$]/.test(line[i - 1]!))) {
      let j = i;
      while (j < line.length && /[0-9._xXa-fA-F]/.test(line[j]!)) j++;
      out.push(span('number', line.slice(i, j)));
      i = j;
      continue;
    }
    if (/[\w$]/.test(line[i]!)) {
      let j = i;
      while (j < line.length && /[\w$]/.test(line[j]!)) j++;
      const word = line.slice(i, j);
      if (kw?.has(word)) out.push(span('keyword', word));
      else if ((lang === 'typescript' || lang === 'svelte') && types.has(word)) out.push(span('type', word));
      else out.push(span('plain', word));
      i = j;
      continue;
    }
    out.push(span('plain', line[i]!));
    i++;
  }
  return out.join('');
}

function highlightMarkup(line: string, svelte: boolean): string {
  if (svelte && /^\s*[{}]/.test(line)) return highlightCode(line, 'svelte');
  let out = '';
  let i = 0;
  while (i < line.length) {
    if (line[i] === '<') {
      const close = line.indexOf('>', i);
      const end = close >= 0 ? close + 1 : line.length;
      const tag = line.slice(i, end);
      const m = /^<\/?([a-zA-Z][\w:-]*)/.exec(tag);
      if (m) {
        out += escapeHtml(tag.slice(0, m.index! + (tag.startsWith('</') ? 2 : 1)));
        out += span('keyword', m[1]!);
        const rest = tag.slice(m[0].length);
        out += rest.replace(/([a-zA-Z_:][\w:.-]*)(=("[^"]*"|'[^']*'|[^\s/>]+))?/g, (_a, an, eq, av) =>
          eq ? span('type', an) + '=' + span('string', av) : span('type', an)
        );
      } else {
        out += escapeHtml(tag);
      }
      i = end;
      continue;
    }
    out += escapeHtml(line[i]!);
    i++;
  }
  return out;
}

function highlightCss(line: string): string {
  if (line.trimStart().startsWith('/*') || line.trimStart().startsWith('*')) return span('comment', line);
  return highlightCode(line, 'css').replace(/(@[\w-]+)/g, (_a, at) => span('keyword', at));
}

function highlightJson(line: string): string {
  return line.replace(/("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?/g, (all, str, colon, kw) => {
    if (str && colon) return span('type', str) + colon;
    if (str) return span('string', str);
    if (kw) return span('keyword', kw);
    return span('number', all);
  });
}

function highlightMarkdown(line: string): string {
  if (/^#{1,6}\s/.test(line)) return span('keyword', line);
  if (/^>\s/.test(line) || /^[-*+]\s/.test(line)) return span('comment', line);
  if (/^```/.test(line)) return span('keyword', line);
  return escapeHtml(line).replace(/`([^`]+)`/g, (_a, code) => `<span class="hl-string">\`${escapeHtml(code)}\`</span>`);
}
