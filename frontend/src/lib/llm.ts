import { getLLM, getRouter } from '$lib/api';
import { omitEmptySecret } from '$lib/components/SecretField.svelte';
import { matchText, type Option } from '$lib/filter';
import type { Family, ModelChoice, ModelOptions } from '$lib/types';

export type OptionsDraft = {
  thinking: string;
  thinkingBudget: number | string;
  effort: string;
  historyBudgetKiB: number | string;
  contextTokens: number | string;
};

export type FamilyDraft = {
  id: string;
  protocol: string;
  baseUrl: string;
  models: string[];
  options: OptionsDraft[];
  key: string;
  clear: boolean;
  keySet: boolean;
};

export function blankOptions(): OptionsDraft {
  return { thinking: '', thinkingBudget: '', effort: '', historyBudgetKiB: '', contextTokens: '' };
}

export function blankFamily(): FamilyDraft {
  return {
    id: '',
    protocol: 'anthropic',
    baseUrl: '',
    models: [''],
    options: [blankOptions()],
    key: '',
    clear: false,
    keySet: false
  };
}

function toOptionsDraft(options: ModelOptions | undefined): OptionsDraft {
  return {
    thinking: options?.thinking ?? '',
    thinkingBudget: options?.thinkingBudget || '',
    effort: options?.effort ?? '',
    historyBudgetKiB: options?.historyBudgetKiB || '',
    contextTokens: options?.contextTokens || ''
  };
}

export function toDraft(family: Family): FamilyDraft {
  const models = family.models.length ? [...family.models] : [''];
  return {
    id: family.id,
    protocol: family.protocol || 'anthropic',
    baseUrl: family.baseUrl,
    models,
    options: models.map((model) => toOptionsDraft(family.modelOptions?.[model])),
    key: '',
    clear: false,
    keySet: family.keySet
  };
}

export function keepKey(family: Family) {
  return {
    id: family.id,
    protocol: family.protocol,
    baseUrl: family.baseUrl,
    models: family.models,
    modelOptions: family.modelOptions ?? {}
  };
}

export function fromDraft(draft: FamilyDraft) {
  const models: string[] = [];
  const modelOptions: Record<string, ModelOptions> = {};
  draft.models.forEach((raw, index) => {
    const model = raw.trim();
    if (!model) return;
    models.push(model);
    const options = cleanOptions(draft.protocol, draft.options[index] ?? blankOptions());
    if (options) modelOptions[model] = options;
  });
  return {
    id: draft.id.trim(),
    protocol: draft.protocol,
    baseUrl: draft.baseUrl.trim(),
    models,
    modelOptions,
    ...omitEmptySecret('apiKey', draft.key, draft.clear)
  };
}

export type Choice = { value: string; label: string };

/** Thinking values the protocol takes. Adaptive is anthropic only (RFC-079). */
export function thinkingChoices(protocol: string): Choice[] {
  const choices = [
    { value: '', label: 'Provider default' },
    { value: 'enabled', label: 'On' },
    { value: 'disabled', label: 'Off' }
  ];
  return protocol === 'openai' ? choices : [...choices, { value: 'adaptive', label: 'Adaptive' }];
}

const efforts: Record<string, string[]> = {
  anthropic: ['low', 'medium', 'high', 'xhigh', 'max'],
  openai: ['none', 'minimal', 'low', 'medium', 'high', 'xhigh']
};

export function effortChoices(protocol: string): Choice[] {
  const list = efforts[protocol === 'openai' ? 'openai' : 'anthropic'];
  return [{ value: '', label: 'Provider default' }, ...list.map((value) => ({ value, label: value }))];
}

/** A budget applies only to thinking On on the anthropic protocol. */
export function usesBudget(protocol: string, thinking: string) {
  return protocol !== 'openai' && thinking === 'enabled';
}

/** The settings the protocol takes, or null when none is set. */
export function cleanOptions(protocol: string, draft: OptionsDraft): ModelOptions | null {
  const out: ModelOptions = {};
  if (thinkingChoices(protocol).some((choice) => choice.value === draft.thinking && choice.value !== '')) {
    out.thinking = draft.thinking;
  }
  const budget = Number(draft.thinkingBudget);
  if (out.thinking && usesBudget(protocol, out.thinking) && Number.isFinite(budget) && budget > 0) {
    out.thinkingBudget = budget;
  }
  if (effortChoices(protocol).some((choice) => choice.value === draft.effort && choice.value !== '')) {
    out.effort = draft.effort;
  }
  const history = Number(draft.historyBudgetKiB);
  if (draft.historyBudgetKiB !== '' && Number.isFinite(history) && history > 0) {
    out.historyBudgetKiB = history;
  }
  const context = Number(draft.contextTokens);
  if (draft.contextTokens !== '' && Number.isFinite(context) && context > 0) {
    out.contextTokens = context;
  }
  return Object.keys(out).length ? out : null;
}

/** Every listed model with its family and protocol, to choose the model of a refine (RFC-124). */
export function modelChoices(families: Family[]): ModelChoice[] {
  return families.flatMap((family) =>
    family.models.map((id) => ({ id, family: family.id, protocol: protocolOf(family) }))
  );
}

/** The models a refine may choose and the implement route's model, or none when either load fails (RFC-124). */
export async function refineModels(): Promise<{ models: ModelChoice[]; implementModel: string }> {
  try {
    const [llm, router] = await Promise.all([getLLM(), getRouter()]);
    return { models: modelChoices(llm.families), implementModel: router.routes.implement?.model ?? '' };
  } catch {
    return { models: [], implementModel: '' };
  }
}

/** Thinking or effort choices for one request, where the empty value keeps the model's saved setting. */
export function requestChoices(choices: Choice[]): Choice[] {
  return choices.map((choice) => (choice.value === '' ? { ...choice, label: 'Model default' } : choice));
}

export function protocolLabel(protocol: string) {
  return protocol === 'openai' ? 'OpenAI' : 'Anthropic';
}

export type ProtocolFilter = 'all' | 'anthropic' | 'openai';
export type KeyFilter = 'all' | 'saved' | 'unset';

export const protocolFilters: readonly ProtocolFilter[] = ['all', 'anthropic', 'openai'];
export const keyFilters: readonly KeyFilter[] = ['all', 'saved', 'unset'];

function protocolOf(family: Family): ProtocolFilter {
  return family.protocol === 'openai' ? 'openai' : 'anthropic';
}

export function protocolOptions(families: Family[]): Option<ProtocolFilter>[] {
  const openai = families.filter((family) => protocolOf(family) === 'openai').length;
  return [
    { value: 'all', label: 'All', count: families.length },
    { value: 'anthropic', label: 'Anthropic', count: families.length - openai },
    { value: 'openai', label: 'OpenAI', count: openai }
  ];
}

export function keyOptions(families: Family[]): Option<KeyFilter>[] {
  const saved = families.filter((family) => family.keySet).length;
  return [
    { value: 'all', label: 'All', count: families.length },
    { value: 'saved', label: 'Saved', count: saved },
    { value: 'unset', label: 'Not set', count: families.length - saved }
  ];
}

export function filterFamilies(families: Family[], filter: { q: string; protocol: ProtocolFilter; key: KeyFilter }) {
  return families.filter(
    (family) =>
      (filter.protocol === 'all' || protocolOf(family) === filter.protocol) &&
      (filter.key === 'all' || family.keySet === (filter.key === 'saved')) &&
      matchText([family.id, family.baseUrl, ...family.models], filter.q)
  );
}
