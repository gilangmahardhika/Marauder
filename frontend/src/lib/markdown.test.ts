import { describe, expect, it } from 'vitest';
import { listItems, parse, parseInline, plainText, safeHref, type Inline } from './markdown';

const text = (value: string): Inline => ({ type: 'text', text: value });

describe('markdown blocks', () => {
  it('keeps HTML as text', () => {
    expect(parse('<script>alert(1)</script>').blocks).toEqual([
      { type: 'paragraph', children: [text('<script>alert(1)</script>')] }
    ]);
  });

  it('reads headings and leaves non-headings as paragraphs', () => {
    const blocks = parse('# One\n### Three ###\n#123\n####### seven').blocks;
    expect(blocks[0]).toEqual({ type: 'heading', level: 1, children: [text('One')] });
    expect(blocks[1]).toEqual({ type: 'heading', level: 3, children: [text('Three')] });
    expect(blocks[2]).toEqual({ type: 'paragraph', children: [text('#123 ####### seven')] });
  });

  it('keeps fenced code literal and runs an unclosed fence to the end', () => {
    expect(parse('```ts\n**x**\n# y\n```\nafter').blocks).toEqual([
      { type: 'code', lang: 'ts', text: '**x**\n# y' },
      { type: 'paragraph', children: [text('after')] }
    ]);
    expect(parse('~~~\na\n```\nb').blocks).toEqual([{ type: 'code', lang: '', text: 'a\n```\nb' }]);
  });

  it('reads lists, one nested level, and marker switches', () => {
    const [ordered] = parse('3. three\n4. four').blocks;
    expect(ordered).toMatchObject({
      type: 'list',
      ordered: true,
      start: 3,
      items: [{ children: [text('three')] }, { children: [text('four')] }]
    });

    const [nested] = parse('- a\n  - b\n    - c\n- d').blocks;
    expect(nested).toMatchObject({
      type: 'list',
      items: [
        { children: [text('a')], sublists: [{ items: [{ children: [text('b')] }, { children: [text('c')] }] }] },
        { children: [text('d')], sublists: [] }
      ]
    });

    const split = parse('- a\n+ b').blocks;
    expect(split).toHaveLength(2);
    expect(parse('- a\ncontinued').blocks[0]).toMatchObject({ items: [{ children: [text('a continued')] }] });
  });

  it('tells rules from lists', () => {
    expect(parse('---').blocks).toEqual([{ type: 'rule' }]);
    expect(parse('- - -').blocks).toEqual([{ type: 'rule' }]);
    expect(parse('***').blocks).toEqual([{ type: 'rule' }]);
    expect(parse('- x').blocks[0].type).toBe('list');
  });

  it('reads a blockquote with a list', () => {
    expect(parse('> note\n> - a').blocks).toMatchObject([
      { type: 'quote', children: [{ type: 'paragraph' }, { type: 'list', items: [{ children: [text('a')] }] }] }
    ]);
  });

  it('cleans line endings and control characters, and reports a cut', () => {
    expect(parse('a\u0000b\r\nc').blocks).toEqual([{ type: 'paragraph', children: [text('ab c')] }]);
    expect(parse('abcdef', 3)).toEqual({ blocks: [{ type: 'paragraph', children: [text('abc')] }], cut: true });
  });
});

describe('markdown inline', () => {
  it('keeps only GitHub and local links', () => {
    expect(parseInline('[docs](https://github.com/acme/widget)')).toEqual([
      { type: 'link', href: 'https://github.com/acme/widget', children: [text('docs')] }
    ]);
    for (const url of ['javascript:alert(1)', 'JAVASCRIPT:x', 'data:text/html,x', '/cards/1', 'https://example.com']) {
      expect(parseInline(`[x](${url})`)).toEqual([text(`x (${url})`)]);
    }
    expect(safeHref('http://127.0.0.1:8787/x')).toBe('http://127.0.0.1:8787/x');
    expect(safeHref('http://127.0.0.1.evil.test/')).toBeNull();
    expect(safeHref('http://127.0.0.1:1@evil.test/')).toBeNull();
    expect(safeHref('http://127.0.0.1:x@evil.test')).toBeNull();
    expect(safeHref('http://127.0.0.1')).toBe('http://127.0.0.1');
    expect(parseInline('<https://github.com/a>')).toEqual([
      { type: 'link', href: 'https://github.com/a', children: [text('https://github.com/a')] }
    ]);
    expect(parseInline('<https://example.com>')).toEqual([text('https://example.com')]);
  });

  it('nests emphasis', () => {
    expect(parseInline('**bold *it***')).toEqual([
      { type: 'strong', children: [text('bold '), { type: 'em', children: [text('it')] }] }
    ]);
    expect(parseInline('*a **b** c*')).toEqual([
      { type: 'em', children: [text('a '), { type: 'strong', children: [text('b')] }, text(' c')] }
    ]);
    expect(parseInline('***x***')).toEqual([{ type: 'strong', children: [{ type: 'em', children: [text('x')] }] }]);
  });

  it('leaves loose markers as text', () => {
    for (const value of ['**open', 'snake_case_name', '2 * 3 * 4']) expect(parseInline(value)).toEqual([text(value)]);
    expect(parseInline('\\*x\\*')).toEqual([text('*x*')]);
  });

  it('protects code spans', () => {
    expect(parseInline('`**x**`')).toEqual([{ type: 'code', text: '**x**' }]);
    expect(parseInline('``a`b``')).toEqual([{ type: 'code', text: 'a`b' }]);
  });

  it('turns two trailing spaces or a backslash into a break, and a newline into a space', () => {
    expect(parseInline('a  \nb')).toEqual([text('a'), { type: 'br' }, text('b')]);
    expect(parseInline('a\\\nb')).toEqual([text('a'), { type: 'br' }, text('b')]);
    expect(parseInline('a\nb')).toEqual([text('a b')]);
  });

  it('parses pathological input quickly', () => {
    const started = performance.now();
    parse('*'.repeat(20_000));
    parse('['.repeat(20_000));
    parse('*a '.repeat(7_000));
    expect(performance.now() - started).toBeLessThan(1000);
  });
});

describe('plainText', () => {
  it('drops markers for a one-line excerpt', () => {
    expect(plainText('# Title\n\n- **a**\n- [b](https://x)\n\n```\nx  y\n```')).toBe('Title a · b (https://x) x y');
    expect(plainText('[b](https://github.com/x)')).toBe('b');
  });
});

describe('listItems', () => {
  it('keeps only list items from a pasted definition of done', () => {
    const doc = [
      'Here are the acceptance criteria. They are written to be checkable.',
      '',
      '## Acceptance Criteria / Definition of Done',
      '',
      '**Workflow file exists and is wired up**',
      '- [ ] A workflow file exists at `.github/workflows/lint.yml`.',
      '- [ ] The workflow triggers on **both** `pull_request` and `push` to `main`.',
      '',
      '**PHP checks**',
      '- [ ] Laravel Pint runs in check-only mode (`vendor/bin/pint --test`).',
      '',
      '---',
      '',
      'Want me to fold these into the original prompt?'
    ].join('\r\n');
    expect(listItems(doc)).toEqual({
      items: [
        'A workflow file exists at `.github/workflows/lint.yml`.',
        'The workflow triggers on **both** `pull_request` and `push` to `main`.',
        'Laravel Pint runs in check-only mode (`vendor/bin/pint --test`).'
      ],
      skipped: 5
    });
  });

  it('joins wrapped items, keeps nested items, and skips fenced code', () => {
    const doc =
      '1. first line\n   wraps here\n2) second\n  - nested\n\n   after a blank\n```\n- not an item\n```\n* [x] done\n- [ ]';
    expect(listItems(doc)).toEqual({
      items: ['first line wraps here', 'second', 'nested after a blank', 'done'],
      skipped: 3
    });
  });

  it('does not join a label or heading onto the item above', () => {
    expect(listItems('- a\n**Label**\n- b\n# Head\n- c')).toEqual({ items: ['a', 'b', 'c'], skipped: 2 });
  });

  it('is null without a list item', () => {
    expect(listItems('one\ntwo\n---\n- ')).toBeNull();
  });
});
