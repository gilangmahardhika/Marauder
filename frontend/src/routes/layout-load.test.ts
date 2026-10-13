import { afterEach, expect, test } from 'vitest';
import { session } from '$lib/session';
import { load } from './+layout';

function outcome(path: string): string {
  try {
    load({ url: new URL(`http://127.0.0.1:4173${path}`) } as Parameters<typeof load>[0]);
    return 'open';
  } catch (thrown) {
    return (thrown as { location?: string }).location ?? 'error';
  }
}

afterEach(() => sessionStorage.clear());

test('a project route without a stored tribe goes to the tribe selector', () => {
  session.set('bearer-1');
  expect(outcome('/')).toBe('/choose-tribe');
  expect(outcome('/intake')).toBe('/choose-tribe');
  expect(outcome('/cards/card_1')).toBe('/choose-tribe');
  expect(outcome('/settings/deploy')).toBe('/choose-tribe');
  expect(outcome('/admin/users')).toBe('open');
  expect(outcome('/tribes/new')).toBe('open');
  expect(outcome('/choose-tribe')).toBe('open');

  session.setTribe('tribe_a');
  expect(outcome('/')).toBe('open');
});

test('without a bearer every page but sign-in goes to /login', () => {
  expect(outcome('/')).toBe('/login');
  expect(outcome('/choose-tribe')).toBe('/login');
  expect(outcome('/login')).toBe('open');
});
