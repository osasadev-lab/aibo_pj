"use client";

import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import {
  AlertTriangle,
  AtSign,
  Bell,
  Check,
  Clock,
  FolderPlus,
  FolderX,
  Loader2,
  LogIn,
  LogOut,
  UserMinus,
  UserPlus,
} from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";
import IconButton from "@/components/ui/IconButton";
import SidePanel from "@/components/ui/SidePanel";

type SummaryTask = {
  task_id: string;
  workspace_id: string;
  project_id?: string;
  title: string;
};

type Notification = {
  id: string;
  type: string;
  payload: Record<string, unknown> | null;
  read_at: string | null;
  created_at: string;
};

type ListResponse = {
  items: Notification[];
  has_more: boolean;
};

const SUMMARY_TYPES = new Set(["due_today_summary", "overdue_summary"]);
const PAGE_SIZE = 20;

// 通知一覧の右サイドバーパネル。左サイドバー「通知」から開く（現在のページを
// 維持したまま重ねて表示するため、/notifications ルートへは遷移しない）。
// /w/[workspaceId]/notifications への直接アクセス用にpage.tsxからも使う。
// ページネーション（2026-08-27追加）：既定で最新PAGE_SIZE件を表示し、末尾の
// 「もっと見る」でさらに古いものを追加取得する。
export default function NotificationsPanel({ onClose }: { onClose: () => void }) {
  const { user } = useAuth();
  const router = useRouter();
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!user) return;
    apiFetch<ListResponse>(`/notifications?limit=${PAGE_SIZE}`)
      .then((res) => {
        setNotifications(res.items);
        setHasMore(res.has_more);
      })
      .catch(() => setError("通知の取得に失敗しました"));
  }, [user]);

  useEffect(() => {
    load();
  }, [load]);

  async function handleLoadMore() {
    if (notifications.length === 0 || loadingMore) return;
    setLoadingMore(true);
    try {
      const lastId = notifications[notifications.length - 1].id;
      const res = await apiFetch<ListResponse>(`/notifications?limit=${PAGE_SIZE}&before=${lastId}`);
      setNotifications((prev) => [...prev, ...res.items]);
      setHasMore(res.has_more);
    } catch {
      setError("通知の追加取得に失敗しました");
    } finally {
      setLoadingMore(false);
    }
  }

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
    // コメントへのメンション通知はcomment_idを持つ（説明欄メンションには無い）。
    // 遷移先にこれを含めることで、そのコメントを直接表示・ハイライトできる
    // （2026-08-27追加、CommentThread.initialCommentId参照）。
    const commentId = n.payload?.comment_id as string | undefined;
    if (!taskId && !projectId) return;
    if (!n.read_at) handleMarkRead(n.id);
    onClose();
    const commentQuery = commentId ? `&comment=${commentId}` : "";
    const href = taskId
      ? projectId
        ? `/w/${workspaceId}/projects/${projectId}?task=${taskId}${commentQuery}`
        : `/w/${workspaceId}/my-tasks?task=${taskId}${commentQuery}`
      : `/w/${workspaceId}/projects/${projectId}`;
    router.push(href);
  }

  // due_today_summary/overdue_summaryは1件の通知内に複数タスクへのリンクを持つため、
  // payload.tasksの各要素が自前のworkspace_idを持つ（設計判断3：複数ワークスペースの
  // タスクをまとめうるため、表示中のworkspaceIdに依存せず遷移先を組み立てられる）。
  function handleOpenSummaryTask(n: Notification, t: SummaryTask) {
    if (!n.read_at) handleMarkRead(n.id);
    onClose();
    const href = t.project_id
      ? `/w/${t.workspace_id}/projects/${t.project_id}?task=${t.task_id}`
      : `/w/${t.workspace_id}/my-tasks?task=${t.task_id}`;
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
              // due_today_summary/overdue_summaryは行全体ではなく内部の各タスクリンクが
              // 遷移先を持つため、行自体はクリック対象にしない。
              const isSummary = SUMMARY_TYPES.has(n.type);
              const hasTarget =
                !isSummary && n.type !== "project_deleted" && (!!n.payload?.task_id || !!n.payload?.project_id);
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
                      {isSummary && <SummaryTaskList n={n} onOpenTask={handleOpenSummaryTask} />}
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

        {hasMore && (
          <button
            type="button"
            onClick={handleLoadMore}
            disabled={loadingMore}
            className="flex items-center justify-center gap-1.5 self-center text-xs text-indigo-600 hover:underline disabled:opacity-50 dark:text-indigo-400"
          >
            {loadingMore && <Loader2 className="h-3 w-3 animate-spin" />}
            もっと見る
          </button>
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
  if (n.type === "due_today_summary") {
    const count = (n.payload?.task_count as number) ?? 0;
    return `本日期限のタスクが${count}件あります`;
  }
  if (n.type === "overdue_summary") {
    const count = (n.payload?.task_count as number) ?? 0;
    return `期限を過ぎているタスクが${count}件あります`;
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
  if (type === "due_today_summary") return <Clock className={className} />;
  if (type === "overdue_summary") return <AlertTriangle className={className} />;
  return <AtSign className={className} />;
}

// due_today_summary/overdue_summary通知内のタスク一覧（最大5件、それぞれ個別に
// クリック可能。6件目以降はtask_countとの差分を「他N件」として非リンク表示する）。
function SummaryTaskList({ n, onOpenTask }: { n: Notification; onOpenTask: (n: Notification, t: SummaryTask) => void }) {
  const tasks = (n.payload?.tasks as SummaryTask[] | undefined) ?? [];
  const totalCount = (n.payload?.task_count as number) ?? tasks.length;
  const remaining = totalCount - tasks.length;

  if (tasks.length === 0) return null;

  return (
    <ul className="mt-1 flex flex-col gap-0.5">
      {tasks.map((t) => (
        <li key={t.task_id}>
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              onOpenTask(n, t);
            }}
            className="text-left text-xs text-indigo-600 hover:underline dark:text-indigo-400"
          >
            {t.title}
          </button>
        </li>
      ))}
      {remaining > 0 && <li className="text-xs text-muted-foreground">他{remaining}件</li>}
    </ul>
  );
}
