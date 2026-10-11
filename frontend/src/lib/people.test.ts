import { describe, expect, it } from 'vitest';
import { accessOptions, filterUsers } from './people';
import type { SessionUser } from './types';

const users: SessionUser[] = [
  { id: 'u1', email: 'ada@example.com', role: 'lead_developer', admin: true },
  { id: 'u2', email: 'grace@example.com', role: 'qa_lead', admin: false },
  { id: 'u3', email: 'linus@example.com', role: 'developer', admin: false, disabled: true }
];

const ids = (list: SessionUser[]) => list.map((user) => user.id);

describe('user filters', () => {
  it('filters by role, access and email search', () => {
    expect(ids(filterUsers(users, { q: '', role: 'qa_lead', access: 'all' }))).toEqual(['u2']);
    expect(ids(filterUsers(users, { q: '', role: '', access: 'admins' }))).toEqual(['u1']);
    expect(ids(filterUsers(users, { q: '', role: '', access: 'active' }))).toEqual(['u1', 'u2']);
    expect(ids(filterUsers(users, { q: '', role: '', access: 'disabled' }))).toEqual(['u3']);
    expect(ids(filterUsers(users, { q: 'GRACE', role: '', access: 'all' }))).toEqual(['u2']);
  });

  it('counts each access option', () => {
    expect(accessOptions(users).map((option) => option.count)).toEqual([3, 1, 2, 1]);
  });
});
