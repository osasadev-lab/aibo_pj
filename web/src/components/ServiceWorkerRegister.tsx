"use client";

import { useEffect } from "react";

// アプリ起動時に無条件でService Workerを登録する（M8）。M7時点ではPush購読を
// 有効化したユーザーのみPushNotificationSection.tsxが登録していたが、オフライン
// シェルキャッシュは全ユーザーに効かせたいためルートレイアウトに移す
// （同一URLへの再registerはブラウザ側で副作用なく冪等）。
export default function ServiceWorkerRegister() {
  useEffect(() => {
    if (!("serviceWorker" in navigator)) return;
    navigator.serviceWorker.register("/sw.js").catch(() => {
      // オフラインキャッシュが効かないだけで、アプリ自体の利用は継続できるため
      // エラー表示はしない。
    });
  }, []);

  return null;
}
