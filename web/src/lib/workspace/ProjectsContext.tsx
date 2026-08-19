"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

import { apiFetch } from "@/lib/apiClient";

export type WorkspaceProject = {
  id: string;
  name: string;
  description: string | null;
  visibility: "public" | "private";
  created_by: string;
  // 自分がこのプロジェクトの責任者(manager)かworkspace Ownerかどうか
  // （バックエンド側で判定済み）。設定画面でのタグ管理対象プロジェクトの絞り込みに使う。
  is_manager: boolean;
};

type ProjectsContextValue = {
  projects: WorkspaceProject[];
  refresh: () => void;
};

const ProjectsContext = createContext<ProjectsContextValue | null>(null);

// ワークスペース内のプロジェクト一覧を1箇所で取得・共有する。
// サイドバーのナビゲーションとプロジェクト一覧画面が別々にfetchしていたため、
// 一覧画面でプロジェクトを作成してもサイドバーに反映されない（要リロード）問題が
// あった。作成後にrefresh()を呼べば両方が同時に更新される。
export function ProjectsProvider({ workspaceId, children }: { workspaceId: string; children: ReactNode }) {
  const [projects, setProjects] = useState<WorkspaceProject[]>([]);

  const refresh = useCallback(() => {
    if (!workspaceId) return;
    apiFetch<WorkspaceProject[]>(`/workspaces/${workspaceId}/projects`)
      .then(setProjects)
      .catch(() => setProjects([]));
  }, [workspaceId]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const value = useMemo(() => ({ projects, refresh }), [projects, refresh]);

  return <ProjectsContext.Provider value={value}>{children}</ProjectsContext.Provider>;
}

export function useProjects(): ProjectsContextValue {
  const ctx = useContext(ProjectsContext);
  if (!ctx) {
    throw new Error("useProjects must be used within a ProjectsProvider");
  }
  return ctx;
}
