import { afterEach, expect, test, vi } from 'vitest';
import { session } from '$lib/session';

const api = vi.hoisted(() => ({ getProjects: vi.fn() }));
vi.mock('$lib/api', () => ({ getProjects: api.getProjects }));

import { load } from './+layout';

async function outcome(path: string): Promise<string> {
  try {
    await load({ url: new URL(`http://127.0.0.1:4173${path}`) } as Parameters<typeof load>[0]);
    return 'open';
  } catch (thrown) {
    return (thrown as { location?: string }).location ?? 'error';
  }
}

afterEach(() => {
  sessionStorage.clear();
  vi.clearAllMocks();
});

test('a project route without a stored tribe goes to the tribe selector', async () => {
  session.set('bearer-1');
  expect(await outcome('/')).toBe('/choose-tribe');
  expect(await outcome('/intake')).toBe('/choose-tribe');
  expect(await outcome('/cards/card_1')).toBe('/choose-tribe');
  expect(await outcome('/settings/deploy')).toBe('/choose-tribe');
  expect(await outcome('/admin/users')).toBe('open');
  expect(await outcome('/tribes/new')).toBe('open');
  expect(await outcome('/choose-tribe')).toBe('open');

  session.setTribe('tribe_a');
  expect(await outcome('/')).toBe('open');
});

test('without a bearer every page but sign-in goes to /login', async () => {
  expect(await outcome('/')).toBe('/login');
  expect(await outcome('/choose-tribe')).toBe('/login');
  expect(await outcome('/login')).toBe('open');
});

test('a card link with ?project= switches this tab to that project and its tribe', async () => {
  session.set('bearer-1');
  session.setTribe('tribe_a');
  session.setProject('proj_a');
  api.getProjects.mockResolvedValue({
    projects: [
      { id: 'proj_a', tribeId: 'tribe_a', name: 'A', repo: '' },
      { id: 'proj_b', tribeId: 'tribe_b', name: 'B', repo: '' }
    ]
  });
  expect(await outcome('/cards/card_1?project=proj_b')).toBe('open');
  expect(session.project()).toBe('proj_b');
  expect(session.tribe()).toBe('tribe_b');

  expect(await outcome('/cards/card_1?project=proj_gone')).toBe('open');
  expect(session.project()).toBe('proj_b');

  await outcome('/cards/card_1?project=proj_b');
  expect(api.getProjects).toHaveBeenCalledTimes(2);
});
