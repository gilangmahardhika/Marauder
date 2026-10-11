import { matchText } from './filter';
import type { Decision, Requirement } from './types';

export type StatusFilter = 'open' | 'accepted' | 'all';
export type SourceFilter = '' | Requirement['sourceKind'];

export type IntakeFilter = {
  q: string;
  status: StatusFilter;
  source: SourceFilter;
};

export const statusOptions: { value: StatusFilter; label: string }[] = [
  { value: 'open', label: 'Needs acceptance' },
  { value: 'accepted', label: 'Accepted' },
  { value: 'all', label: 'All' }
];

export const sourceOptions: { value: SourceFilter; label: string }[] = [
  { value: '', label: 'All sources' },
  { value: 'prd', label: 'PRD' },
  { value: 'rfc', label: 'RFC' },
  { value: 'manual', label: 'Manual' },
  { value: 'draft', label: 'Draft' }
];

export function sourceLabel(kind: Requirement['sourceKind']) {
  return sourceOptions.find((option) => option.value === kind)?.label ?? kind;
}

export function defaultStatus(reqs: Requirement[]): StatusFilter {
  return reqs.some((req) => !req.accepted) ? 'open' : 'all';
}

export function parseStatus(value: string | null, reqs: Requirement[]): StatusFilter {
  return statusOptions.some((option) => option.value === value) ? (value as StatusFilter) : defaultStatus(reqs);
}

export function parseSource(value: string | null): SourceFilter {
  return sourceOptions.some((option) => option.value === value) ? (value as SourceFilter) : '';
}

export function filterRequirements(reqs: Requirement[], filter: IntakeFilter) {
  return reqs.filter(
    (req) =>
      (filter.status === 'all' || req.accepted === (filter.status === 'accepted')) &&
      (filter.source === '' || req.sourceKind === filter.source) &&
      matchText([req.id, req.statement, req.heading, req.sourcePath, ...req.acceptance], filter.q)
  );
}

export function filterDecisions(decisions: Decision[], q: string) {
  return decisions.filter((decision) => matchText([decision.id, decision.statement, decision.sourcePath], q));
}

export function intakeCounts(reqs: Requirement[]): Record<StatusFilter, number> {
  const accepted = reqs.filter((req) => req.accepted).length;
  return { open: reqs.length - accepted, accepted, all: reqs.length };
}

export function acceptableIds(shown: Requirement[]) {
  return shown.filter((req) => !req.accepted).map((req) => req.id);
}
