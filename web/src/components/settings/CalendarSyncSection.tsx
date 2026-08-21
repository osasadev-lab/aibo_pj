"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import clsx from "clsx";

import { apiFetch, getToken, API_BASE_URL } from "@/lib/apiClient";
import Button from "@/components/ui/Button";
import DatePicker from "@/components/ui/DatePicker";

type Mode = "auto" | "manual";

type CalendarSettings = {
  enabled: boolean;
  mode: Mode | null;
  connected: boolean;
};

const MODE_OPTIONS: { value: Mode; label: string; description: string }[] = [
  { value: "auto", label: "自動", description: "担当タスクの作成・更新・削除のたびに即時反映する" },
  { value: "manual", label: "手動", description: "指定した日付が期限のタスクをまとめて反映する（都度実行が必要）" },
];

// Googleカレンダー連携（個人設定、M6）。連携前は「連携する」ボタンのみ表示し、
// クリックでGoogle同意画面へのフルページ遷移を行う（Bearer JWT方式のため
// Authorizationヘッダーが使えず、tokenをクエリパラメータで渡す。
// docs/aibo/m6-implementation-plan.md参照）。
export default function CalendarSyncSection() {
  const params = useParams<{ workspaceId: string }>();
  const router = useRouter();
  const searchParams = useSearchParams();

  // Google同意フローから /w/:workspaceId/settings?calendar=connected|error で
  // 戻ってきた場合の結果表示。初回レンダリング時点のクエリから初期値を計算する
  // （useEffect内でのsetStateはカスケード再レンダリングを招くため避ける）。
  const calendarResult = searchParams.get("calendar");
  const [settings, setSettings] = useState<CalendarSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(() =>
    calendarResult === "error" ? "Googleカレンダーとの連携に失敗しました。もう一度お試しください。" : null,
  );
  const [syncDate, setSyncDate] = useState("");
  const [syncing, setSyncing] = useState(false);
  const [syncResult, setSyncResult] = useState<string | null>(null);
  const [notice] = useState<string | null>(() =>
    calendarResult === "connected" ? "Googleカレンダーとの連携が完了しました。" : null,
  );

  useEffect(() => {
    apiFetch<CalendarSettings>("/me/calendar-settings")
      .then(setSettings)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  // 表示したらURLからクエリを消し、リロード時に再表示されないようにする
  // （setStateを伴わない副作用のみ）。
  useEffect(() => {
    if (!calendarResult) return;
    router.replace(`/w/${params.workspaceId}/settings`);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function handleConnect() {
    const token = getToken();
    if (!token) return;
    // 別オリジンのGo API（Cloud Run）への遷移のため、Next.jsのuseRouterではなく
    // フルページ遷移を使う（意図的な外部ナビゲーション）。連携完了後にこの
    // ワークスペースの設定画面へ戻れるようworkspace_idも渡す。
    // eslint-disable-next-line @next/next/no-location-assign-relative-destination
    window.location.href = `${API_BASE_URL}/auth/google/calendar/connect?token=${encodeURIComponent(token)}&workspace_id=${params.workspaceId}`;
  }

  async function handleToggle(nextEnabled: boolean) {
    if (!settings) return;
    const prev = settings;
    const nextMode = settings.mode ?? "auto";
    setSettings({ ...settings, enabled: nextEnabled, mode: nextEnabled ? nextMode : settings.mode });
    setError(null);
    try {
      const updated = await apiFetch<CalendarSettings>("/me/calendar-settings", {
        method: "PATCH",
        body: JSON.stringify(nextEnabled ? { enabled: true, mode: nextMode } : { enabled: false }),
      });
      setSettings(updated);
    } catch {
      setSettings(prev);
      setError("連携設定の更新に失敗しました");
    }
  }

  async function handleModeChange(mode: Mode) {
    if (!settings) return;
    const prev = settings;
    setSettings({ ...settings, mode });
    setError(null);
    try {
      const updated = await apiFetch<CalendarSettings>("/me/calendar-settings", {
        method: "PATCH",
        body: JSON.stringify({ enabled: true, mode }),
      });
      setSettings(updated);
    } catch {
      setSettings(prev);
      setError("連携設定の更新に失敗しました");
    }
  }

  async function handleManualSync() {
    if (!syncDate) return;
    setSyncing(true);
    setSyncResult(null);
    setError(null);
    try {
      const res = await apiFetch<{ synced: number; failed: number }>("/me/calendar-sync", {
        method: "POST",
        body: JSON.stringify({ date: syncDate }),
      });
      setSyncResult(
        res.failed > 0 ? `${res.synced}件連携しました（失敗: ${res.failed}件）` : `${res.synced}件連携しました`,
      );
    } catch {
      setError("連携の実行に失敗しました");
    } finally {
      setSyncing(false);
    }
  }

  return (
    <section className="rounded-xl border border-border bg-surface p-4">
      <h2 className="mb-1 text-sm font-semibold text-foreground">Googleカレンダー連携</h2>
      <p className="mb-3 text-xs text-muted-foreground">
        期限が設定されている自分の担当タスクをGoogleカレンダーに反映します。
      </p>
      {error && <p className="mb-2 text-sm text-red-600 dark:text-red-400">{error}</p>}
      {notice && <p className="mb-2 text-sm text-emerald-600 dark:text-emerald-400">{notice}</p>}

      {loading ? (
        <p className="text-sm text-muted-foreground">読み込み中...</p>
      ) : !settings?.connected ? (
        <Button variant="primary" size="sm" onClick={handleConnect}>
          Googleカレンダーと連携する
        </Button>
      ) : (
        <div className="flex flex-col gap-3">
          <div className="flex items-center justify-between gap-2">
            <label className="flex cursor-pointer items-center gap-2 text-sm text-foreground">
              <input
                type="checkbox"
                checked={settings.enabled}
                onChange={(e) => handleToggle(e.target.checked)}
              />
              連携する
            </label>
            <Button variant="ghost" size="sm" onClick={handleConnect}>
              Googleアカウントを繋ぎ直す
            </Button>
          </div>
          <p className="text-xs text-muted-foreground/70">
            テストモードのGoogleアプリはアクセス権限が7日で失効します。同期されなくなった場合はこちらから繋ぎ直してください。
          </p>

          {settings.enabled && (
            <>
              <div className="flex flex-col gap-1.5">
                {MODE_OPTIONS.map((opt) => (
                  <label
                    key={opt.value}
                    className={clsx(
                      "flex cursor-pointer items-start gap-2 rounded-lg px-2 py-1.5 text-sm transition-colors",
                      settings.mode === opt.value ? "bg-indigo-50 dark:bg-indigo-500/10" : "hover:bg-surface-muted",
                    )}
                  >
                    <input
                      type="radio"
                      name="calendar-sync-mode"
                      checked={settings.mode === opt.value}
                      onChange={() => handleModeChange(opt.value)}
                      className="mt-1"
                    />
                    <span>
                      <span className="block text-foreground">{opt.label}</span>
                      <span className="block text-xs text-muted-foreground">{opt.description}</span>
                    </span>
                  </label>
                ))}
              </div>

              {settings.mode === "manual" && (
                <div className="flex flex-col gap-2 border-t border-border pt-3">
                  <p className="text-xs text-muted-foreground">
                    指定した日付が期限になっている担当タスクをまとめて連携します（最大50件）。
                  </p>
                  <div className="flex items-center gap-2">
                    <DatePicker value={syncDate} onChange={setSyncDate} className="max-w-[10rem]" />
                    <Button variant="secondary" size="sm" disabled={!syncDate || syncing} onClick={handleManualSync}>
                      {syncing ? "連携中..." : "この日のタスクを連携"}
                    </Button>
                  </div>
                  {syncResult && <p className="text-xs text-muted-foreground">{syncResult}</p>}
                </div>
              )}
            </>
          )}
        </div>
      )}
    </section>
  );
}
