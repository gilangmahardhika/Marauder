import { describe, expect, it } from 'vitest';
import { mentionAt, mentionChoices, mentionState, mentions, reviewerChoices } from './requirements';

const servers = [
  { slug: 'tickets', enabled: true, toolCount: 2 },
  { slug: 'docs', enabled: false, toolCount: 1 }
];

describe('mentions', () => {
  it('keeps mentions by the RFC-101 grammar', () => {
    expect(mentions('Read @tickets and (@db), then @tickets again.')).toEqual(['tickets', 'db']);
    expect(mentions('Ends with @db.')).toEqual(['db']);
    expect(mentions('**@tickets** is bold')).toEqual(['tickets']);
    expect(mentions('Mail a@b.co, install @scope/pkg, use @pytest.fixture and @Override.')).toEqual([]);
    expect(mentions('Too short @x, too long @abcdefghijklmnopq, not @db_x')).toEqual([]);
  });

  it('skips code, fences, and links', () => {
    expect(mentions('Code `@tickets` and ``a ` @twice`` stay code')).toEqual([]);
    expect(mentions('```\n@tickets\n```\nafter @docs')).toEqual(['docs']);
    expect(mentions('[@tickets](https://example.com) and <https://x.dev/@tickets>')).toEqual([]);
  });

  it('keeps at most 16', () => {
    const text = Array.from({ length: 20 }, (_, i) => `@s${String(i).padStart(2, '0')}`).join(' ');
    expect(mentions(text)).toHaveLength(16);
  });
});

describe('mentionAt', () => {
  it('finds the @ and query before the caret', () => {
    expect(mentionAt('Use @ti', 7)).toEqual({ start: 4, query: 'ti' });
    expect(mentionAt('@', 1)).toEqual({ start: 0, query: '' });
    expect(mentionAt('(@do', 4)).toEqual({ start: 1, query: 'do' });
  });

  it('is null after a space or inside an email', () => {
    expect(mentionAt('Use @ti ', 8)).toBeNull();
    expect(mentionAt('ada@ex', 6)).toBeNull();
    expect(mentionAt('Use @Ti', 7)).toBeNull();
  });
});

describe('mentionChoices and mentionState', () => {
  it('lists slugs that start with the query, with tool counts or Disabled', () => {
    expect(mentionChoices(servers, 't')).toEqual([{ value: 'tickets', label: 'tickets', detail: '2 tools' }]);
    expect(mentionChoices(servers, '').map((option) => option.detail)).toEqual(['2 tools', 'Disabled']);
  });

  it('names a known, disabled, or unknown server', () => {
    expect(mentionState('tickets', servers)).toBe('known');
    expect(mentionState('docs', servers)).toBe('disabled');
    expect(mentionState('nothere', servers)).toBe('unknown');
  });
});

describe('reviewerChoices', () => {
  it('labels a member without a verified login and leaves out the creator', () => {
    const options = [
      { id: 'usr_1', name: 'Grace Hopper', email: 'grace@example.com', githubLogin: 'grace' },
      { id: 'usr_2', name: 'Hendro', email: 'hendro@example.com', githubLogin: '' },
      { id: 'usr_3', name: 'Ada', email: 'ada@example.com', githubLogin: 'ada' }
    ];
    expect(reviewerChoices(options, 'ada@example.com')).toEqual([
      { value: 'usr_1', label: 'Grace Hopper', detail: 'grace · grace@example.com' },
      { value: 'usr_2', label: 'Hendro', detail: 'No verified GitHub login · hendro@example.com' }
    ]);
  });
});
