import { describe, expect, it } from "vitest";
import { filterFamilies, keyOptions, protocolOptions } from "./llm";
import type { Family } from "./types";

const families: Family[] = [
  {
    id: "claude",
    protocol: "anthropic",
    baseUrl: "https://api.anthropic.com",
    models: ["claude-sonnet"],
    keySet: true,
  },
  {
    id: "gpt",
    protocol: "openai",
    baseUrl: "https://api.openai.com/v1",
    models: ["gpt-5", "gpt-5-mini"],
    keySet: false,
  },
  {
    id: "glm",
    protocol: "",
    baseUrl: "https://open.bigmodel.cn",
    models: ["glm-4.6"],
    keySet: true,
  },
];

const ids = (list: Family[]) => list.map((family) => family.id);
const all = { q: "", protocol: "all", key: "all" } as const;

describe("LLM family filters", () => {
  it("searches id, base URL, and models", () => {
    expect(ids(filterFamilies(families, { ...all, q: "GLM" }))).toEqual([
      "glm",
    ]);
    expect(ids(filterFamilies(families, { ...all, q: "openai.com" }))).toEqual([
      "gpt",
    ]);
    expect(ids(filterFamilies(families, { ...all, q: "gpt-5-mini" }))).toEqual([
      "gpt",
    ]);
  });

  it("treats any protocol other than openai as Anthropic", () => {
    expect(
      ids(filterFamilies(families, { ...all, protocol: "anthropic" })),
    ).toEqual(["claude", "glm"]);
    expect(
      ids(filterFamilies(families, { ...all, protocol: "openai" })),
    ).toEqual(["gpt"]);
  });

  it("filters by key state", () => {
    expect(ids(filterFamilies(families, { ...all, key: "saved" }))).toEqual([
      "claude",
      "glm",
    ]);
    expect(ids(filterFamilies(families, { ...all, key: "unset" }))).toEqual([
      "gpt",
    ]);
  });

  it("counts each option", () => {
    expect(protocolOptions(families).map((option) => option.count)).toEqual([
      3, 2, 1,
    ]);
    expect(keyOptions(families).map((option) => option.count)).toEqual([
      3, 2, 1,
    ]);
  });
});
