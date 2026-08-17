"use client";

import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";

import { CalendarClock, X } from "lucide-react";

import { apiFetch } from "@/lib/apiClient";
import KanbanBoard from "@/components/kanban/KanbanBoard";
import TaskDetailPanel from "@/components/TaskDetailPanel";
import { Input } from "@/components/ui/fields";
import IconButton from "@/components/ui/IconButton";

type Task = {
  id: string;
  title: string;
  status: "not_started" | "in_progress" | "done" | "on_hold";
  project_id: string | null;
  due_today: boolean;
};

const STATUS_COLUMNS = [
  { id: "not_started", label: "未対応" },
  { id: "in_progress", label: "着手中" },
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

  const [tasks, setTasks] = useState<Task[]>([]);
  const [error, setError] = useState<string | null>(null);
  // 未指定（空文字）が既定動作＝今まで通り全件表示。日付を選ぶとその日以前が
  // 期限のタスクだけに絞り込む。
  const [date, setDate] = useState("");

  const load = useCallback(() => {
    if (!workspaceId) return;
    const query = date ? `?date=${date}` : "";
    apiFetch<Task[]>(`/workspaces/${workspaceId}/my-tasks${query}`)
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
      <header className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold tracking-tight text-foreground">マイタスク</h1>
        <div className="flex shrink-0 items-center gap-2">
          <CalendarClock className="h-4 w-4 shrink-0 text-muted-foreground" />
          <span className="shrink-0 whitespace-nowrap text-sm text-muted-foreground">基準日</span>
          <Input
            id="my-tasks-date"
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            className="w-40 shrink-0 text-sm"
          />
          {date && (
            <IconButton size="sm" onClick={() => setDate("")} title="クリア">
              <X className="h-3.5 w-3.5" />
            </IconButton>
          )}
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
            className={`w-full text-left ${t.due_today ? "border-l-2 border-red-500 pl-2" : ""}`}
          >
            <p className="text-sm font-medium text-foreground">{t.title}</p>
            {t.due_today && (
              <p className="mt-1 text-xs font-medium text-red-600 dark:text-red-400">
                {date ? `${date}期限` : "本日期限"}
              </p>
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
        />
      )}
    </div>
  );
}
