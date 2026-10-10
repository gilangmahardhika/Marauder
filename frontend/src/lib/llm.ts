import { omitEmptySecret } from '$lib/components/SecretField.svelte';
import type { Family } from '$lib/types';

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
  return { id: '', protocol: 'anthropic', baseUrl: '', models: [''], key: '', clear: false, keySet: false };
}

export function toDraft(family: Family): FamilyDraft {
  return {
    id: family.id,
    protocol: family.protocol || 'anthropic',
    baseUrl: family.baseUrl,
    models: family.models.length ? [...family.models] : [''],
    key: '',
    clear: false,
    keySet: family.keySet
  };
}

export function keepKey(family: Family) {
  return { id: family.id, protocol: family.protocol, baseUrl: family.baseUrl, models: family.models };
}

export function fromDraft(draft: FamilyDraft) {
  return {
    id: draft.id.trim(),
    protocol: draft.protocol,
    baseUrl: draft.baseUrl.trim(),
    models: draft.models.map((model) => model.trim()).filter(Boolean),
    ...omitEmptySecret('apiKey', draft.key, draft.clear)
  };
}

export function protocolLabel(protocol: string) {
  return protocol === 'openai' ? 'OpenAI' : 'Anthropic';
}
