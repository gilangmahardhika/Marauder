import { render, screen } from '@testing-library/svelte';
import { expect, test } from 'vitest';
import { failureText, focusInvalid, formFailure } from './forms';
import { unreachableMessage } from './origin';
import FormAlert from './ui/FormAlert.svelte';

class FakeApiError extends Error {
  fields: Record<string, string>;
  constructor(message: string, fields: Record<string, string> = {}) {
    super(message);
    this.fields = fields;
  }
}

test('formFailure keeps the message of an error with no fields', () => {
  expect(formFailure(new FakeApiError('Only a Lead Developer can save.'))).toEqual({
    message: 'Only a Lead Developer can save.',
    fields: {},
    extra: []
  });
});

test('formFailure points at shown fields and lists the rest as extra', () => {
  const failure = formFailure(new FakeApiError('validation', { name: 'Name is required', org: 'Unknown org' }), ['name']);
  expect(failure.fields).toEqual({ name: 'Name is required', org: 'Unknown org' });
  expect(failure.extra).toEqual(['Unknown org']);
  expect(failure.message).toBe('Check the fields marked below.');
});

test('formFailure with no shown field has no general message, only the extra lines', () => {
  const failure = formFailure(new FakeApiError('validation', { org: 'Unknown org' }));
  expect(failure.message).toBe('');
  expect(failure.extra).toEqual(['Unknown org']);
});

test('failureText shows field errors for an action with no inputs', () => {
  expect(failureText(new FakeApiError('validation', { token: 'an admin must save the company GitHub token' }))).toBe(
    'an admin must save the company GitHub token'
  );
  expect(failureText(new FakeApiError('forbidden'))).toBe('forbidden');
});

test('a value that is not an error reads as the API being unreachable', () => {
  expect(formFailure('boom').message).toBe(unreachableMessage);
  expect(unreachableMessage).toContain("Can't reach the API at http://127.0.0.1:8787");
  const abort = new Error('aborted');
  abort.name = 'AbortError';
  expect(failureText(abort)).toBe('');
});

test('focusInvalid focuses the first invalid control, else the alert', async () => {
  const form = document.createElement('form');
  form.innerHTML =
    '<div role="alert" tabindex="-1">Bad</div><input id="a"><input id="b" aria-invalid="true"><input id="c" aria-invalid="true">';
  document.body.append(form);
  await focusInvalid(form);
  expect(document.activeElement?.id).toBe('b');
  form.querySelectorAll('[aria-invalid]').forEach((node) => node.removeAttribute('aria-invalid'));
  await focusInvalid(form);
  expect(document.activeElement?.getAttribute('role')).toBe('alert');
  form.remove();
});

test('focusInvalid focuses the first control inside an invalid group', async () => {
  const form = document.createElement('form');
  form.innerHTML = '<div aria-invalid="true"><button type="button" id="mode">Continue</button></div>';
  document.body.append(form);
  await focusInvalid(form);
  expect(document.activeElement?.id).toBe('mode');
  form.remove();
});

test('FormAlert shows the message and the extra lines, and nothing when empty', () => {
  const { unmount } = render(FormAlert, { message: 'Check the fields marked below.', extra: ['Unknown org'] });
  const alert = screen.getByRole('alert');
  expect(alert.getAttribute('tabindex')).toBe('-1');
  expect(alert.textContent).toContain('Check the fields marked below.');
  expect(screen.getByRole('listitem').textContent).toBe('Unknown org');
  unmount();
  render(FormAlert, { message: '', extra: [] });
  expect(screen.queryByRole('alert')).toBeNull();
});
