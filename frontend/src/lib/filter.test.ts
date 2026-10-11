import { describe, expect, it } from 'vitest';
import { matchText, pickParam, withParams } from './filter';

describe('filter helpers', () => {
  it('matches any part, ignoring case, and treats a blank query as a match', () => {
    expect(matchText(['Sign in', 'REQ-001'], 'req-001')).toBe(true);
    expect(matchText(['Sign in', null, undefined], 'SIGN')).toBe(true);
    expect(matchText(['Sign in'], 'export')).toBe(false);
    expect(matchText([], '   ')).toBe(true);
  });

  it('sets and removes URL parameters', () => {
    const url = withParams(new URL('http://127.0.0.1:4173/admin/users?q=ada&role=qa_lead'), { q: '', access: 'admins' });
    expect(url.searchParams.get('q')).toBeNull();
    expect(url.searchParams.get('role')).toBe('qa_lead');
    expect(url.searchParams.get('access')).toBe('admins');
  });

  it('falls back on an unknown value', () => {
    expect(pickParam('admins', ['all', 'admins'] as const, 'all')).toBe('admins');
    expect(pickParam('root', ['all', 'admins'] as const, 'all')).toBe('all');
    expect(pickParam(null, ['all', 'admins'] as const, 'all')).toBe('all');
  });
});
