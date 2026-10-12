import { describe, expect, test } from 'vitest';
import type { UsageRow } from './types';
import {
  dailyTotals,
  filterUsage,
  groupUsage,
  none,
  rangeWindow,
  sortGroups,
  usageOptions,
  usageSummary,
  type UsageFilter
} from './usage';

function row(over: Partial<UsageRow>): UsageRow {
  return {
    day: '2026-10-01',
    projectId: 'proj_default',
    projectName: 'Default',
    familyId: 'anthropic-main',
    protocol: 'anthropic',
    modelId: 'claude-a',
    task: 'implement',
    userId: 'usr_ana',
    userName: 'Ana',
    userEmail: 'ana@example.com',
    calls: 1,
    inputTokens: 100,
    outputTokens: 10,
    cachedInputTokens: 0,
    ...over
  };
}

const usageRows: UsageRow[] = [
  row({ calls: 3, inputTokens: 300, outputTokens: 30, cachedInputTokens: 100 }),
  row({ task: 'review', familyId: 'openai-main', protocol: 'openai', modelId: 'gpt-b', inputTokens: 50, outputTokens: 5 }),
  row({ day: '2026-10-03', projectId: 'proj_gadget', projectName: 'Gadget', userId: 'usr_ben', userName: '', userEmail: 'ben@example.com', task: 'refine_requirement', inputTokens: 40, outputTokens: 4 }),
  row({ day: '2026-10-03', familyId: '', protocol: '', modelId: 'retired', userId: '', userName: '', userEmail: '', inputTokens: 7, outputTokens: 1 })
];

const all: UsageFilter = { llm: '', model: '', project: '', user: '', task: '' };

describe('usage', () => {
  test('groups by each dimension with labels for missing values', () => {
    const byKey = (by: Parameters<typeof groupUsage>[1]) =>
      Object.fromEntries(groupUsage(usageRows, by).map((group) => [group.label, group.total]));
    expect(byKey('llm')).toEqual({ 'anthropic-main': 374, 'openai-main': 55, 'Unknown family': 8 });
    expect(byKey('user')).toEqual({ Ana: 385, 'ben@example.com': 44, 'No owner': 8 });
    expect(byKey('task')).toEqual({ Implement: 338, Review: 55, 'Refine requirement': 44 });
    expect(byKey('project')).toEqual({ Default: 393, Gadget: 44 });
    const model = groupUsage(usageRows, 'model').find((group) => group.key === 'claude-a');
    expect(model).toMatchObject({ calls: 4, inputTokens: 340, outputTokens: 34, cachedInputTokens: 100, total: 374 });
  });

  test('filters by every dimension, with the none value for a missing one', () => {
    expect(filterUsage(usageRows, { ...all, model: 'claude-a' })).toHaveLength(2);
    expect(filterUsage(usageRows, { ...all, user: none })).toEqual([usageRows[3]]);
    expect(filterUsage(usageRows, { ...all, llm: none })).toEqual([usageRows[3]]);
    expect(filterUsage(usageRows, { ...all, project: 'proj_gadget', task: 'review' })).toEqual([]);
  });

  test('options count the rows the other filters keep and keep a chosen value', () => {
    const models = usageOptions(usageRows, { ...all, model: 'gpt-b' }, 'model');
    expect(models.map((option) => option.value)).toEqual(['', 'claude-a', 'gpt-b', 'retired']);
    expect(models[1]).toMatchObject({ label: 'claude-a · 374', count: 374 });
    const users = usageOptions(usageRows, { ...all, project: 'proj_gadget' }, 'user');
    expect(users.map((option) => option.value)).toEqual(['', 'usr_ben']);
    const stale = usageOptions(usageRows, { ...all, task: 'implement_hard' }, 'task');
    expect(stale[1]).toMatchObject({ value: 'implement_hard', count: 0 });
  });

  test('sorts by any column in either direction, ties by name', () => {
    const groups = groupUsage(usageRows, 'model');
    expect(sortGroups(groups, 'total', 'desc').map((group) => group.key)).toEqual(['claude-a', 'gpt-b', 'retired']);
    expect(sortGroups(groups, 'total', 'asc').map((group) => group.key)).toEqual(['retired', 'gpt-b', 'claude-a']);
    expect(sortGroups(groups, 'name', 'desc').map((group) => group.key)).toEqual(['retired', 'gpt-b', 'claude-a']);
    expect(sortGroups(groups, 'cached', 'desc').map((group) => group.key)).toEqual(['claude-a', 'gpt-b', 'retired']);
    expect(sortGroups(groups, 'calls', 'asc').map((group) => group.key)).toEqual(['gpt-b', 'retired', 'claude-a']);
  });

  test('daily totals fill every day in the range', () => {
    const days = dailyTotals(usageRows, '2026-09-30', '2026-10-03');
    expect(days.map((entry) => [entry.day, entry.total])).toEqual([
      ['2026-09-30', 0],
      ['2026-10-01', 385],
      ['2026-10-02', 0],
      ['2026-10-03', 52]
    ]);
  });

  test('summary totals input plus output and a cache share of all input read', () => {
    expect(usageSummary(usageRows)).toEqual({
      calls: 6,
      inputTokens: 397,
      outputTokens: 40,
      cachedInputTokens: 100,
      total: 437,
      cacheShare: 100 / 497
    });
    expect(usageSummary([]).cacheShare).toBe(0);
  });

  test('a range window ends today in UTC', () => {
    expect(rangeWindow(30, new Date('2026-10-12T23:30:00Z'))).toEqual({ from: '2026-09-13', to: '2026-10-12' });
    expect(rangeWindow(7, new Date('2026-03-02T01:00:00Z'))).toEqual({ from: '2026-02-24', to: '2026-03-02' });
  });
});
