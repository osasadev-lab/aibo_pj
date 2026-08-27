"use client";

import { useParams } from "next/navigation";

import TaskDetailPanel from "@/components/TaskDetailPanel";

// タスク詳細のポップアウト先ページ（M8）。TaskDetailPanelの「ポップアウト」ボタンから
// window.openで開かれる想定の、左サイドバー等を持たない最小ページ
// （chromeの抑制はWorkspaceLayoutInnerのisPopoutRoute分岐で行う。
// docs/aibo/m8-implementation-plan.md スコープ追加C）。
export default function TaskPopoutPage() {
  const params = useParams<{ workspaceId: string; taskId: string }>();

  return (
    <TaskDetailPanel
      taskId={params.taskId}
      workspaceId={params.workspaceId}
      variant="page"
      onClose={() => window.close()}
    />
  );
}
