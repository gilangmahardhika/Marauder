export const controlBase =
  'w-full rounded-control border border-line bg-surface px-3 text-sm text-ink placeholder:text-muted/60 hover:border-line-strong disabled:bg-ground disabled:text-muted aria-invalid:border-red-400 aria-invalid:hover:border-red-500 aria-invalid:focus-visible:outline-danger';

export const control = `${controlBase} h-11`;

/** The `aria-describedby` of a control with an optional hint and error, ids `{id}-hint` and `{id}-error`. */
export function describedBy(id: string, hint?: string, error?: string) {
  const ids = [hint ? `${id}-hint` : '', error ? `${id}-error` : ''].filter(Boolean).join(' ');
  return ids || undefined;
}

export const labelClass = 'text-[13px] font-medium text-ink';

export const widths = {
  sm: 'max-w-field-sm',
  md: 'max-w-field-md',
  lg: 'max-w-field-lg',
  full: ''
} as const;

export type Width = keyof typeof widths;

export const th = 'h-10 px-4 text-[11px] font-semibold uppercase tracking-wider text-muted';
export const tr = 'h-12 border-b border-line last:border-b-0 hover:bg-ground/50';
export const td = 'px-4 py-2 align-middle';
