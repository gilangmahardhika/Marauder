import { describe, expect, it } from 'vitest';
import { filterCards, ownerChoices, parseOwners } from './board';
import type { Card } from './types';

function card(id: string, partial: Partial<Card> = {}): Card {
  return {
    id,
    title: id,
    status: 'backlog',
    group: 'waiting',
    actions: [],
    unblockTo: null,
    owner: null,
    reviewers: [],
    skills: [],
    requirementIds: [],
    decisionIds: [],
    dependsOn: [],
    queued: false,
    waitingOn: [],
    risk: 'low',
    service: 'default',
    manual: false,
    localCheck: null,
    devCheck: null,
    attemptsUsed: 0,
    maxAttempts: 3,
    branch: null,
    prUrl: null,
    headSha: null,
    reviewedSha: null,
    reviewIndependent: null,
    reviewVerdict: null,
    selfReview: false,
    approvedSha: null,
    deployedSha: null,
    environment: 'dev',
    failureReason: null,
    blockedReason: null,
    updatedAt: '2026-10-10T05:00:00Z',
    ...partial
  };
}

const ada = { id: 'user_ada', name: '', email: 'Ada@example.com', githubLogin: 'ada' };
const grace = { id: 'user_grace', name: 'Grace Hopper', email: 'grace@example.com', githubLogin: 'grace' };
const lin = { id: 'user_lin', name: '', email: 'lin@example.com', githubLogin: '' };

const cards = [
  card('card_1', {
    title: 'Sign-in form',
    owner: ada,
    requirementIds: ['REQ-001']
  }),
  card('card_2', {
    title: 'Export CSV',
    owner: grace,
    status: 'blocked',
    requirementIds: ['REQ-003']
  }),
  card('card_4', {
    title: 'Billing page',
    owner: null,
    status: 'human_review',
    reviewers: [lin, { id: 'user_kay', name: 'Kay Ng', email: 'kay@example.com', githubLogin: '' }]
  }),
  card('card_3', { title: 'Audit log', owner: null })
];

const ids = (list: Card[]) => list.map((item) => item.id);

describe('board filters', () => {
  it('keeps cards of any selected owner', () => {
    const pick = (owners: string[]) => ids(filterCards(cards, { q: '', owners, blocked: false }));
    expect(pick([])).toEqual(['card_1', 'card_2', 'card_4', 'card_3']);
    expect(pick(['user_ada'])).toEqual(['card_1']);
    expect(pick(['user_ada', 'user_grace'])).toEqual(['card_1', 'card_2']);
    expect(pick(['none'])).toEqual(['card_4', 'card_3']);
  });

  it('combines owners with blocked only and search', () => {
    expect(ids(filterCards(cards, { q: '', owners: ['user_ada', 'user_grace'], blocked: true }))).toEqual(['card_2']);
    expect(ids(filterCards(cards, { q: 'audit', owners: ['none'], blocked: false }))).toEqual(['card_3']);
    expect(ids(filterCards(cards, { q: 'audit', owners: ['user_ada'], blocked: false }))).toEqual([]);
  });

  it('keeps only blocked cards', () => {
    expect(ids(filterCards(cards, { q: '', owners: [], blocked: true }))).toEqual(['card_2']);
  });

  it('searches title, id, requirement ids and owner', () => {
    const search = (q: string) => ids(filterCards(cards, { q, owners: [], blocked: false }));
    expect(search('export')).toEqual(['card_2']);
    expect(search('REQ-001')).toEqual(['card_1']);
    expect(search('grace')).toEqual(['card_2']);
    expect(search('card_3')).toEqual(['card_3']);
    expect(search('hopper')).toEqual(['card_2']);
  });

  it('searches reviewer emails and logins', () => {
    const search = (q: string) => ids(filterCards(cards, { q, owners: [], blocked: false }));
    expect(search('lin@example')).toEqual(['card_4']);
    expect(search('kay ng')).toEqual(['card_4']);
  });

  it('searches the status label', () => {
    const search = (q: string) => ids(filterCards(cards, { q, owners: [], blocked: false }));
    expect(search('waiting for review')).toEqual(['card_4']);
    expect(search('human_review')).toEqual(['card_4']);
  });
});

describe('owner choices', () => {
  it('lists each owner once, the signed-in user first, then by name, then no owner', () => {
    const choices = ownerChoices([...cards, card('card_5', { owner: grace })], 'GRACE@example.com');
    expect(choices.map((choice) => [choice.id, choice.count])).toEqual([
      ['user_grace', 2],
      ['user_ada', 1],
      ['none', 2]
    ]);
    expect(ownerChoices(cards, '').map((choice) => choice.id)).toEqual(['user_ada', 'user_grace', 'none']);
  });

  it('leaves out no owner when every card has one', () => {
    expect(ownerChoices(cards.slice(0, 2), '').map((choice) => choice.id)).toEqual(['user_ada', 'user_grace']);
  });

  it('reads the owner param and maps an old mine link', () => {
    const choices = ownerChoices(cards, 'ada@example.com');
    expect(parseOwners(null, choices, 'ada@example.com')).toEqual([]);
    expect(parseOwners('user_grace, none,user_grace,', choices, 'ada@example.com')).toEqual(['user_grace', 'none']);
    expect(parseOwners('mine', choices, 'Ada@Example.com')).toEqual(['user_ada']);
    expect(parseOwners('mine', choices, 'nobody@example.com')).toEqual([]);
  });
});
