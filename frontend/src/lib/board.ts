import { matchText, type Option } from './filter';
import type { Card } from './types';

export type OwnerFilter = 'all' | 'mine';

export type BoardFilter = {
  q: string;
  owner: OwnerFilter;
  blocked: boolean;
};

export const ownerOptions: Option<OwnerFilter>[] = [
  { value: 'all', label: 'Everyone' },
  { value: 'mine', label: 'Mine' }
];

export function isMine(card: Card, email: string) {
  return email !== '' && card.owner?.email.toLowerCase() === email.toLowerCase();
}

export function filterCards(cards: Card[], filter: BoardFilter, email: string) {
  return cards.filter(
    (card) =>
      (filter.owner === 'all' || isMine(card, email)) &&
      (!filter.blocked || card.status === 'blocked') &&
      matchText([card.id, card.title, card.status, card.owner?.email, card.owner?.githubLogin, ...card.requirementIds], filter.q)
  );
}
