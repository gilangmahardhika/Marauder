import { describe, expect, it } from "vitest";
import { filterCards, reviewerSummary } from "./board";
import type { Card } from "./types";

function card(id: string, partial: Partial<Card> = {}): Card {
  return {
    id,
    title: id,
    status: "backlog",
    group: "waiting",
    actions: [],
    unblockTo: null,
    owner: null,
    reviewers: [],
    skills: [],
    requirementIds: [],
    decisionIds: [],
    dependsOn: [],
    risk: "low",
    service: "default",
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
    approvedSha: null,
    deployedSha: null,
    environment: "dev",
    failureReason: null,
    blockedReason: null,
    updatedAt: "2026-10-10T05:00:00Z",
    ...partial,
  };
}

const ada = { id: "user_ada", email: "Ada@example.com", githubLogin: "ada" };
const grace = {
  id: "user_grace",
  email: "grace@example.com",
  githubLogin: "grace",
};

const cards = [
  card("card_1", {
    title: "Sign-in form",
    owner: ada,
    requirementIds: ["REQ-001"],
  }),
  card("card_2", {
    title: "Export CSV",
    owner: grace,
    status: "blocked",
    requirementIds: ["REQ-003"],
  }),
  card("card_4", {
    title: "Billing page",
    owner: null,
    reviewers: [{ id: "user_lin", email: "lin@example.com", githubLogin: "" }],
  }),
  card("card_3", { title: "Audit log", owner: null }),
];

const ids = (list: Card[]) => list.map((item) => item.id);

describe("board filters", () => {
  it("keeps only the signed-in user cards for Mine, ignoring email case", () => {
    expect(
      ids(
        filterCards(
          cards,
          { q: "", owner: "mine", blocked: false },
          "ada@example.com",
        ),
      ),
    ).toEqual(["card_1"]);
    expect(
      ids(filterCards(cards, { q: "", owner: "mine", blocked: false }, "")),
    ).toEqual([]);
  });

  it("keeps only blocked cards", () => {
    expect(
      ids(
        filterCards(
          cards,
          { q: "", owner: "all", blocked: true },
          "ada@example.com",
        ),
      ),
    ).toEqual(["card_2"]);
  });

  it("searches title, id, requirement ids and owner", () => {
    const search = (q: string) =>
      ids(filterCards(cards, { q, owner: "all", blocked: false }, ""));
    expect(search("export")).toEqual(["card_2"]);
    expect(search("REQ-001")).toEqual(["card_1"]);
    expect(search("grace")).toEqual(["card_2"]);
    expect(search("card_3")).toEqual(["card_3"]);
  });

  it("searches reviewer emails and logins", () => {
    const search = (q: string) =>
      ids(filterCards(cards, { q, owner: "all", blocked: false }, ""));
    expect(search("lin@example")).toEqual(["card_4"]);
  });

  it("names two reviewers and counts the rest", () => {
    const lin = { id: "user_lin", email: "lin@example.com", githubLogin: "" };
    expect(reviewerSummary([])).toBe("");
    expect(reviewerSummary([ada, lin])).toBe("ada, lin@example.com");
    expect(reviewerSummary([ada, grace, lin, ada])).toBe("ada, grace, +2");
  });
});
