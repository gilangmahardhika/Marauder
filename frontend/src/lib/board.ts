import { matchText } from './filter';
import { displayName } from './people';
import { statusLabel } from './status';
import type { Card, CardOwner } from './types';

/** The owner-filter value for cards that have no owner. */
export const noOwner = 'none';

export type BoardFilter = {
  q: string;
  owners: string[];
  blocked: boolean;
};

export type OwnerChoice = { id: string; person: CardOwner | null; count: number };

export function personName(person: CardOwner) {
  return displayName(person);
}

function sameEmail(a: string, b: string) {
  return a !== '' && a.toLowerCase() === b.toLowerCase();
}

/** Each card owner once: the signed-in user first, others by name, then "No owner" when a card has none. */
export function ownerChoices(cards: Card[], email: string): OwnerChoice[] {
  const owners = new Map<string, OwnerChoice>();
  let unowned = 0;
  for (const card of cards) {
    if (!card.owner) {
      unowned++;
      continue;
    }
    const found = owners.get(card.owner.id);
    if (found) found.count++;
    else owners.set(card.owner.id, { id: card.owner.id, person: card.owner, count: 1 });
  }
  const list = [...owners.values()].sort((a, b) => {
    const selfA = sameEmail(email, a.person!.email);
    const selfB = sameEmail(email, b.person!.email);
    if (selfA !== selfB) return selfA ? -1 : 1;
    return displayName(a.person!).localeCompare(displayName(b.person!));
  });
  if (unowned > 0) list.push({ id: noOwner, person: null, count: unowned });
  return list;
}

/** The `owner` URL value as ids. An old `mine` link becomes the signed-in user's id. */
export function parseOwners(param: string | null, choices: OwnerChoice[], email: string) {
  const self = choices.find((choice) => choice.person && sameEmail(email, choice.person.email))?.id;
  const out: string[] = [];
  for (const raw of (param ?? '').split(',')) {
    const id = raw.trim() === 'mine' ? self : raw.trim();
    if (id && !out.includes(id)) out.push(id);
  }
  return out;
}

export function filterCards(cards: Card[], filter: BoardFilter) {
  return cards.filter(
    (card) =>
      (filter.owners.length === 0 || filter.owners.includes(card.owner?.id ?? noOwner)) &&
      (!filter.blocked || card.status === 'blocked') &&
      matchText(
        [
          card.id,
          card.title,
          card.status,
          statusLabel(card.status),
          card.owner?.name,
          card.owner?.email,
          card.owner?.githubLogin,
          ...(card.reviewers ?? []).flatMap((reviewer) => [reviewer.name, reviewer.email, reviewer.githubLogin]),
          ...card.requirementIds
        ],
        filter.q
      )
  );
}
