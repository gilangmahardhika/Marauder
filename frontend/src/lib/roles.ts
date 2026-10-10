export const jobRoles = [
  { value: 'lead_developer', label: 'Lead Developer' },
  { value: 'senior_developer', label: 'Senior Developer' },
  { value: 'developer', label: 'Developer' },
  { value: 'qa_lead', label: 'QA Lead' },
  { value: 'qa', label: 'QA' },
  { value: 'product_owner', label: 'Product Owner' }
] as const;

export function roleLabel(role: string) {
  return jobRoles.find((item) => item.value === role)?.label ?? role;
}

export function initials(email: string) {
  const local = email.split('@')[0] ?? '';
  const parts = local.split(/[._+-]/).filter((part) => part.length > 0);
  const letters = parts.length > 1 ? `${parts[0][0]}${parts[1][0]}` : local.slice(0, 2);
  return letters.toUpperCase();
}
