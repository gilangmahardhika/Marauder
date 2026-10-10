import { beforeEach, expect, test, vi } from 'vitest';
import { getCards } from './api';
import { session } from './session';

beforeEach(() => {
  session.set('stored-bearer');
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify({ error: { code: 'unauthenticated', message: 'bearer' } }), { status: 401 }))
  );
});

test('401 clears the bearer', async () => {
  await expect(getCards()).rejects.toThrow(/bearer/);
  expect(session.bearer()).toBe('');
});
