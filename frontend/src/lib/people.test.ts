import { describe, expect, it } from 'vitest';
import { accessOptions, describePerson, displayName, filterUsers } from './people';
import type { SessionUser } from './types';

const users: SessionUser[] = [
  { id: 'u1', name: 'Ada Lovelace', email: 'ada@example.com', role: 'lead_developer', admin: true },
  { id: 'u2', name: '', email: 'grace@example.com', role: 'qa_lead', admin: false },
  { id: 'u3', name: '', email: 'linus@example.com', role: 'developer', admin: false, disabled: true }
];

const ids = (list: SessionUser[]) => list.map((user) => user.id);

describe('user filters', () => {
  it('filters by role, access and email search', () => {
    expect(ids(filterUsers(users, { q: '', role: 'qa_lead', access: 'all' }))).toEqual(['u2']);
    expect(ids(filterUsers(users, { q: '', role: '', access: 'admins' }))).toEqual(['u1']);
    expect(ids(filterUsers(users, { q: '', role: '', access: 'active' }))).toEqual(['u1', 'u2']);
    expect(ids(filterUsers(users, { q: '', role: '', access: 'disabled' }))).toEqual(['u3']);
    expect(ids(filterUsers(users, { q: 'GRACE', role: '', access: 'all' }))).toEqual(['u2']);
    expect(ids(filterUsers(users, { q: 'lovelace', role: '', access: 'all' }))).toEqual(['u1']);
  });

  it('counts each access option', () => {
    expect(accessOptions(users).map((option) => option.count)).toEqual([3, 1, 2, 1]);
  });
});

describe('display names', () => {
  it('falls back from the name to the login to the email', () => {
    expect(displayName({ name: 'Ada Lovelace', email: 'ada@example.com', githubLogin: 'ada' })).toBe('Ada Lovelace');
    expect(displayName({ name: '', email: 'ada@example.com', githubLogin: 'ada' })).toBe('ada');
    expect(displayName({ name: '', email: 'ada@example.com', githubLogin: null })).toBe('ada@example.com');
  });

  it('adds what tells two people apart', () => {
    expect(describePerson({ name: 'Ada Lovelace', email: 'ada@example.com', githubLogin: 'ada' })).toBe(
      'Ada Lovelace (ada)'
    );
    expect(describePerson({ name: 'Ada Lovelace', email: 'ada@example.com', githubLogin: '' })).toBe(
      'Ada Lovelace (ada@example.com)'
    );
    expect(describePerson({ name: '', email: 'ada@example.com', githubLogin: 'ada' })).toBe('ada (ada@example.com)');
    expect(describePerson({ name: '', email: 'ada@example.com', githubLogin: '' })).toBe('ada@example.com');
  });
});
