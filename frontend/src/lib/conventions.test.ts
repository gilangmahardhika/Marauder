import { describe, expect, it } from 'vitest';
import { filterRules, filterStacks, ruleBody, ruleGroups, splitList, stackDraft, stackInput } from './conventions';
import type { ConventionRule, Stack } from './types';

function stack(id: string, name: string): Stack {
  return {
    id,
    name,
    detect: { files: [], npmDeps: [], pyDeps: [] },
    version: 1,
    contentHash: 'h',
    disabled: false,
    updatedBy: null,
    updatedAt: '2026-10-10T00:00:00Z'
  };
}

function rule(id: string, stackId = ''): ConventionRule {
  return {
    id,
    scope: stackId ? 'stack' : 'global',
    stackId,
    category: 'style',
    title: id,
    body: 'b',
    version: 1,
    contentHash: 'h',
    disabled: false,
    updatedBy: null,
    updatedAt: '2026-10-10T00:00:00Z'
  };
}

describe('conventions helpers', () => {
  it('splits marker lists on commas and lines', () => {
    expect(splitList('go.mod, tsconfig.json\n go.mod,,')).toEqual(['go.mod', 'tsconfig.json']);
  });

  it('round trips a stack draft', () => {
    const draft = stackDraft({ ...stack('stk_go', 'Go'), detect: { files: ['go.mod'], npmDeps: [], pyDeps: [] } });
    expect(draft.files).toBe('go.mod');
    expect(stackInput({ ...draft, npmDeps: 'react, @sveltejs/kit' }).detect.npmDeps).toEqual(['react', '@sveltejs/kit']);
  });

  it('groups global rules first and stacks by name', () => {
    const groups = ruleGroups(
      [rule('r1', 'stk_ts'), rule('r2'), rule('r3', 'stk_go')],
      [stack('stk_ts', 'TypeScript'), stack('stk_go', 'Go'), stack('stk_py', 'Python')]
    );
    expect(groups.map((group) => group.label)).toEqual(['Global', 'Go', 'TypeScript']);
  });

  it('filters rules by search, category, and scope', () => {
    const rules = [rule('Naming'), { ...rule('Go errors', 'stk_go'), category: 'framework' as const, body: 'wrap with %w' }];
    expect(filterRules(rules, { q: '%W', category: '', applies: '' }).map((item) => item.id)).toEqual(['Go errors']);
    expect(filterRules(rules, { q: '', category: 'style', applies: '' }).map((item) => item.id)).toEqual(['Naming']);
    expect(filterRules(rules, { q: '', category: '', applies: 'global' }).map((item) => item.id)).toEqual(['Naming']);
    expect(filterRules(rules, { q: '', category: '', applies: 'stk_go' }).map((item) => item.id)).toEqual(['Go errors']);
  });

  it('finds a stack by a marker', () => {
    const go = { ...stack('stk_go', 'Go'), detect: { files: ['go.mod'], npmDeps: [], pyDeps: [] } };
    expect(filterStacks([go, stack('stk_ts', 'TypeScript')], 'go.MOD').map((item) => item.id)).toEqual(['stk_go']);
  });

  it('sends no stack for a global rule', () => {
    expect(ruleBody({ scope: 'global', stackId: 'stk_go', category: 'naming', title: 't', body: 'b', disabled: false }).stackId).toBe('');
  });
});
