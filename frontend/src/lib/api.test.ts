import { beforeEach, expect, test, vi } from 'vitest';
import type { RefineEvent } from './types';
import {
  addRequirement,
  createProject,
  getCards,
  getTribeCandidates,
  putDeploy,
  putLinks,
  putMe,
  runCard,
  setAIReviewMode,
  setInstructions,
  setTribeAdmin,
  streamRefine,
  syncRepository
} from './api';
import { session } from './session';

beforeEach(() => {
  session.set('stored-bearer');
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            error: { code: 'unauthenticated', message: 'bearer' }
          }),
          { status: 401 }
        )
    )
  );
});

test('401 clears the bearer', async () => {
  await expect(getCards()).rejects.toThrow(/bearer/);
  expect(session.bearer()).toBe('');
});

test('project settings use the selected project', async () => {
  const calls: { url: string; body: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, body: String(init.body ?? '') });
      return new Response('{}', { status: 200 });
    })
  );
  session.setProject('proj_widget');
  await putDeploy({ environment: 'dev' });
  await syncRepository();
  await putLinks([{ projectId: 'proj_api', sharedPaths: ['docs/api'] }]);
  await createProject({
    tribeId: 'tribe_a',
    name: 'Widget',
    repo: 'widget',
    baseBranch: 'main',
    links: [{ projectId: 'proj_api', sharedPaths: [] }]
  });
  expect(calls.map((call) => call.url)).toEqual([
    'http://127.0.0.1:8787/api/projects/proj_widget/settings/deploy',
    'http://127.0.0.1:8787/api/projects/proj_widget/sync',
    'http://127.0.0.1:8787/api/projects/proj_widget/settings/links',
    'http://127.0.0.1:8787/api/projects'
  ]);
  expect(JSON.parse(calls[2].body)).toEqual({
    links: [{ projectId: 'proj_api', sharedPaths: ['docs/api'] }]
  });
  expect(JSON.parse(calls[3].body)).toEqual({
    tribeId: 'tribe_a',
    name: 'Widget',
    repo: 'widget',
    baseBranch: 'main',
    links: [{ projectId: 'proj_api', sharedPaths: [] }]
  });
});

test('tribe admin calls use the tribe and member ids', async () => {
  const calls: { url: string; method: string; body: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({
        url,
        method: String(init.method ?? 'GET'),
        body: String(init.body ?? '')
      });
      return new Response(init.method === 'PUT' ? null : '{"users":[]}', {
        status: init.method === 'PUT' ? 204 : 200
      });
    })
  );
  await setTribeAdmin('tribe_a', 'user_b', true);
  await getTribeCandidates('tribe_a');
  expect(calls.map((call) => `${call.method} ${call.url}`)).toEqual([
    'PUT http://127.0.0.1:8787/api/tribes/tribe_a/members/user_b',
    'GET http://127.0.0.1:8787/api/tribes/tribe_a/candidates'
  ]);
  expect(JSON.parse(calls[0].body)).toEqual({ tribeAdmin: true });
});

test('a new requirement sends its AI review mode, and the mode has its own route', async () => {
  const calls: { url: string; method: string; body: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, method: String(init.method ?? 'GET'), body: String(init.body ?? '') });
      return new Response('{}', { status: 200 });
    })
  );
  session.setProject('proj_widget');
  await addRequirement({ id: '', statement: 'Export', acceptance: [], selfReview: false, aiReviewMode: 'manual' });
  await setAIReviewMode('REQ-002', 'continue');
  expect(calls.map((call) => `${call.method} ${call.url}`)).toEqual([
    'POST http://127.0.0.1:8787/api/projects/proj_widget/requirements',
    'PUT http://127.0.0.1:8787/api/projects/proj_widget/requirements/REQ-002/ai-review-mode'
  ]);
  expect(JSON.parse(calls[0].body)).toMatchObject({ aiReviewMode: 'manual' });
  expect(JSON.parse(calls[1].body)).toEqual({ aiReviewMode: 'continue' });
});

test('run sends instructions only when given', async () => {
  const calls: { url: string; method: string; body: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, method: String(init.method ?? 'GET'), body: String(init.body ?? '') });
      return new Response('{}', { status: 200 });
    })
  );
  session.setProject('proj_widget');
  await runCard('card_0123456789ab');
  await runCard('card_0123456789ab', '');
  await setInstructions('card_0123456789ab', 'Use lint.');
  expect(calls.map((call) => `${call.method} ${call.url}`)).toEqual([
    'POST http://127.0.0.1:8787/api/projects/proj_widget/cards/card_0123456789ab/run',
    'POST http://127.0.0.1:8787/api/projects/proj_widget/cards/card_0123456789ab/run',
    'PUT http://127.0.0.1:8787/api/projects/proj_widget/cards/card_0123456789ab/instructions'
  ]);
  expect(calls.map((call) => JSON.parse(call.body))).toEqual([{}, { instructions: '' }, { instructions: 'Use lint.' }]);
});

test('putMe saves the caller name', async () => {
  const calls: { url: string; method: string; body: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, method: String(init.method ?? 'GET'), body: String(init.body ?? '') });
      return new Response('{"name":"Ada Lovelace"}', { status: 200 });
    })
  );
  await putMe({ name: 'Ada Lovelace' });
  expect(calls).toEqual([{ url: 'http://127.0.0.1:8787/api/me', method: 'PUT', body: '{"name":"Ada Lovelace"}' }]);
});

function chunked(parts: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      for (const part of parts) controller.enqueue(encoder.encode(part));
      controller.close();
    }
  });
}

const refineBody = {
  field: 'acceptance' as const,
  statement: 'Export the board.',
  acceptance: ['Works well.'],
  instructions: '',
  requirementId: 'REQ-005'
};

test('streamRefine reads events split across chunks', async () => {
  const calls: { url: string; init: RequestInit }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit) => {
      calls.push({ url, init });
      return new Response(
        chunked([
          'event: start\ndata: {"modelId":"model-a"}\n\nevent: del',
          'ta\ndata: {"text":"One row "}\n',
          '\nevent: delta\ndata: {"text":"per card."}\n\n',
          'event: done\ndata: {"text":"One row per card.","acceptance":["One row per card."],"modelId":"model-a"}\n\n'
        ]),
        { status: 200, headers: { 'Content-Type': 'text/event-stream' } }
      );
    })
  );
  session.setProject('proj_widget');
  const events: RefineEvent[] = [];
  await streamRefine(refineBody, (event) => events.push(event), new AbortController().signal);
  expect(calls[0].url).toBe('http://127.0.0.1:8787/api/projects/proj_widget/requirements/refine');
  expect(new Headers(calls[0].init.headers).get('Authorization')).toBe('Bearer stored-bearer');
  expect(JSON.parse(String(calls[0].init.body))).toEqual(refineBody);
  expect(events).toEqual([
    { kind: 'start', modelId: 'model-a' },
    { kind: 'delta', text: 'One row ' },
    { kind: 'delta', text: 'per card.' },
    { kind: 'done', text: 'One row per card.', acceptance: ['One row per card.'], modelId: 'model-a' }
  ]);
});

test('streamRefine throws ApiError for a 409 before the stream', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({ error: { code: 'router_config', message: 'set a model for the implement route in Router settings' } }),
          { status: 409, headers: { 'Content-Type': 'application/json' } }
        )
    )
  );
  const onEvent = vi.fn();
  await expect(streamRefine(refineBody, onEvent, new AbortController().signal)).rejects.toMatchObject({
    name: 'ApiError',
    status: 409,
    code: 'router_config',
    message: expect.stringContaining('implement route')
  });
  expect(onEvent).not.toHaveBeenCalled();
});

test('streamRefine passes an abort through instead of calling it a network error', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (_url: string, init: RequestInit) => {
      throw init.signal?.aborted ? new DOMException('aborted', 'AbortError') : new TypeError('failed');
    })
  );
  const controller = new AbortController();
  controller.abort();
  await expect(streamRefine(refineBody, vi.fn(), controller.signal)).rejects.toMatchObject({ name: 'AbortError' });
});
