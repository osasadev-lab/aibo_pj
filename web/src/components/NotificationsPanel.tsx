"use client";

import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { AtSign, Bell, Check, FolderPlus, FolderX, LogIn, LogOut, UserMinus, UserPlus } from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";
import IconButton from "@/components/ui/IconButton";
import SidePanel from "@/components/ui/SidePanel";

type Notification = {
  id: string;
  type: string;
  payload: Record<string, unknown> | null;
  read_at: string | null;
  created_at: string;
};

// 通知一覧の右サイドバーパネル。左サイドバー「通知」から開く（現在のページを
// 維持したまま重ねて表示するため、/notifications ルートへは遷移しない）。
// /w/[workspaceId]/notifications への直接アクセス用にpage.tsxからも使う。
export default function NotificationsPanel({ onClose }: { onClose: () => void }) {
  const { user } = useAuth();
  const router = useRouter();
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;
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

  // 通知本文クリックで対象箇所へ遷移する。task_idがあればそのタスク詳細
  // （project_idがあればプロジェクトのカンバン、無ければマイタスク側で開く。
  // どちらも既存の?task=クエリパラメータ方式で開閉するTaskDetailPanelと同じ導線）。
  // task_idが無くproject_idのみ（参画・除外通知）の場合はプロジェクト自体へ遷移する。
  // project_deletedは対象プロジェクトが既に存在しないため遷移先が無い（hasTarget側で除外）。
  // 遷移先が現在の画面と同じ場合もあるため、まずこのパネル自体を閉じてから遷移する。
  function handleOpen(n: Notification) {
    const taskId = n.payload?.task_id as string | undefined;
    const projectId = n.payload?.project_id as string | null | undefined;
    if (!taskId && !projectId) return;
    if (!n.read_at) handleMarkRead(n.id);
    onClose();
    const href = taskId
      ? projectId
        ? `/w/${workspaceId}/projects/${projectId}?task=${taskId}`
        : `/w/${workspaceId}/my-tasks?task=${taskId}`
      : `/w/${workspaceId}/projects/${projectId}`;
    router.push(href);
  }

  return (
    <SidePanel title="通知" onClose={onClose}>
      <div className="flex flex-col gap-6">
        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

        {notifications.length === 0 ? (
          <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-border py-16 text-center">
            <Bell className="h-6 w-6 text-muted-foreground/50" />
            <p className="text-sm text-muted-foreground">通知はありません。</p>
          </div>
        ) : (
          <ul className="flex flex-col gap-2">
            {notifications.map((n) => {
              // project_deletedは対象プロジェクトが既に存在しないため遷移先にできない。
              const hasTarget = n.type !== "project_deleted" && (!!n.payload?.task_id || !!n.payload?.project_id);
              return (
                <li key={n.id}>
                  {/* 通知全体をクリック可能にしつつ既読ボタンもネストするため、
                      buttonのネスト（無効なHTML）を避けてdiv+role="button"にする。 */}
                  <div
                    role={hasTarget ? "button" : undefined}
                    tabIndex={hasTarget ? 0 : undefined}
                    onClick={hasTarget ? () => handleOpen(n) : undefined}
                    onKeyDown={
                      hasTarget
                        ? (e) => {
                            if (e.key === "Enter" || e.key === " ") {
                              e.preventDefault();
                              handleOpen(n);
                            }
                          }
                        : undefined
                    }
                    className={clsx(
                      "flex w-full items-start gap-3 rounded-xl border px-4 py-3 text-left text-sm transition-colors",
                      n.read_at ? "border-border text-muted-foreground" : "border-indigo-200 bg-indigo-50/50 dark:border-indigo-500/30 dark:bg-indigo-500/5",
                      hasTarget && "cursor-pointer hover:border-indigo-300 dark:hover:border-indigo-500/50",
                    )}
                  >
                    <span
                      className={clsx(
                        "mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full",
                        n.read_at ? "bg-surface-muted text-muted-foreground" : "bg-indigo-100 text-indigo-600 dark:bg-indigo-500/20 dark:text-indigo-400",
                      )}
                    >
                      <NotificationIcon type={n.type} className="h-3.5 w-3.5" />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className={clsx("block", !n.read_at && "text-foreground")}>{describeNotification(n)}</span>
                      <span className="text-xs text-muted-foreground">{new Date(n.created_at).toLocaleString()}</span>
                    </span>
                    {!n.read_at && (
                      <IconButton
                        size="sm"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleMarkRead(n.id);
                        }}
                        title="既読にする"
                      >
                        <Check className="h-4 w-4" />
                      </IconButton>
                    )}
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </SidePanel>
  );
}

function describeNotification(n: Notification): string {
  if (n.type === "mentioned") {
    const by = (n.payload?.mentioned_by_name as string) ?? "誰か";
    const excerpt = (n.payload?.excerpt as string) ?? "";
    return `${by}さんにメンションされました: ${excerpt}`;
  }
  if (n.type === "assigned") {
    const by = (n.payload?.changed_by_name as string) ?? "誰か";
    const title = (n.payload?.task_title as string) ?? "";
    return `${by}さんがあなたを担当者に追加しました: ${title}`;
  }
  if (n.type === "unassigned") {
    const by = (n.payload?.changed_by_name as string) ?? "誰か";
    const title = (n.payload?.task_title as string) ?? "";
    return `${by}さんがあなたを担当者から外しました: ${title}`;
  }
  if (n.type === "project_joined") {
    const by = (n.payload?.changed_by_name as string) ?? "誰か";
    const name = (n.payload?.project_name as string) ?? "";
    return `${by}さんがあなたをプロジェクトに追加しました: ${name}`;
  }
  if (n.type === "project_removed") {
    const by = (n.payload?.changed_by_name as string) ?? "誰か";
    const name = (n.payload?.project_name as string) ?? "";
    return `${by}さんがあなたをプロジェクトから外しました: ${name}`;
  }
  if (n.type === "project_created") {
    const by = (n.payload?.changed_by_name as string) ?? "誰か";
    const name = (n.payload?.project_name as string) ?? "";
    return `${by}さんが新しいプロジェクトを作成しました: ${name}`;
  }
  if (n.type === "project_deleted") {
    const by = (n.payload?.changed_by_name as string) ?? "誰か";
    const name = (n.payload?.project_name as string) ?? "";
    return `${by}さんがプロジェクトを削除しました: ${name}`;
  }
  return n.type;
}

function NotificationIcon({ type, className }: { type: string; className?: string }) {
  if (type === "assigned") return <UserPlus className={className} />;
  if (type === "unassigned") return <UserMinus className={className} />;
  if (type === "project_joined") return <LogIn className={className} />;
  if (type === "project_removed") return <LogOut className={className} />;
  if (type === "project_created") return <FolderPlus className={className} />;
  if (type === "project_deleted") return <FolderX className={className} />;
  return <AtSign className={className} />;
}
