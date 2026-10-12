import { describe, expect, it } from 'vitest';
import { filterFamilies, fromDraft, keepKey, keyOptions, protocolOptions, toDraft } from './llm';
import type { Family } from './types';

const families: Family[] = [
  {
    id: 'claude',
    protocol: 'anthropic',
    baseUrl: 'https://api.anthropic.com',
    models: ['claude-sonnet'],
    keySet: true
  },
  {
    id: 'gpt',
    protocol: 'openai',
    baseUrl: 'https://api.openai.com/v1',
    models: ['gpt-5', 'gpt-5-mini'],
    keySet: false
  },
  {
    id: 'glm',
    protocol: '',
    baseUrl: 'https://open.bigmodel.cn',
    models: ['glm-4.6'],
    keySet: true
  }
];

const ids = (list: Family[]) => list.map((family) => family.id);
const all = { q: '', protocol: 'all', key: 'all' } as const;

describe('LLM family filters', () => {
  it('searches id, base URL, and models', () => {
    expect(ids(filterFamilies(families, { ...all, q: 'GLM' }))).toEqual(['glm']);
    expect(ids(filterFamilies(families, { ...all, q: 'openai.com' }))).toEqual(['gpt']);
    expect(ids(filterFamilies(families, { ...all, q: 'gpt-5-mini' }))).toEqual(['gpt']);
  });

  it('treats any protocol other than openai as Anthropic', () => {
    expect(ids(filterFamilies(families, { ...all, protocol: 'anthropic' }))).toEqual(['claude', 'glm']);
    expect(ids(filterFamilies(families, { ...all, protocol: 'openai' }))).toEqual(['gpt']);
  });

  it('filters by key state', () => {
    expect(ids(filterFamilies(families, { ...all, key: 'saved' }))).toEqual(['claude', 'glm']);
    expect(ids(filterFamilies(families, { ...all, key: 'unset' }))).toEqual(['gpt']);
  });

  it('counts each option', () => {
    expect(protocolOptions(families).map((option) => option.count)).toEqual([3, 2, 1]);
    expect(keyOptions(families).map((option) => option.count)).toEqual([3, 2, 1]);
  });
});

describe('LLM model options', () => {
  const claude: Family = {
    id: 'claude',
    protocol: 'anthropic',
    baseUrl: 'https://api.anthropic.com',
    models: ['claude-a', 'claude-b'],
    modelOptions: { 'claude-a': { thinking: 'enabled', thinkingBudget: 4096, effort: 'high' } },
    keySet: true
  };

  it('keepKey sends the saved options of another family', () => {
    expect(keepKey(claude).modelOptions).toEqual(claude.modelOptions);
    expect(keepKey(families[0]).modelOptions).toEqual({});
  });

  it('toDraft and fromDraft round-trip the options', () => {
    const draft = toDraft(claude);
    expect(draft.options).toEqual([
      { thinking: 'enabled', thinkingBudget: 4096, effort: 'high', historyBudgetKiB: '' },
      { thinking: '', thinkingBudget: '', effort: '', historyBudgetKiB: '' }
    ]);
    expect(fromDraft(draft).modelOptions).toEqual(claude.modelOptions);
  });

  it('fromDraft drops empty settings, removed models, and unused budgets', () => {
    const draft = toDraft(claude);
    draft.models[1] = ' claude-c ';
    draft.options[1] = { thinking: 'adaptive', thinkingBudget: 2048, effort: '', historyBudgetKiB: '' };
    expect(fromDraft(draft).modelOptions).toEqual({
      'claude-a': { thinking: 'enabled', thinkingBudget: 4096, effort: 'high' },
      'claude-c': { thinking: 'adaptive' }
    });

    draft.models = ['claude-c'];
    draft.options = [draft.options[1]];
    expect(Object.keys(fromDraft(draft).modelOptions)).toEqual(['claude-c']);

    draft.options[0] = { thinking: '', thinkingBudget: '', effort: '', historyBudgetKiB: '' };
    expect(fromDraft(draft).modelOptions).toEqual({});
  });

  it('fromDraft keeps only what the protocol takes', () => {
    const draft = toDraft(claude);
    draft.protocol = 'openai';
    draft.options[0] = { thinking: 'adaptive', thinkingBudget: 4096, effort: 'max', historyBudgetKiB: '' };
    draft.options[1] = { thinking: 'enabled', thinkingBudget: 4096, effort: 'minimal', historyBudgetKiB: '' };
    expect(fromDraft(draft).modelOptions).toEqual({ 'claude-b': { thinking: 'enabled', effort: 'minimal' } });
  });

  it('round-trips the history budget on either protocol', () => {
    const draft = toDraft({ ...claude, modelOptions: { 'claude-b': { historyBudgetKiB: 64 } } });
    expect(draft.options[1].historyBudgetKiB).toBe(64);
    draft.protocol = 'openai';
    draft.options[0].historyBudgetKiB = '128';
    expect(fromDraft(draft).modelOptions).toEqual({
      'claude-a': { historyBudgetKiB: 128 },
      'claude-b': { historyBudgetKiB: 64 }
    });
    draft.options[1].historyBudgetKiB = '';
    expect(fromDraft(draft).modelOptions).toEqual({ 'claude-a': { historyBudgetKiB: 128 } });
  });
});
