import { afterEach, beforeEach, expect, test, vi } from 'vitest';

const api = vi.hoisted(() => ({
  getPushKey: vi.fn(),
  savePushSubscription: vi.fn(),
  deletePushSubscription: vi.fn()
}));
vi.mock('$lib/api', () => api);

import { disablePush, enablePush, syncPush } from './push';

// 65 bytes: 0x04 then 64 bytes of 0x01, as base64url.
const publicKey = 'BAEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE';

function subscription(endpoint: string, key: Uint8Array | null) {
  return {
    endpoint,
    options: { applicationServerKey: key ? key.buffer : null },
    toJSON: () => ({ endpoint, keys: { p256dh: 'p', auth: 'a' } }),
    unsubscribe: vi.fn().mockResolvedValue(true)
  };
}

let held: ReturnType<typeof subscription> | null;
let permission: NotificationPermission;
const pushManager = {
  getSubscription: vi.fn(async () => held),
  subscribe: vi.fn(async ({ applicationServerKey }: { userVisibleOnly: boolean; applicationServerKey: Uint8Array }) => {
    held = subscription('https://fcm.googleapis.com/fcm/send/new', applicationServerKey);
    return held;
  })
};
const registration = { pushManager };
const serviceWorker = {
  register: vi.fn(async () => registration),
  getRegistration: vi.fn(async () => registration),
  ready: Promise.resolve(registration)
};

beforeEach(() => {
  held = null;
  permission = 'default';
  vi.clearAllMocks();
  api.getPushKey.mockResolvedValue({ publicKey, subscriptions: 0 });
  vi.stubGlobal('PushManager', class {});
  vi.stubGlobal('Notification', {
    get permission() {
      return permission;
    },
    requestPermission: vi.fn(async () => (permission = 'granted'))
  });
  Object.defineProperty(navigator, 'serviceWorker', { value: serviceWorker, configurable: true });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('enable subscribes with the server key and saves the subscription', async () => {
  expect(await enablePush()).toBe('on');
  expect(serviceWorker.register).toHaveBeenCalledWith('/service-worker.js', expect.any(Object));
  const options = pushManager.subscribe.mock.calls[0][0];
  expect(options.userVisibleOnly).toBe(true);
  expect(options.applicationServerKey.length).toBe(65);
  expect(options.applicationServerKey[0]).toBe(4);
  expect(api.savePushSubscription).toHaveBeenCalledWith({
    endpoint: 'https://fcm.googleapis.com/fcm/send/new',
    keys: { p256dh: 'p', auth: 'a' }
  });
});

test('a subscription made with another key is replaced', async () => {
  const old = subscription('https://fcm.googleapis.com/fcm/send/old', new Uint8Array(65));
  held = old;
  expect(await enablePush()).toBe('on');
  expect(old.unsubscribe).toHaveBeenCalledOnce();
  expect(pushManager.subscribe).toHaveBeenCalledOnce();
});

test('a denied permission is blocked and subscribes nothing', async () => {
  vi.mocked(Notification.requestPermission).mockImplementation(async () => (permission = 'denied'));
  expect(await enablePush()).toBe('blocked');
  expect(pushManager.subscribe).not.toHaveBeenCalled();
  expect(await syncPush()).toBe('blocked');
});

test('sync sends a held subscription again', async () => {
  expect(await syncPush()).toBe('off');
  permission = 'granted';
  held = subscription('https://fcm.googleapis.com/fcm/send/held', null);
  expect(await syncPush()).toBe('on');
  expect(api.savePushSubscription).toHaveBeenCalledOnce();
});

test('disable unsubscribes and deletes the endpoint', async () => {
  const sub = subscription('https://fcm.googleapis.com/fcm/send/held', null);
  held = sub;
  expect(await disablePush()).toBe('off');
  expect(sub.unsubscribe).toHaveBeenCalledOnce();
  expect(api.deletePushSubscription).toHaveBeenCalledWith('https://fcm.googleapis.com/fcm/send/held');
});
