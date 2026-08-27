"use client";

import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";

import { Calendar, CalendarClock } from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import KanbanBoard from "@/components/kanban/KanbanBoard";
import TaskDetailPanel from "@/components/TaskDetailPanel";
import Badge from "@/components/ui/Badge";
import DatePicker from "@/components/ui/DatePicker";
import type { Tag } from "@/lib/types";

type Task = {
  id: string;
  title: string;
  status: "not_started" | "in_progress" | "done" | "on_hold";
  project_id: string | null;
  due_today: boolean;
  // タグ・開始日時・期限もプロジェクトのカンバンカードと同様に表示する
  // （2026-08-28追加、ユーザー要望）。
  tags?: Tag[];
  start_date: string | null;
  due_date: string | null;
};

const STATUS_COLUMNS = [
  { id: "not_started", label: "未対応" },
  { id: "in_progress", label: "対応中" },
  { id: "done", label: "対応済" },
  { id: "on_hold", label: "保留" },
];

// マイタスク画面。プロジェクト横断で共通4ステータスにより集約する（spec.md 4.3）。
// D&Dで列を移動するとPATCH /tasks/:id {status} を呼び、
// プロジェクトのカンバン側にも自動反映される（バックエンドの同期ロジックはM2実装済み）。
export default function MyTasksPage() {
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const openTaskId = searchParams.get("task");
  const openCommentId = searchParams.get("comment") ?? undefined;

  const [tasks, setTasks] = useState<Task[]>([]);
  const [error, setError] = useState<string | null>(null);
  // 未指定（空文字）が既定動作＝今まで通り全件表示。日付を選ぶとその日以前が
  // 期限のタスクだけに絞り込む。
  const [date, setDate] = useState("");

  // TaskDetailPanelのonChangedがこの一覧の再取得完了までスピナーを出せるよう、
  // Promiseを返す（ユーザーフィードバック：保存後の反映が遅い体感の改善）。
  const load = useCallback(() => {
    if (!workspaceId) return Promise.resolve();
    const query = date ? `?date=${date}` : "";
    return apiFetch<Task[]>(`/workspaces/${workspaceId}/my-tasks${query}`)
      .then(setTasks)
      .catch(() => setError("マイタスクの取得に失敗しました"));
  }, [workspaceId, date]);

  useEffect(() => {
    load();
  }, [load]);

  function openTask(id: string) {
    const p = new URLSearchParams(searchParams);
    p.set("task", id);
    router.replace(`${pathname}?${p.toString()}`);
  }

  function closeTaskPanel() {
    const p = new URLSearchParams(searchParams);
    p.delete("task");
    const qs = p.toString();
    router.replace(qs ? `${pathname}?${qs}` : pathname);
  }

  // D&Dでの列移動は、サーバー往復を待たずに即座にローカル状態を更新する
  // （楽観的更新）。PATCHはバックグラウンドで行い、失敗時のみ再取得して巻き戻す。
  async function handleDrop(taskId: string, status: string) {
    setTasks((prev) =>
      prev.map((t) => (t.id === taskId ? { ...t, status: status as Task["status"] } : t)),
    );
    try {
      await apiFetch(`/tasks/${taskId}`, { method: "PATCH", body: JSON.stringify({ status }) });
    } catch {
      setError("タスクの更新に失敗しました");
      load();
    }
  }

  return (
    <div className="flex max-w-full flex-col gap-6 px-6 py-8 lg:px-10">
      <header className="flex flex-wrap items-center gap-3">
        <h1 className="text-2xl font-semibold tracking-tight text-foreground">マイタスク</h1>
        <div className="flex shrink-0 items-center gap-2">
          <CalendarClock className="h-4 w-4 shrink-0 text-muted-foreground" />
          <span className="shrink-0 whitespace-nowrap text-sm text-muted-foreground">基準日</span>
          <DatePicker value={date} onChange={setDate} placeholder="全期間" className="w-40" />
        </div>
      </header>
      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

      <KanbanBoard
        columns={STATUS_COLUMNS}
        itemsByColumn={Object.fromEntries(
          STATUS_COLUMNS.map((c) => [c.id, tasks.filter((t) => t.status === c.id)]),
        )}
        getItemId={(t) => t.id}
        onDrop={handleDrop}
        renderCard={(t) => (
          <button
            type="button"
            onClick={() => openTask(t.id)}
            className={clsx(
              "flex w-full flex-col gap-1.5 text-left",
              t.due_today && "border-l-2 border-red-500 pl-2",
            )}
          >
            <p className="text-sm font-medium text-foreground">{t.title}</p>
            {t.due_today && (
              <p className="text-xs font-medium text-red-600 dark:text-red-400">
                {date ? `${date}期限` : "本日期限"}
              </p>
            )}
            {t.tags && t.tags.length > 0 && (
              <div className="flex flex-wrap gap-1">
                {t.tags.map((tag) => (
                  <Badge key={tag.id} tone={tag.color as "zinc" | "red" | "amber" | "green" | "indigo"}>
                    {tag.name}
                  </Badge>
                ))}
              </div>
            )}
            {(t.start_date || t.due_date) && (
              <div className="flex w-full flex-col gap-0.5">
                {t.start_date && (
                  <span className="flex items-center gap-1 text-xs text-muted-foreground">
                    <Calendar className="h-3 w-3 shrink-0" />
                    開始: {t.start_date}
                  </span>
                )}
                {t.due_date && (
                  <span className="flex items-center gap-1 text-xs text-muted-foreground">
                    <Calendar className="h-3 w-3 shrink-0" />
                    期限: {t.due_date}
                  </span>
                )}
              </div>
            )}
          </button>
        )}
      />

      {openTaskId && (
        <TaskDetailPanel
          key={openTaskId}
          taskId={openTaskId}
          workspaceId={workspaceId}
          onClose={closeTaskPanel}
          onChanged={load}
          initialCommentId={openCommentId}
        />
      )}
    </div>
  );
}
