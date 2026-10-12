import { describe, expect, it } from 'vitest';
import { acceptableIds, defaultStatus, filterDecisions, filterRequirements, intakeCounts, parseSource, parseStatus } from './intake';
import type { Requirement } from './types';

function req(id: string, partial: Partial<Requirement> = {}): Requirement {
  return {
    id,
    sourceKind: 'prd',
    sourcePath: 'docs/prd/app.md',
    heading: id,
    statement: 'text',
    acceptance: [],
    accepted: false,
    createdBy: null,
    linkedCards: [],
    reviewers: [],
    canEditReviewers: false,
    selfReview: false,
    canSetSelfReview: false,
    ...partial
  };
}

const reqs = [
  req('REQ-001', { statement: 'Sign in with Google', accepted: true }),
  req('REQ-002', { sourceKind: 'rfc', sourcePath: 'docs/rfc/0003-export.md', acceptance: ['One CSV row per card.'] }),
  req('REQ-003', { sourceKind: 'manual', heading: 'Audit log' })
];

describe('intake filters', () => {
  it('searches id, statement, heading, source path and acceptance, ignoring case', () => {
    const ids = (q: string) => filterRequirements(reqs, { q, status: 'all', source: '' }).map((item) => item.id);
    expect(ids('req-003')).toEqual(['REQ-003']);
    expect(ids('GOOGLE')).toEqual(['REQ-001']);
    expect(ids('audit')).toEqual(['REQ-003']);
    expect(ids('0003-export')).toEqual(['REQ-002']);
    expect(ids('csv row')).toEqual(['REQ-002']);
    expect(ids('  ')).toHaveLength(3);
  });

  it('filters by status and source', () => {
    expect(filterRequirements(reqs, { q: '', status: 'open', source: '' }).map((item) => item.id)).toEqual(['REQ-002', 'REQ-003']);
    expect(filterRequirements(reqs, { q: '', status: 'accepted', source: '' }).map((item) => item.id)).toEqual(['REQ-001']);
    expect(filterRequirements(reqs, { q: '', status: 'all', source: 'manual' }).map((item) => item.id)).toEqual(['REQ-003']);
  });

  it('counts by status', () => {
    expect(intakeCounts(reqs)).toEqual({ open: 2, accepted: 1, all: 3 });
  });

  it('offers only unaccepted rows for acceptance', () => {
    expect(acceptableIds(reqs)).toEqual(['REQ-002', 'REQ-003']);
  });

  it('defaults to needs acceptance only while something is unaccepted', () => {
    expect(defaultStatus(reqs)).toBe('open');
    expect(defaultStatus([req('REQ-001', { accepted: true })])).toBe('all');
    expect(parseStatus('bogus', reqs)).toBe('open');
    expect(parseStatus('accepted', reqs)).toBe('accepted');
    expect(parseSource('rfc')).toBe('rfc');
    expect(parseSource('other')).toBe('');
  });

  it('searches decisions', () => {
    const decisions = [
      { id: 'RFC-001', sourcePath: 'docs/rfc/0001.md', statement: 'Go harness' },
      { id: 'RFC-002', sourcePath: 'docs/rfc/0002.md', statement: 'PostgreSQL board' }
    ];
    expect(filterDecisions(decisions, 'postgres').map((item) => item.id)).toEqual(['RFC-002']);
  });
});
