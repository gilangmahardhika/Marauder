import { tick } from 'svelte';
import { unreachableMessage } from '$lib/origin';

export type FormFailure = {
  /** The general message for the form's alert. */
  message: string;
  /** Field errors by API field name, for the inputs that show them. */
  fields: Record<string, string>;
  /** Field errors no input on the form shows, listed in the alert so none is lost. */
  extra: string[];
};

export const noFailure: FormFailure = { message: '', fields: {}, extra: [] };

/**
 * The text of a failed request for a toast or a page error. An action with no
 * inputs shows its field errors here, since nothing else would. An abort is empty.
 */
export function failureText(error: unknown): string {
  if (!(error instanceof Error)) return unreachableMessage;
  if (error.name === 'AbortError') return '';
  const fields = (error as Error & { fields?: unknown }).fields;
  if (fields && typeof fields === 'object') {
    const lines = Object.values(fields).filter((value): value is string => typeof value === 'string' && value !== '');
    if (lines.length > 0) return lines.join(' ');
  }
  return error.message || unreachableMessage;
}

/**
 * Splits a failed submit into the alert message and its field errors. `shown`
 * names the fields an input on the form renders; the rest go to `extra`.
 */
export function formFailure(error: unknown, shown: readonly string[] = []): FormFailure {
  const raw = error instanceof Error ? (error as Error & { fields?: unknown }).fields : undefined;
  const fields: Record<string, string> = {};
  if (raw && typeof raw === 'object') {
    for (const [key, value] of Object.entries(raw)) if (typeof value === 'string' && value) fields[key] = value;
  }
  const keys = Object.keys(fields);
  if (keys.length === 0) return { message: failureText(error), fields, extra: [] };
  const extra = keys.filter((key) => !shown.includes(key)).map((key) => fields[key]);
  const message = extra.length < keys.length ? 'Check the fields marked below.' : '';
  return { message, fields, extra };
}

const focusable = 'input, select, textarea, button, [tabindex]';

/** After the errors render, focuses the first invalid control, else the form's alert. */
export async function focusInvalid(form: HTMLElement | null | undefined) {
  await tick();
  if (!form) return;
  const bad = form.querySelector<HTMLElement>('[aria-invalid="true"]');
  const target = bad && !bad.matches(focusable) ? bad.querySelector<HTMLElement>(focusable) : bad;
  (target ?? form.querySelector<HTMLElement>('[role="alert"]'))?.focus();
}
