import { expect, test } from 'vitest';
import { omitEmptySecret } from './SecretField.svelte';

test('empty secret is omitted', () => {
  const body = omitEmptySecret('token', '', false);
  expect(body).not.toHaveProperty('token');
  expect(JSON.stringify(body)).not.toContain('token');
});
