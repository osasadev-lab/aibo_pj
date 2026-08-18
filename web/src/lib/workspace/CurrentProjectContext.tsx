"use client";

import { usePathname, useRouter } from "next/navigation";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import { apiFetch, ApiError } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";
import Avatar from "@/components/ui/Avatar";
import Button from "@/components/ui/Button";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import MemberPicker from "@/components/MemberPicker";
import SidePanel from "@/components/ui/SidePanel";
import { Select } from "@/components/ui/fields";
import { useProjects } from "@/lib/workspace/ProjectsContext";
import type { MemberSummary, ProjectMemberSummary, Workspace } from "@/lib/types";

export type CurrentProject = {
  id: string;
  workspace_id: string;
  name: string;
  description: string | null;
  visibility: "public" | "private";
  created_by: string;
};

type CurrentProjectContextValue = {
  project: CurrentProject | null;
  isManager: boolean;
  reload: () => void;
  openMembersPanel: () => void;
  openDeleteConfirm: () => void;
};

const CurrentProjectContext = createContext<CurrentProjectContextValue | null>(null);

// 現在表示中のプロジェクト（/w/:workspaceId/projects/:projectId配下）の情報・
// 責任者判定・参画メンバー管理/削除の状態を、左サイドバーとページ本体の両方で
// 共有するためのプロバイダ。サイドバーのプロジェクト名の横に参画メンバー/削除
// ボタンを置けるよう、パネル自体もここでレンダーする（レイアウト直下＝どのページ
// からでも同じ見た目・状態で開ける）。
export function CurrentProjectProvider({
  workspaceId,
  children,
}: {
  workspaceId: string;
  children: ReactNode;
}) {
  const { user } = useAuth();
  const router = useRouter();
  const pathname = usePathname();
  const { refresh: refreshProjects } = useProjects();

  const projectId = useMemo(() => {
    const m = pathname.match(/^\/w\/[^/]+\/projects\/([^/?]+)/);
    return m ? m[1] : null;
  }, [pathname]);

  const [workspaceRole, setWorkspaceRole] = useState<Workspace["role"] | undefined>(undefined);
  const [project, setProject] = useState<CurrentProject | null>(null);
  const [projectMembers, setProjectMembers] = useState<ProjectMemberSummary[]>([]);
  const [workspaceMembers, setWorkspaceMembers] = useState<MemberSummary[]>([]);
  const [editingMembers, setEditingMembers] = useState(false);
  const [memberIds, setMemberIds] = useState<string[]>([]);
  const [memberError, setMemberError] = useState<string | null>(null);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  useEffect(() => {
    if (!workspaceId) return;
    apiFetch<Workspace>(`/workspaces/${workspaceId}`)
      .then((w) => setWorkspaceRole(w.role))
      .catch(() => setWorkspaceRole(undefined));
  }, [workspaceId]);

  // 素早く別プロジェクトへ遷移した場合、古いリクエストのレスポンスが新しいものより
  // 後から返ってくると古いデータで上書きしてしまう（ネットワークのout-of-order）。
  // 呼び出し時点のprojectIdを最新値と比較し、ズレていれば結果を捨てて防ぐ。
  const latestProjectIdRef = useRef(projectId);
  useEffect(() => {
    latestProjectIdRef.current = projectId;
  }, [projectId]);

  // setStateは常にPromiseのthen内で行う（useEffect内で同期的にsetStateを呼ぶと
  // cascading renderになるというreact-hooks/set-state-in-effectの指摘を避けるため。
  // projectId無し・切り替え時のクリアも含めて非同期化する）。
  const reload = useCallback(() => {
    const requestedId = projectId;
    if (!requestedId) {
      Promise.resolve().then(() => {
        setProject(null);
        setProjectMembers([]);
      });
      return;
    }
    apiFetch<CurrentProject>(`/projects/${requestedId}`)
      .then((p) => {
        if (latestProjectIdRef.current === requestedId) setProject(p);
      })
      .catch(() => {
        if (latestProjectIdRef.current === requestedId) setProject(null);
      });
    apiFetch<ProjectMemberSummary[]>(`/projects/${requestedId}/members`)
      .then((members) => {
        if (latestProjectIdRef.current === requestedId) setProjectMembers(members);
      })
      .catch(() => {
        if (latestProjectIdRef.current === requestedId) setProjectMembers([]);
      });
  }, [projectId]);

  useEffect(() => {
    reload();
  }, [reload]);

  // プロジェクトを切り替えたら開きっぱなしのパネルは閉じる。
  useEffect(() => {
    Promise.resolve().then(() => {
      setEditingMembers(false);
      setConfirmingDelete(false);
    });
  }, [projectId]);

  const isManager =
    !!user &&
    (workspaceRole === "owner" || projectMembers.find((pm) => pm.user_id === user.id)?.role === "manager");

  // publicは参画という概念が無く「誰が責任者か」だけを編集する。private同様、
  // ワークスペース全員を一覧表示しプルダウンで責任者/スタッフを個別に切り替える
  // （現在のprojectMembersは常にmanagerのみ＝一覧中で責任者として選択済みの行）。
  // privateは参画メンバーの追加・削除（memberIds）を編集する。
  // openMembersPanel/openDeleteConfirmはcontext経由でサイドバー（別コンポーネント）
  // からも呼ばれるため、useCallbackで安定させclosure内のproject/projectMembersが
  // 古いまま参照され続けない（stale closure）ようにする。
  const openMembersPanel = useCallback(() => {
    if (!project) return;
    if (project.visibility === "private") {
      setMemberIds(projectMembers.map((pm) => pm.user_id));
    } else {
      apiFetch<MemberSummary[]>(`/workspaces/${workspaceId}/members`)
        .then(setWorkspaceMembers)
        .catch(() => setWorkspaceMembers([]));
    }
    setMemberError(null);
    setEditingMembers(true);
  }, [project, projectMembers, workspaceId]);

  const closeMembersPanel = useCallback(() => {
    setEditingMembers(false);
    setMemberError(null);
  }, []);

  async function handleSaveMembers() {
    if (!projectId) return;
    try {
      await apiFetch(`/projects/${projectId}/members`, {
        method: "PUT",
        body: JSON.stringify({ member_ids: memberIds }),
      });
      setEditingMembers(false);
      reload();
    } catch (err) {
      if (err instanceof ApiError && err.status === 400) {
        setMemberError("責任者が1人もいなくなるため更新できません");
      } else {
        setMemberError("参画メンバーの更新に失敗しました");
      }
    }
  }

  // 行ごとのプルダウン変更を即時反映する（private側のhandleChangeMemberRoleと
  // 同じUX）。現在の責任者集合はprojectMembersが常にmanagerのみを保持している
  // ためそれを真値とし、対象ユーザーの有無を入れ替えた配列をPUTする。
  async function handleToggleManager(userId: string, makeManager: boolean) {
    if (!projectId) return;
    const currentIds = projectMembers.map((pm) => pm.user_id);
    const nextIds = makeManager ? [...currentIds, userId] : currentIds.filter((id) => id !== userId);
    try {
      await apiFetch(`/projects/${projectId}/managers`, {
        method: "PUT",
        body: JSON.stringify({ user_ids: nextIds }),
      });
      reload();
    } catch (err) {
      if (err instanceof ApiError && err.status === 400) {
        setMemberError("責任者が1人もいなくなるため更新できません");
      } else {
        setMemberError("責任者の更新に失敗しました");
      }
    }
  }

  async function handleChangeMemberRole(memberId: string, role: ProjectMemberSummary["role"]) {
    if (!projectId) return;
    try {
      await apiFetch(`/projects/${projectId}/members/${memberId}`, {
        method: "PATCH",
        body: JSON.stringify({ role }),
      });
      reload();
    } catch (err) {
      if (err instanceof ApiError && err.status === 400) {
        setMemberError("最後の1人の責任者は降格できません");
      } else {
        setMemberError("責任者/スタッフの変更に失敗しました");
      }
    }
  }

  const openDeleteConfirm = useCallback(() => {
    setDeleteError(null);
    setConfirmingDelete(true);
  }, []);

  // タスク・コメント・カラム・参画メンバー等の紐づくデータもサーバー側で一括削除される
  // （server/internal/handler/project.go Delete参照）。
  async function handleDeleteProject() {
    if (!projectId) return;
    try {
      await apiFetch(`/projects/${projectId}`, { method: "DELETE" });
      refreshProjects();
      setConfirmingDelete(false);
      router.push(`/w/${workspaceId}/projects`);
    } catch {
      setDeleteError("プロジェクトの削除に失敗しました");
    }
  }

  const value = useMemo<CurrentProjectContextValue>(
    () => ({ project, isManager, reload, openMembersPanel, openDeleteConfirm }),
    [project, isManager, reload, openMembersPanel, openDeleteConfirm],
  );

  return (
    <CurrentProjectContext.Provider value={value}>
      {children}

      {editingMembers && project && (
        <SidePanel
          title={project.visibility === "private" ? "参画メンバー" : "責任者の付与"}
          onClose={closeMembersPanel}
        >
          {/* パネル内操作（保存・ロール変更）のエラーはパネル内だけに表示する。 */}
          {memberError && <p className="mb-3 text-sm text-red-600 dark:text-red-400">{memberError}</p>}
          {project.visibility === "private" ? (
            <div className="flex flex-col gap-3">
              <ul className="flex flex-col gap-1.5">
                {projectMembers.map((pm) => (
                  <li key={pm.id} className="flex items-center justify-between gap-3 text-sm">
                    <span className="flex min-w-0 items-center gap-2">
                      <Avatar name={pm.name} seed={pm.user_id} size="sm" />
                      <span className="truncate text-foreground">{pm.name}</span>
                    </span>
                    <Select
                      value={pm.role}
                      onChange={(e) => handleChangeMemberRole(pm.id, e.target.value as "manager" | "staff")}
                      className="w-auto py-1 text-xs"
                    >
                      <option value="manager">責任者</option>
                      <option value="staff">スタッフ</option>
                    </Select>
                  </li>
                ))}
              </ul>
              <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">参画メンバーの追加・削除</p>
              <MemberPicker workspaceId={workspaceId} selected={memberIds} onChange={setMemberIds} />
              <Button variant="primary" size="sm" className="self-start" onClick={handleSaveMembers}>
                保存
              </Button>
            </div>
          ) : (
            <div className="flex flex-col gap-3">
              {/* publicはワークスペース全員が閲覧できるため参画という概念が無く、
                  全員を一覧表示しプルダウンで責任者/スタッフを個別に切り替える
                  （対象はワークスペース全員）。 */}
              <ul className="flex flex-col gap-1.5">
                {workspaceMembers.map((m) => {
                  const isManagerRow = projectMembers.some((pm) => pm.user_id === m.user_id);
                  return (
                    <li key={m.user_id} className="flex items-center justify-between gap-3 text-sm">
                      <span className="flex min-w-0 items-center gap-2">
                        <Avatar name={m.name} seed={m.user_id} size="sm" />
                        <span className="truncate text-foreground">{m.name}</span>
                      </span>
                      <Select
                        value={isManagerRow ? "manager" : "staff"}
                        onChange={(e) => handleToggleManager(m.user_id, e.target.value === "manager")}
                        className="w-auto py-1 text-xs"
                      >
                        <option value="manager">責任者</option>
                        <option value="staff">スタッフ</option>
                      </Select>
                    </li>
                  );
                })}
              </ul>
            </div>
          )}
        </SidePanel>
      )}

      {project && (
        <ConfirmDialog
          open={confirmingDelete}
          title="プロジェクトを削除しますか？"
          message={`「${project.name}」を削除します。このプロジェクトのタスク・コメント・列などすべてのデータが削除され、元に戻せません。`}
          confirmLabel="削除する"
          error={deleteError}
          onConfirm={handleDeleteProject}
          onCancel={() => setConfirmingDelete(false)}
        />
      )}
    </CurrentProjectContext.Provider>
  );
}

export function useCurrentProject(): CurrentProjectContextValue {
  const ctx = useContext(CurrentProjectContext);
  if (!ctx) {
    throw new Error("useCurrentProject must be used within a CurrentProjectProvider");
  }
  return ctx;
}
