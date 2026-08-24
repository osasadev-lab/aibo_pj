// Web Push受信専用の最小限のService Worker（M7）。
// M8のPWA本体（manifest.json・オフラインキャッシュ等）を追加する際、
// このファイルとの統合方法を再検討すること（docs/aibo/m7-implementation-plan.md 設計判断4）。

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
