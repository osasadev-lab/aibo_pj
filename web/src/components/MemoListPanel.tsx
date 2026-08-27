"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { NotebookPen } from "lucide-react";

import { apiFetch } from "@/lib/apiClient";
import SidePanel from "@/components/ui/SidePanel";

type TaskMemoEntry = {
  task_id: string;
  task_title: string;
  project_id: string | null;
  project_name: string | null;
  excerpt: string;
  updated_at: string;
};

// 自分がメモを持つタスクの一覧（左サイドバー「メモ」から開く右サイドパネル、M8.5）。
// 表示範囲は現在のワークスペースのみ（ピン留めと同じ考え方、
// docs/aibo/m8.5-implementation-plan.md）。
export default function MemoListPanel({ onClose }: { onClose: () => void }) {
  const router = useRouter();
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;

  const [memos, setMemos] = useState<TaskMemoEntry[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!workspaceId) return;
    apiFetch<TaskMemoEntry[]>(`/workspaces/${workspaceId}/task-memos`)
      .then(setMemos)
      .catch(() => setError("メモ一覧の取得に失敗しました"));
  }, [workspaceId]);

  function openTask(m: TaskMemoEntry) {
    onClose();
    const href = m.project_id
      ? `/w/${workspaceId}/projects/${m.project_id}?task=${m.task_id}`
      : `/w/${workspaceId}/my-tasks?task=${m.task_id}`;
    router.push(href);
  }

  return (
    <SidePanel title="メモ" onClose={onClose}>
      {error && <p className="mb-3 text-sm text-red-600 dark:text-red-400">{error}</p>}
      {memos === null ? (
        <p className="text-sm text-muted-foreground">読み込み中...</p>
      ) : memos.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-16 text-center">
          <NotebookPen className="h-6 w-6 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground">
            メモを残したタスクはここに表示されます。タスク詳細のメモ欄から追加できます。
          </p>
        </div>
      ) : (
        <ul className="flex flex-col gap-2">
          {memos.map((m) => (
            <li key={m.task_id}>
              <button
                type="button"
                onClick={() => openTask(m)}
                className="flex w-full flex-col gap-1 rounded-xl border border-border bg-surface px-4 py-3 text-left text-sm transition-colors hover:border-indigo-300 dark:hover:border-indigo-500/50"
              >
                <span className="flex items-center justify-between gap-2">
                  <span className="min-w-0 flex-1 truncate text-foreground">{m.task_title}</span>
                  <span className="shrink-0 text-xs text-muted-foreground">{m.project_name ?? "単体タスク"}</span>
                </span>
                {m.excerpt && <span className="truncate text-xs text-muted-foreground">{m.excerpt}</span>}
              </button>
            </li>
          ))}
        </ul>
      )}
    </SidePanel>
  );
}
