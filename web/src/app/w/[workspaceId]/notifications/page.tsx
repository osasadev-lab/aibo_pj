"use client";

import { useCallback, useEffect, useState } from "react";
import { AtSign, Bell, Check } from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";
import IconButton from "@/components/ui/IconButton";

type Notification = {
  id: string;
  type: string;
  payload: Record<string, unknown> | null;
  read_at: string | null;
  created_at: string;
};

// 通知一覧。notifications.user_idにworkspace_idは無くデータ自体はワークスペース
// 非依存だが、他画面（プロジェクト/マイタスク/メンバー）とヘッダーを揃えるため
// w/[workspaceId]/配下に配置する（WorkspaceLayoutを継承）。
export default function NotificationsPage() {
  const { user } = useAuth();
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!user) return;
    apiFetch<Notification[]>("/notifications")
      .then(setNotifications)
      .catch(() => setError("通知の取得に失敗しました"));
  }, [user]);

  useEffect(() => {
    load();
  }, [load]);

  async function handleMarkRead(id: string) {
    try {
      await apiFetch(`/notifications/${id}/read`, { method: "PATCH" });
      load();
    } catch {
      setError("既読化に失敗しました");
    }
  }

  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-6 px-6 py-8">
      <h1 className="text-2xl font-semibold tracking-tight text-foreground">通知</h1>
      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

      {notifications.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-border py-16 text-center">
          <Bell className="h-6 w-6 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground">通知はありません。</p>
        </div>
      ) : (
        <ul className="flex flex-col gap-2">
          {notifications.map((n) => (
            <li
              key={n.id}
              className={clsx(
                "flex items-start gap-3 rounded-xl border px-4 py-3 text-sm transition-colors",
                n.read_at ? "border-border text-muted-foreground" : "border-indigo-200 bg-indigo-50/50 dark:border-indigo-500/30 dark:bg-indigo-500/5",
              )}
            >
              <span
                className={clsx(
                  "mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full",
                  n.read_at ? "bg-surface-muted text-muted-foreground" : "bg-indigo-100 text-indigo-600 dark:bg-indigo-500/20 dark:text-indigo-400",
                )}
              >
                <AtSign className="h-3.5 w-3.5" />
              </span>
              <div className="min-w-0 flex-1">
                <p className={n.read_at ? "" : "text-foreground"}>{describeNotification(n)}</p>
                <span className="text-xs text-muted-foreground">{new Date(n.created_at).toLocaleString()}</span>
              </div>
              {!n.read_at && (
                <IconButton size="sm" onClick={() => handleMarkRead(n.id)} title="既読にする">
                  <Check className="h-4 w-4" />
                </IconButton>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function describeNotification(n: Notification): string {
  if (n.type === "mentioned") {
    const by = (n.payload?.mentioned_by_name as string) ?? "誰か";
    const excerpt = (n.payload?.excerpt as string) ?? "";
    return `${by}さんにメンションされました: ${excerpt}`;
  }
  return n.type;
}
