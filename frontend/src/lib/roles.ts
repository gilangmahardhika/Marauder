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

/** Two letters for an avatar, from a name ("Ada Lovelace" is "AL") or an email. */
export function initials(text: string) {
  if (!text.includes('@')) {
    const words = text.trim().split(/\s+/).filter((word) => word.length > 0);
    const letters = words.length > 1 ? `${words[0][0]}${words[words.length - 1][0]}` : (words[0] ?? '').slice(0, 2);
    return letters.toUpperCase();
  }
  const local = text.split('@')[0] ?? '';
  const parts = local.split(/[._+-]/).filter((part) => part.length > 0);
  const letters = parts.length > 1 ? `${parts[0][0]}${parts[1][0]}` : local.slice(0, 2);
  return letters.toUpperCase();
}
