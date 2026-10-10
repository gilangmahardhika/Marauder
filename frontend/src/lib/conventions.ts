import type { ConventionRule, RuleCategory, RuleInput, Skill, SkillInput, Stack, StackInput } from '$lib/types';

export const categories: { value: RuleCategory; label: string }[] = [
  { value: 'naming', label: 'Naming' },
  { value: 'database', label: 'Database' },
  { value: 'style', label: 'Style' },
  { value: 'framework', label: 'Framework' },
  { value: 'security', label: 'Security' },
  { value: 'testing', label: 'Testing' }
];

export function categoryLabel(value: string) {
  return categories.find((item) => item.value === value)?.label ?? value;
}

/** Splits a comma or newline separated list, dropping blanks and repeats. */
export function splitList(text: string) {
  return [...new Set(text.split(/[\n,]/).map((item) => item.trim()).filter(Boolean))];
}

export function joinList(list: string[]) {
  return list.join(', ');
}

export type StackDraft = { name: string; files: string; npmDeps: string; pyDeps: string; disabled: boolean };

export function stackDraft(stack?: Stack): StackDraft {
  return {
    name: stack?.name ?? '',
    files: joinList(stack?.detect.files ?? []),
    npmDeps: joinList(stack?.detect.npmDeps ?? []),
    pyDeps: joinList(stack?.detect.pyDeps ?? []),
    disabled: stack?.disabled ?? false
  };
}

export function stackInput(draft: StackDraft): StackInput {
  return {
    name: draft.name,
    detect: { files: splitList(draft.files), npmDeps: splitList(draft.npmDeps), pyDeps: splitList(draft.pyDeps) },
    disabled: draft.disabled
  };
}

export function ruleInput(rule?: ConventionRule, stackId = ''): RuleInput {
  return {
    scope: rule?.scope ?? (stackId ? 'stack' : 'global'),
    stackId: rule?.stackId ?? stackId,
    category: rule?.category ?? 'style',
    title: rule?.title ?? '',
    body: rule?.body ?? '',
    disabled: rule?.disabled ?? false
  };
}

/** A global rule sends no stack. */
export function ruleBody(rule: RuleInput): RuleInput {
  return { ...rule, stackId: rule.scope === 'global' ? '' : rule.stackId };
}

export function skillInput(skill?: Skill): SkillInput {
  return {
    slug: skill?.slug ?? '',
    title: skill?.title ?? '',
    useWhen: skill?.useWhen ?? '',
    body: skill?.body ?? '',
    disabled: skill?.disabled ?? false
  };
}

export type RuleGroup = { key: string; label: string; stackId: string; rules: ConventionRule[] };

/** Global rules first, then one group per stack that has rules, by stack name. */
export function ruleGroups(rules: ConventionRule[], stacks: Stack[]): RuleGroup[] {
  const groups: RuleGroup[] = [
    { key: 'global', label: 'Global', stackId: '', rules: rules.filter((rule) => rule.scope === 'global') }
  ];
  const named = [...stacks].sort((a, b) => a.name.localeCompare(b.name));
  for (const stack of named) {
    const own = rules.filter((rule) => rule.scope === 'stack' && rule.stackId === stack.id);
    if (own.length > 0) groups.push({ key: stack.id, label: stack.name, stackId: stack.id, rules: own });
  }
  return groups;
}

export function markersSummary(stack: Stack) {
  const parts = [...stack.detect.files, ...stack.detect.npmDeps.map((dep) => `npm ${dep}`), ...stack.detect.pyDeps.map((dep) => `py ${dep}`)];
  return parts.length > 0 ? parts.join(', ') : 'Added by hand only';
}

export function editedBy(item: { updatedBy: string | null; updatedAt: string }) {
  const when = item.updatedAt.replace('T', ' ').replace(/:\d\dZ$/, ' UTC');
  return item.updatedBy ? `Last edited by ${item.updatedBy}, ${when}` : `Seeded by the release, ${when}`;
}
