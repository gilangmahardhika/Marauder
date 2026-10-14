/// <reference types="@sveltejs/kit" />
/// <reference no-default-lib="true"/>
/// <reference lib="esnext" />
/// <reference lib="webworker" />

const sw = self as unknown as ServiceWorkerGlobalScope;

type Message = { title?: string; body?: string; url?: string; tag?: string };

/** A same-origin path, or the board. */
function target(url: unknown): string {
  return typeof url === 'string' && url.startsWith('/') && !url.startsWith('//') ? url : '/';
}

sw.addEventListener('install', () => {
  void sw.skipWaiting();
});

sw.addEventListener('activate', (event) => {
  event.waitUntil(sw.clients.claim());
});

sw.addEventListener('push', (event) => {
  let message: Message = {};
  try {
    message = (event.data?.json() as Message | null) ?? {};
  } catch {
    message = {};
  }
  event.waitUntil(
    sw.registration.showNotification(message.title || 'Orchestrator', {
      body: message.body ?? '',
      tag: message.tag || undefined,
      data: { url: target(message.url) }
    })
  );
});

sw.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const url = new URL(target((event.notification.data as { url?: unknown } | null)?.url), sw.location.origin).href;
  event.waitUntil(
    (async () => {
      const windows = await sw.clients.matchAll({ type: 'window', includeUncontrolled: true });
      for (const client of windows) {
        if (new URL(client.url).origin !== sw.location.origin) continue;
        const focused = await client.focus();
        try {
          await focused.navigate(url);
          return;
        } catch {
          break;
        }
      }
      await sw.clients.openWindow(url);
    })()
  );
});
