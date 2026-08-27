"use client";

import { useEffect, useState } from "react";
import { X } from "lucide-react";

import { apiFetch } from "@/lib/apiClient";
import IconButton from "@/components/ui/IconButton";

type Task = {
  id: string;
  title: string;
  status: "not_started" | "in_progress" | "done" | "on_hold";
  status_column_id: string | null;
  due_date: string | null;
};

type StatusColumn = { id: string; name: string };

const STATUS_LABELS: Record<Task["status"], string> = {
  not_started: "未対応",
  in_progress: "対応中",
  done: "対応済",
  on_hold: "保留",
};

type Props = {
  workspaceId: string;
  projectId: string;
  projectName: string;
  onClose: () => void;
  // 行クリックでタスク詳細を開く（2026-08-28追加、AssigneeTasksModalと同様）。
  onTaskClick: (taskId: string) => void;
};

// 進捗画面「プロジェクト別」の1行をクリックしたときに開く、そのプロジェクトの
// タスク一覧モーダル（AssigneeTasksModalの「担当者別」版と対になる構成、
// 2026-08-28追加）。タスク名-期限-カンバンステータスを表示し、行クリックで
// タスク詳細を開く。
export default function ProjectTasksModal({ workspaceId, projectId, projectName, onClose, onTaskClick }: Props) {
  const [tasks, setTasks] = useState<Task[] | null>(null);
  const [columnNames, setColumnNames] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let ignore = false;
    apiFetch<Task[]>(`/workspaces/${workspaceId}/tasks?project_id=${projectId}`)
      .then(async (list) => {
        if (ignore) return;
        setTasks(list);
        const cols = await apiFetch<StatusColumn[]>(`/projects/${projectId}/status-columns`).catch(
          () => [] as StatusColumn[],
        );
        if (ignore) return;
        const map: Record<string, string> = {};
        for (const col of cols) map[col.id] = col.name;
        setColumnNames(map);
      })
      .catch(() => setError("タスクの取得に失敗しました"));
    return () => {
      ignore = true;
    };
  }, [workspaceId, projectId]);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  function statusLabel(t: Task): string {
    if (t.status_column_id && columnNames[t.status_column_id]) return columnNames[t.status_column_id];
    return STATUS_LABELS[t.status];
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/50" onClick={onClose} />
      <div className="relative flex max-h-[80vh] w-full max-w-2xl flex-col rounded-xl border border-border bg-surface shadow-2xl">
        <div className="flex items-center justify-between border-b border-border px-5 py-4">
          <h2 className="text-sm font-semibold text-foreground">{projectName}のタスク</h2>
          <IconButton size="sm" onClick={onClose} title="閉じる">
            <X className="h-4 w-4" />
          </IconButton>
        </div>
        <div className="overflow-y-auto px-5 py-4">
          {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
          {!tasks ? (
            <p className="text-sm text-muted-foreground">読み込み中...</p>
          ) : tasks.length === 0 ? (
            <p className="text-sm text-muted-foreground">タスクがありません。</p>
          ) : (
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="text-xs text-muted-foreground">
                  <th className="pb-2 pr-3 font-medium">タスク名</th>
                  <th className="pb-2 pr-3 font-medium">期限</th>
                  <th className="pb-2 font-medium">ステータス</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {tasks.map((t) => (
                  <tr
                    key={t.id}
                    onClick={() => onTaskClick(t.id)}
                    className="cursor-pointer transition-colors hover:bg-surface-muted"
                  >
                    <td className="py-2 pr-3 align-top text-foreground">{t.title}</td>
                    <td className="py-2 pr-3 align-top text-muted-foreground">{t.due_date ?? "-"}</td>
                    <td className="py-2 align-top text-muted-foreground">{statusLabel(t)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  );
}
