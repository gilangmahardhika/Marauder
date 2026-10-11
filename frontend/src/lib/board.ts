import { matchText, type Option } from "./filter";
import type { Card, CardOwner } from "./types";

export type OwnerFilter = "all" | "mine";

export type BoardFilter = {
  q: string;
  owner: OwnerFilter;
  blocked: boolean;
};

export const ownerOptions: Option<OwnerFilter>[] = [
  { value: "all", label: "Everyone" },
  { value: "mine", label: "Mine" },
];

export function isMine(card: Card, email: string) {
  return (
    email !== "" && card.owner?.email.toLowerCase() === email.toLowerCase()
  );
}

export function personName(person: CardOwner) {
  return person.githubLogin || person.email;
}

/** The first two reviewer names and how many more, e.g. "ada, grace, +2". */
export function reviewerSummary(reviewers: CardOwner[]) {
  const names = reviewers.slice(0, 2).map(personName);
  if (reviewers.length > 2) names.push(`+${reviewers.length - 2}`);
  return names.join(", ");
}

export function filterCards(cards: Card[], filter: BoardFilter, email: string) {
  return cards.filter(
    (card) =>
      (filter.owner === "all" || isMine(card, email)) &&
      (!filter.blocked || card.status === "blocked") &&
      matchText(
        [
          card.id,
          card.title,
          card.status,
          card.owner?.email,
          card.owner?.githubLogin,
          ...(card.reviewers ?? []).flatMap((reviewer) => [
            reviewer.email,
            reviewer.githubLogin,
          ]),
          ...card.requirementIds,
        ],
        filter.q,
      ),
  );
}
