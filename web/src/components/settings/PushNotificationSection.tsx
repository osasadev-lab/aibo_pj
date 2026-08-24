"use client";

import { useEffect, useState } from "react";

import { apiFetch } from "@/lib/apiClient";
import Button from "@/components/ui/Button";

// ブラウザのbase64url文字列をPushManager.subscribeが要求するUint8Arrayへ変換する。
function urlBase64ToUint8Array(base64: string): Uint8Array {
  const padding = "=".repeat((4 - (base64.length % 4)) % 4);
  const b64 = (base64 + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = window.atob(b64);
  const output = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) {
    output[i] = raw.charCodeAt(i);
  }
  return output;
}

// Web Push購読UI（個人設定、M7）。Notification許可の取得→Service Worker登録→
// PushManager.subscribe→サーバーへ登録、までを1ボタンで行う。
export default function PushNotificationSection() {
  // このコンポーネントは認証ゲート済みのワークスペースレイアウト配下でのみ
  // マウントされる（=既にクライアント側の再レンダリング後）ため、遅延初期化での
  // window参照はハイドレーション不整合を起こさない。useEffect内でのsetStateは
  // カスケード再レンダリングを招くため避ける。
  const [supported] = useState(
    () => typeof window !== "undefined" && "serviceWorker" in navigator && "PushManager" in window,
  );
  const [subscribed, setSubscribed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!supported) return;
    navigator.serviceWorker
      .getRegistration("/sw.js")
      .then((reg) => reg?.pushManager.getSubscription())
      .then((sub) => setSubscribed(!!sub))
      .catch(() => {});
  }, [supported]);

  async function handleSubscribe() {
    setBusy(true);
    setError(null);
    try {
      const permission = await Notification.requestPermission();
      if (permission !== "granted") {
        setError("ブラウザの通知が許可されませんでした");
        return;
      }
      const registration = await navigator.serviceWorker.register("/sw.js");
      const { public_key: publicKey } = await apiFetch<{ public_key: string }>("/notifications/push-public-key");
      const subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(publicKey) as BufferSource,
      });
      const json = subscription.toJSON();
      await apiFetch("/notifications/subscribe", {
        method: "POST",
        body: JSON.stringify({
          endpoint: json.endpoint,
          keys: { p256dh: json.keys?.p256dh, auth: json.keys?.auth },
        }),
      });
      setSubscribed(true);
    } catch {
      setError("通知の有効化に失敗しました");
    } finally {
      setBusy(false);
    }
  }

  async function handleUnsubscribe() {
    setBusy(true);
    setError(null);
    try {
      const registration = await navigator.serviceWorker.getRegistration("/sw.js");
      const subscription = await registration?.pushManager.getSubscription();
      if (subscription) {
        await apiFetch("/notifications/subscribe", {
          method: "DELETE",
          body: JSON.stringify({ endpoint: subscription.endpoint }),
        });
        await subscription.unsubscribe();
      }
      setSubscribed(false);
    } catch {
      setError("通知の無効化に失敗しました");
    } finally {
      setBusy(false);
    }
  }

  if (!supported) return null;

  return (
    <section className="rounded-xl border border-border bg-surface p-4">
      <h2 className="mb-1 text-sm font-semibold text-foreground">プッシュ通知</h2>
      <p className="mb-3 text-xs text-muted-foreground">
        このブラウザにアプリ内通知をプッシュ通知としても届けます。
      </p>
      {error && <p className="mb-2 text-sm text-red-600 dark:text-red-400">{error}</p>}
      {subscribed ? (
        <Button variant="secondary" size="sm" disabled={busy} onClick={handleUnsubscribe}>
          このブラウザの通知を無効にする
        </Button>
      ) : (
        <Button variant="primary" size="sm" disabled={busy} onClick={handleSubscribe}>
          このブラウザで通知を有効にする
        </Button>
      )}
    </section>
  );
}
