// M7でWeb Push受信専用として先行実装。M8でアプリシェルのオフラインキャッシュを追記した
// （docs/aibo/m8-implementation-plan.md 設計判断2：next-pwa等のビルドプラグインは使わず
// 手書きのまま拡張する方針）。

const CACHE_NAME = "aisu-shell-v1";
const APP_SHELL = ["/offline", "/manifest.json", "/icons/icon-192.png"];

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(CACHE_NAME).then((cache) => cache.addAll(APP_SHELL)));
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);
  // 別オリジン（Cloud Run API・Supabase）はキャッシュ対象外、素通しする
  // （古いタスク一覧等を誤ってオフライン表示しないための意図的な除外）。
  if (url.origin !== self.location.origin) return;

  if (event.request.mode === "navigate") {
    event.respondWith(
      fetch(event.request)
        .then((res) => {
          const clone = res.clone();
          caches.open(CACHE_NAME).then((cache) => cache.put(event.request, clone));
          return res;
        })
        .catch(() => caches.match(event.request).then((cached) => cached || caches.match("/offline"))),
    );
    return;
  }

  if (url.pathname.startsWith("/_next/static/") || url.pathname.startsWith("/icons/")) {
    event.respondWith(caches.match(event.request).then((cached) => cached || fetch(event.request)));
  }
});

self.addEventListener("push", (event) => {
  let payload = { title: "通知", body: "", url: "/" };
  try {
    if (event.data) {
      payload = { ...payload, ...event.data.json() };
    }
  } catch {
    // JSONでなければデフォルト値のまま表示する。
  }

  event.waitUntil(
    self.registration.showNotification(payload.title, {
      body: payload.body,
      data: { url: payload.url },
    }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = event.notification.data && event.notification.data.url;
  if (url) {
    event.waitUntil(self.clients.openWindow(url));
  }
});
