import type { Option } from './filter';
import { tokens } from './format';
import type { UsageRow } from './types';

export type Dimension = 'llm' | 'model' | 'project' | 'user' | 'task' | 'source' | 'card';
export type SortKey = 'name' | 'calls' | 'input' | 'output' | 'cached' | 'total';
export type SortDir = 'asc' | 'desc';
export type UsageFilter = Record<Dimension, string>;

export const dimensionValues = ['llm', 'model', 'project', 'user', 'task', 'source', 'card'] as const;
export const sortValues = ['name', 'calls', 'input', 'output', 'cached', 'total'] as const;
export const rangeValues = ['7', '30', '90'] as const;

export const dimensionOptions: Option<Dimension>[] = [
  { value: 'llm', label: 'LLM' },
  { value: 'model', label: 'Model' },
  { value: 'project', label: 'Project' },
  { value: 'user', label: 'User' },
  { value: 'task', label: 'Task' },
  { value: 'source', label: 'Source' },
  { value: 'card', label: 'Card' }
];

export const rangeOptions: Option<(typeof rangeValues)[number]>[] = [
  { value: '7', label: '7 days' },
  { value: '30', label: '30 days' },
  { value: '90', label: '90 days' }
];

/** The Group by and filter choices for the dimensions a page allows, in the usual order. */
export function dimensionOptionsFor(allowed: readonly Dimension[]) {
  return dimensionOptions.filter((option) => allowed.includes(option.value));
}

const allLabels: Record<Dimension, string> = {
  llm: 'All LLMs',
  model: 'All models',
  project: 'All projects',
  user: 'All users',
  task: 'All tasks',
  source: 'All sources',
  card: 'All cards'
};

const taskLabels: Record<string, string> = {
  implement: 'Implement',
  implement_hard: 'Implement (hard)',
  review: 'Review',
  refine_requirement: 'Refine requirement'
};

/** The filter value for a family, model, project, or user the row does not have. */
export const none = '-';

/** The value a row has in one dimension, as the filter and the URL carry it. */
export function keyOf(row: UsageRow, by: Dimension) {
  switch (by) {
    case 'llm':
      return row.familyId || none;
    case 'model':
      return row.modelId || none;
    case 'project':
      return row.projectId || none;
    case 'user':
      return row.userId || none;
    case 'source':
      return row.source;
    case 'card':
      return row.cardId || none;
    default:
      return row.task;
  }
}

/** How a row's value in one dimension reads on the page. */
export function labelOf(row: UsageRow, by: Dimension) {
  switch (by) {
    case 'llm':
      return row.familyId || 'Unknown family';
    case 'model':
      return row.modelId || 'Unknown model';
    case 'project':
      return row.projectName || row.projectId || 'No project';
    case 'user':
      return row.userId ? row.userName || row.userEmail || row.userId : 'No owner';
    case 'source':
      return row.source === 'intake' ? 'Intake' : 'Card';
    case 'card':
      return row.cardId ? row.cardTitle || row.cardId : 'Intake';
    default:
      return taskLabels[row.task] ?? row.task;
  }
}

export function filterUsage(rows: UsageRow[], filter: UsageFilter, skip?: Dimension) {
  return rows.filter((row) =>
    dimensionValues.every((by) => by === skip || filter[by] === '' || keyOf(row, by) === filter[by])
  );
}

export type UsageGroup = {
  key: string;
  label: string;
  calls: number;
  inputTokens: number;
  outputTokens: number;
  cachedInputTokens: number;
  /** Input plus output; cached input is counted apart. */
  total: number;
};

export function groupUsage(rows: UsageRow[], by: Dimension): UsageGroup[] {
  const groups = new Map<string, UsageGroup>();
  for (const row of rows) {
    const key = keyOf(row, by);
    let group = groups.get(key);
    if (!group) {
      group = { key, label: labelOf(row, by), calls: 0, inputTokens: 0, outputTokens: 0, cachedInputTokens: 0, total: 0 };
      groups.set(key, group);
    }
    group.calls += row.calls;
    group.inputTokens += row.inputTokens;
    group.outputTokens += row.outputTokens;
    group.cachedInputTokens += row.cachedInputTokens;
    group.total += row.inputTokens + row.outputTokens;
  }
  return [...groups.values()];
}

function sortValue(group: UsageGroup, key: Exclude<SortKey, 'name'>) {
  switch (key) {
    case 'calls':
      return group.calls;
    case 'input':
      return group.inputTokens;
    case 'output':
      return group.outputTokens;
    case 'cached':
      return group.cachedInputTokens;
    default:
      return group.total;
  }
}

/** Groups in order of one column; ties fall back to the name, A to Z. */
export function sortGroups(groups: UsageGroup[], key: SortKey, dir: SortDir) {
  const sign = dir === 'asc' ? 1 : -1;
  return [...groups].sort((a, b) => {
    const byName = a.label.localeCompare(b.label);
    if (key === 'name') return sign * byName;
    return sign * (sortValue(a, key) - sortValue(b, key)) || byName;
  });
}

/**
 * Filter choices for one dimension, largest total first, each with its total tokens. They count the rows the other
 * filters keep, so picking a model still lists the other models. A chosen value with no rows stays listed.
 */
export function usageOptions(rows: UsageRow[], filter: UsageFilter, by: Dimension): Option[] {
  const groups = sortGroups(groupUsage(filterUsage(rows, filter, by), by), 'total', 'desc');
  const options: Option[] = groups.map((group) => ({
    value: group.key,
    label: `${group.label} · ${tokens(group.total)}`,
    count: group.total
  }));
  if (filter[by] !== '' && !groups.some((group) => group.key === filter[by])) {
    options.unshift({ value: filter[by], label: `${filter[by]} · 0`, count: 0 });
  }
  return [{ value: '', label: allLabels[by] }, ...options];
}

export type DayTotal = { day: string; calls: number; inputTokens: number; outputTokens: number; total: number };

/** One entry per day from `from` to `to`, inclusive, with zero for a day with no calls. */
export function dailyTotals(rows: UsageRow[], from: string, to: string): DayTotal[] {
  const days = new Map<string, DayTotal>();
  const end = Date.parse(`${to}T00:00:00Z`);
  for (let at = Date.parse(`${from}T00:00:00Z`); at <= end; at += 86_400_000) {
    const day = new Date(at).toISOString().slice(0, 10);
    days.set(day, { day, calls: 0, inputTokens: 0, outputTokens: 0, total: 0 });
  }
  for (const row of rows) {
    const day = days.get(row.day);
    if (!day) continue;
    day.calls += row.calls;
    day.inputTokens += row.inputTokens;
    day.outputTokens += row.outputTokens;
    day.total += row.inputTokens + row.outputTokens;
  }
  return [...days.values()];
}

export type UsageSummary = {
  calls: number;
  inputTokens: number;
  outputTokens: number;
  cachedInputTokens: number;
  total: number;
  /** Cached input over all input read, 0 to 1. Providers count cached input apart from input. */
  cacheShare: number;
};

export function usageSummary(rows: UsageRow[]): UsageSummary {
  const sum = { calls: 0, inputTokens: 0, outputTokens: 0, cachedInputTokens: 0 };
  for (const row of rows) {
    sum.calls += row.calls;
    sum.inputTokens += row.inputTokens;
    sum.outputTokens += row.outputTokens;
    sum.cachedInputTokens += row.cachedInputTokens;
  }
  const read = sum.inputTokens + sum.cachedInputTokens;
  return { ...sum, total: sum.inputTokens + sum.outputTokens, cacheShare: read === 0 ? 0 : sum.cachedInputTokens / read };
}

/** The `from` and `to` days, in UTC, of a range that ends today. */
export function rangeWindow(days: number, now: Date) {
  const to = now.toISOString().slice(0, 10);
  const from = new Date(Date.parse(`${to}T00:00:00Z`) - (days - 1) * 86_400_000).toISOString().slice(0, 10);
  return { from, to };
}
