import { dev } from '$app/environment';
import { deletePushSubscription, getPushKey, savePushSubscription } from '$lib/api';

export type PushState = 'unsupported' | 'blocked' | 'off' | 'on';

export function pushSupported(): boolean {
  return (
    typeof window !== 'undefined' &&
    'serviceWorker' in navigator &&
    'PushManager' in window &&
    'Notification' in window
  );
}

function keyBytes(key: string): Uint8Array<ArrayBuffer> {
  const base64 = key.replace(/-/g, '+').replace(/_/g, '/');
  return Uint8Array.from(atob(base64 + '='.repeat((4 - (base64.length % 4)) % 4)), (c) => c.charCodeAt(0));
}

function sameKey(a: ArrayBuffer | null, b: Uint8Array): boolean {
  if (!a || a.byteLength !== b.length) return false;
  const bytes = new Uint8Array(a);
  return bytes.every((byte, i) => byte === b[i]);
}

async function current(): Promise<PushSubscription | null> {
  const registration = await navigator.serviceWorker.getRegistration('/');
  return (await registration?.pushManager.getSubscription()) ?? null;
}

/** The browser's notification state. A subscription the browser still holds is sent again, since the server drops ones that fail. */
export async function syncPush(): Promise<PushState> {
  if (!pushSupported()) return 'unsupported';
  if (Notification.permission === 'denied') return 'blocked';
  const subscription = await current();
  if (!subscription || Notification.permission !== 'granted') return 'off';
  await savePushSubscription(subscription.toJSON());
  return 'on';
}

export async function enablePush(): Promise<PushState> {
  if (!pushSupported()) return 'unsupported';
  const permission = await Notification.requestPermission();
  if (permission === 'denied') return 'blocked';
  if (permission !== 'granted') return 'off';
  const key = keyBytes((await getPushKey()).publicKey);
  await navigator.serviceWorker.register('/service-worker.js', { type: dev ? 'module' : 'classic' });
  const registration = await navigator.serviceWorker.ready;
  let subscription = await registration.pushManager.getSubscription();
  if (subscription && !sameKey(subscription.options.applicationServerKey, key)) {
    await subscription.unsubscribe();
    subscription = null;
  }
  subscription ??= await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key });
  await savePushSubscription(subscription.toJSON());
  return 'on';
}

export async function disablePush(): Promise<PushState> {
  if (!pushSupported()) return 'unsupported';
  const subscription = await current();
  if (subscription) {
    const { endpoint } = subscription;
    await subscription.unsubscribe();
    await deletePushSubscription(endpoint);
  }
  return 'off';
}
