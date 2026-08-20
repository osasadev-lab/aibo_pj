"use client";

import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { FolderPlus, FolderPen, FolderX, ListPlus, Repeat, Trash2 } from "lucide-react";

import { apiFetch } from "@/lib/apiClient";
import Avatar from "@/components/ui/Avatar";
import SidePanel from "@/components/ui/SidePanel";
import { Select } from "@/components/ui/fields";
import type { ActivityLogEntry, MemberSummary } from "@/lib/types";

const STATUS_LABELS: Record<string, string> = {
  not_started: "未対応",
  in_progress: "対応中",
  done: "対応済",
  on_hold: "保留",
};

// ハイライト（誰が何をしたか）の右サイドバーパネル。左サイドバー「ハイライト」から
// 開く。NotificationsPanelと同じ構造（現在のページを維持したまま重ねて表示）。
export default function ActivityPanel({ onClose }: { onClose: () => void }) {
  const router = useRouter();
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;

  const [entries, setEntries] = useState<ActivityLogEntry[]>([]);
  const [members, setMembers] = useState<MemberSummary[]>([]);
  const [actorId, setActorId] = useState("");
  const [error, setError] = useState<string | null>(null);
  // ハイライトは既定で「今日」のみ表示する（データ容量懸念への対応、2026-08-20）。
  // サーバー側は直近30日分を保持しているため、必要ならトグルで遡って見られる。
  const [showAll, setShowAll] = useState(false);

  const load = useCallback(() => {
    if (!workspaceId) return;
    const query = actorId ? `?actor_id=${actorId}` : "";
    apiFetch<ActivityLogEntry[]>(`/workspaces/${workspaceId}/activity${query}`)
      .then(setEntries)
      .catch(() => setError("ハイライトの取得に失敗しました"));
  }, [workspaceId, actorId]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (!workspaceId) return;
    apiFetch<MemberSummary[]>(`/workspaces/${workspaceId}/members`)
      .then(setMembers)
      .catch(() => {});
  }, [workspaceId]);

  function handleOpen(entry: ActivityLogEntry) {
    if (!entry.project_id && !entry.task_id) return;
    onClose();
    const href = entry.task_id
      ? entry.project_id
        ? `/w/${workspaceId}/projects/${entry.project_id}?task=${entry.task_id}`
        : `/w/${workspaceId}/my-tasks?task=${entry.task_id}`
      : `/w/${workspaceId}/projects/${entry.project_id}`;
    router.push(href);
  }

  // 既定では今日（ローカル日付が一致するもの）のみ表示する。サーバー側は
  // 直近30日分保持しているため、トグルでその範囲まで遡って見られる。
  const todayStr = new Date().toDateString();
  const visibleEntries = showAll
    ? entries
    : entries.filter((e) => new Date(e.created_at).toDateString() === todayStr);

  return (
    <SidePanel title="ハイライト" onClose={onClose}>
      <div className="flex flex-col gap-4">
        <div className="flex items-center gap-2">
          <Select value={actorId} onChange={(e) => setActorId(e.target.value)} className="flex-1">
            <option value="">全員</option>
            {members.map((m) => (
              <option key={m.user_id} value={m.user_id}>
                {m.name}
              </option>
            ))}
          </Select>
          <button
            type="button"
            onClick={() => setShowAll((v) => !v)}
            className="shrink-0 whitespace-nowrap text-xs text-indigo-600 hover:underline dark:text-indigo-400"
          >
            {showAll ? "今日のみ表示" : "過去30日を表示"}
          </button>
        </div>

        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

        {visibleEntries.length === 0 ? (
          <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-border py-16 text-center">
            <p className="text-sm text-muted-foreground">
              {showAll ? "まだ何も記録されていません。" : "今日の操作はまだありません。"}
            </p>
          </div>
        ) : (
          <ul className="flex flex-col gap-2">
            {visibleEntries.map((entry) => {
              const hasTarget = !!entry.project_id || !!entry.task_id;
              return (
                <li key={entry.id}>
                  <div
                    role={hasTarget ? "button" : undefined}
                    tabIndex={hasTarget ? 0 : undefined}
                    onClick={hasTarget ? () => handleOpen(entry) : undefined}
                    onKeyDown={
                      hasTarget
                        ? (e) => {
                            if (e.key === "Enter" || e.key === " ") {
                              e.preventDefault();
                              handleOpen(entry);
                            }
                          }
                        : undefined
                    }
                    className={
                      "flex w-full items-start gap-3 rounded-xl border border-border px-4 py-3 text-left text-sm transition-colors" +
                      (hasTarget ? " cursor-pointer hover:border-indigo-300 dark:hover:border-indigo-500/50" : "")
                    }
                  >
                    <Avatar name={entry.actor_name ?? "?"} seed={entry.actor_id} size="sm" />
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center gap-1.5 text-foreground">
                        <ActivityIcon type={entry.action_type} className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                        {describeActivity(entry)}
                      </span>
                      <span className="text-xs text-muted-foreground">{new Date(entry.created_at).toLocaleString()}</span>
                    </span>
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

function describeActivity(entry: ActivityLogEntry): string {
  const actor = entry.actor_name ?? "誰か";
  const payload = entry.payload ?? {};

  switch (entry.action_type) {
    case "project.created":
      return `${actor}さんがプロジェクト『${entry.project_name ?? ""}』を作成しました`;
    case "project.updated":
      return `${actor}さんがプロジェクト『${entry.project_name ?? ""}』を変更しました`;
    case "project.deleted":
      return `${actor}さんがプロジェクト『${(payload.name as string) ?? ""}』を削除しました`;
    case "task.created":
      return `${actor}さんがタスク『${entry.task_title ?? ""}』を作成しました`;
    case "task.deleted":
      return `${actor}さんがタスク『${(payload.title as string) ?? ""}』を削除しました`;
    case "task.status_changed": {
      const from = STATUS_LABELS[payload.from as string] ?? (payload.from as string) ?? "";
      const to = STATUS_LABELS[payload.to as string] ?? (payload.to as string) ?? "";
      return `${actor}さんがタスク『${entry.task_title ?? ""}』のステータスを変更しました（${from}→${to}）`;
    }
    default:
      return `${actor}さんが操作しました（${entry.action_type}）`;
  }
}

function ActivityIcon({ type, className }: { type: string; className?: string }) {
  if (type === "project.created") return <FolderPlus className={className} />;
  if (type === "project.updated") return <FolderPen className={className} />;
  if (type === "project.deleted") return <FolderX className={className} />;
  if (type === "task.created") return <ListPlus className={className} />;
  if (type === "task.deleted") return <Trash2 className={className} />;
  if (type === "task.status_changed") return <Repeat className={className} />;
  return null;
}
