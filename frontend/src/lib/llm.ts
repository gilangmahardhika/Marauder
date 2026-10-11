import { omitEmptySecret } from "$lib/components/SecretField.svelte";
import { matchText, type Option } from "$lib/filter";
import type { Family } from "$lib/types";

export type FamilyDraft = {
  id: string;
  protocol: string;
  baseUrl: string;
  models: string[];
  key: string;
  clear: boolean;
  keySet: boolean;
};

export function blankFamily(): FamilyDraft {
  return {
    id: "",
    protocol: "anthropic",
    baseUrl: "",
    models: [""],
    key: "",
    clear: false,
    keySet: false,
  };
}

export function toDraft(family: Family): FamilyDraft {
  return {
    id: family.id,
    protocol: family.protocol || "anthropic",
    baseUrl: family.baseUrl,
    models: family.models.length ? [...family.models] : [""],
    key: "",
    clear: false,
    keySet: family.keySet,
  };
}

export function keepKey(family: Family) {
  return {
    id: family.id,
    protocol: family.protocol,
    baseUrl: family.baseUrl,
    models: family.models,
  };
}

export function fromDraft(draft: FamilyDraft) {
  return {
    id: draft.id.trim(),
    protocol: draft.protocol,
    baseUrl: draft.baseUrl.trim(),
    models: draft.models.map((model) => model.trim()).filter(Boolean),
    ...omitEmptySecret("apiKey", draft.key, draft.clear),
  };
}

export function protocolLabel(protocol: string) {
  return protocol === "openai" ? "OpenAI" : "Anthropic";
}

export type ProtocolFilter = "all" | "anthropic" | "openai";
export type KeyFilter = "all" | "saved" | "unset";

export const protocolFilters: readonly ProtocolFilter[] = [
  "all",
  "anthropic",
  "openai",
];
export const keyFilters: readonly KeyFilter[] = ["all", "saved", "unset"];

function protocolOf(family: Family): ProtocolFilter {
  return family.protocol === "openai" ? "openai" : "anthropic";
}

export function protocolOptions(families: Family[]): Option<ProtocolFilter>[] {
  const openai = families.filter(
    (family) => protocolOf(family) === "openai",
  ).length;
  return [
    { value: "all", label: "All", count: families.length },
    { value: "anthropic", label: "Anthropic", count: families.length - openai },
    { value: "openai", label: "OpenAI", count: openai },
  ];
}

export function keyOptions(families: Family[]): Option<KeyFilter>[] {
  const saved = families.filter((family) => family.keySet).length;
  return [
    { value: "all", label: "All", count: families.length },
    { value: "saved", label: "Saved", count: saved },
    { value: "unset", label: "Not set", count: families.length - saved },
  ];
}

export function filterFamilies(
  families: Family[],
  filter: { q: string; protocol: ProtocolFilter; key: KeyFilter },
) {
  return families.filter(
    (family) =>
      (filter.protocol === "all" || protocolOf(family) === filter.protocol) &&
      (filter.key === "all" || family.keySet === (filter.key === "saved")) &&
      matchText([family.id, family.baseUrl, ...family.models], filter.q),
  );
}
