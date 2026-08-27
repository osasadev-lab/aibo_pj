"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { Pin } from "lucide-react";

import { apiFetch } from "@/lib/apiClient";
import Badge from "@/components/ui/Badge";
import SidePanel from "@/components/ui/SidePanel";

type PinnedTask = {
  id: string;
  title: string;
  project_id: string | null;
  project_name: string | null;
  status: "not_started" | "in_progress" | "done" | "on_hold";
  due_date: string | null;
};

const STATUS_LABELS: Record<PinnedTask["status"], string> = {
  not_started: "未対応",
  in_progress: "対応中",
  done: "対応済",
  on_hold: "保留",
};

// タスクのピン留め一覧（左サイドバー「ピン留め」から開く右サイドパネル、M8）。
// 表示範囲は現在のワークスペースのみ（docs/aibo/m8-implementation-plan.md スコープ追加B）。
export default function PinnedTasksPanel({ onClose }: { onClose: () => void }) {
  const router = useRouter();
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;

  const [tasks, setTasks] = useState<PinnedTask[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!workspaceId) return;
    apiFetch<PinnedTask[]>(`/workspaces/${workspaceId}/pinned-tasks`)
      .then(setTasks)
      .catch(() => setError("ピン留め一覧の取得に失敗しました"));
  }, [workspaceId]);

  function openTask(t: PinnedTask) {
    onClose();
    const href = t.project_id
      ? `/w/${workspaceId}/projects/${t.project_id}?task=${t.id}`
      : `/w/${workspaceId}/my-tasks?task=${t.id}`;
    router.push(href);
  }

  return (
    <SidePanel title="ピン留め" onClose={onClose}>
      {error && <p className="mb-3 text-sm text-red-600 dark:text-red-400">{error}</p>}
      {tasks === null ? (
        <p className="text-sm text-muted-foreground">読み込み中...</p>
      ) : tasks.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-16 text-center">
          <Pin className="h-6 w-6 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground">
            ピン留めしたタスクはここに表示されます。タスク詳細のピンアイコンから追加できます。
          </p>
        </div>
      ) : (
        <ul className="flex flex-col gap-2">
          {tasks.map((t) => (
            <li key={t.id}>
              <button
                type="button"
                onClick={() => openTask(t)}
                className="flex w-full items-center justify-between gap-3 rounded-xl border border-border bg-surface px-4 py-3 text-left text-sm transition-colors hover:border-indigo-300 dark:hover:border-indigo-500/50"
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-foreground">{t.title}</span>
                  <span className="text-xs text-muted-foreground">{t.project_name ?? "単体タスク"}</span>
                </span>
                <Badge>{STATUS_LABELS[t.status]}</Badge>
              </button>
            </li>
          ))}
        </ul>
      )}
    </SidePanel>
  );
}
