"use client";

import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { Calendar, Check, Pencil, Plus, Trash2, X } from "lucide-react";
import clsx from "clsx";

import { apiFetch, ApiError } from "@/lib/apiClient";
import KanbanBoard from "@/components/kanban/KanbanBoard";
import TaskDetailPanel from "@/components/TaskDetailPanel";
import Button from "@/components/ui/Button";
import IconButton from "@/components/ui/IconButton";
import Avatar from "@/components/ui/Avatar";
import { Input, Select, Textarea } from "@/components/ui/fields";
import { useProjects } from "@/lib/workspace/ProjectsContext";
import { useCurrentProject } from "@/lib/workspace/CurrentProjectContext";
import type { MemberSummary } from "@/lib/types";

type StatusColumn = {
  id: string;
  name: string;
  position: number;
  maps_to_status: "not_started" | "in_progress" | "done" | "on_hold";
  is_default: boolean;
};

type Task = {
  id: string;
  parent_task_id: string | null;
  status_column_id: string | null;
  status: "not_started" | "in_progress" | "done" | "on_hold";
  title: string;
  description: string | null;
  priority: "low" | "medium" | "high" | null;
  due_date: string | null;
  assignee_ids?: string[];
};

const MAPS_TO_STATUS_OPTIONS = [
  { value: "not_started", label: "未対応" },
  { value: "in_progress", label: "対応中" },
  { value: "done", label: "対応済" },
  { value: "on_hold", label: "保留" },
] as const;

const PRIORITY_DOT: Record<string, string> = {
  low: "bg-zinc-400",
  medium: "bg-amber-500",
  high: "bg-red-500",
};

export default function ProjectDetailPage() {
  const params = useParams<{ workspaceId: string; projectId: string }>();
  const { workspaceId, projectId } = params;
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const openTaskId = searchParams.get("task");
  const { refresh: refreshProjects } = useProjects();
  const { project, isManager, reload: reloadCurrentProject } = useCurrentProject();

  const [members, setMembers] = useState<MemberSummary[]>([]);
  const [columns, setColumns] = useState<StatusColumn[]>([]);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [addingToColumn, setAddingToColumn] = useState<string | null>(null);
  const [newTaskTitle, setNewTaskTitle] = useState("");
  const [newColumnName, setNewColumnName] = useState("");
  // 既定4列（未対応/対応中/対応済/保留）で共通ステータスは出揃うため、カスタム列を
  // 追加する場合の対応づけ先に「最も多いケース」は無い。プルダウン自体は必須
  // （内部的にどの共通ステータスに属するかを決める値のため省略できない）。
  const [newColumnMapsTo, setNewColumnMapsTo] = useState<string>("not_started");
  const [editingColumnId, setEditingColumnId] = useState<string | null>(null);
  const [editingColumnName, setEditingColumnName] = useState("");
  const [editingName, setEditingName] = useState(false);
  const [nameDraft, setNameDraft] = useState("");
  const [editingDescription, setEditingDescription] = useState(false);
  const [descriptionDraft, setDescriptionDraft] = useState("");

  const load = useCallback(() => {
    if (!projectId || !workspaceId) return;
    apiFetch<MemberSummary[]>(`/workspaces/${workspaceId}/members`).then(setMembers).catch(() => {});
    apiFetch<StatusColumn[]>(`/projects/${projectId}/status-columns`).then(setColumns);
    apiFetch<Task[]>(`/workspaces/${workspaceId}/tasks?project_id=${projectId}`).then(setTasks);
  }, [projectId, workspaceId]);

  useEffect(() => {
    load();
  }, [load]);

  const nameFor = (userId: string) => members.find((m) => m.user_id === userId)?.name ?? "?";

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

  async function handleAddTask(columnId: string) {
    if (!newTaskTitle.trim()) return;
    try {
      await apiFetch(`/workspaces/${workspaceId}/tasks`, {
        method: "POST",
        body: JSON.stringify({ title: newTaskTitle.trim(), project_id: projectId, status_column_id: columnId }),
      });
      setNewTaskTitle("");
      setAddingToColumn(null);
      load();
    } catch {
      setError("タスクの作成に失敗しました");
    }
  }

  // D&Dでの列移動は、サーバー往復を待たずに即座にローカル状態を更新する
  // （楽観的更新）。PATCHはバックグラウンドで行い、失敗時のみ再取得して巻き戻す。
  // status_column_idだけでなくstatusも移動先列のmaps_to_statusに合わせて更新する
  // （サーバー側のUpdateハンドラと同じ同期ロジック）。ここでstatusを更新しないと、
  // 「対応済にする」ボタンの表示条件（status!=="done"）などstatusを見ているUIが
  // 列移動後も古いstatusのまま取り残される。
  async function handleDrop(taskId: string, columnId: string) {
    const targetColumn = columns.find((c) => c.id === columnId);
    setTasks((prev) =>
      prev.map((t) =>
        t.id === taskId
          ? { ...t, status_column_id: columnId, status: targetColumn?.maps_to_status ?? t.status }
          : t,
      ),
    );
    try {
      await apiFetch(`/tasks/${taskId}`, {
        method: "PATCH",
        body: JSON.stringify({ status_column_id: columnId }),
      });
    } catch {
      setError("タスクの移動に失敗しました");
      load();
    }
  }

  async function handleAddColumn(e: React.FormEvent) {
    e.preventDefault();
    if (!newColumnName.trim()) return;
    try {
      await apiFetch(`/projects/${projectId}/status-columns`, {
        method: "POST",
        body: JSON.stringify({ name: newColumnName.trim(), maps_to_status: newColumnMapsTo }),
      });
      setNewColumnName("");
      load();
    } catch (err) {
      if (err instanceof ApiError && err.status === 400) {
        setError("列は最大5つまでです");
      } else {
        setError("列の作成に失敗しました");
      }
    }
  }

  // 列のD&D並び替え。カードD&Dと同様に楽観的更新し、バックグラウンドで
  // 各列のpositionをPATCHで永続化する（失敗時のみ再取得して巻き戻す）。
  async function handleReorderColumns(orderedIds: string[]) {
    setColumns((prev) => {
      const byId = new Map(prev.map((c) => [c.id, c]));
      return orderedIds.map((id, i) => ({ ...byId.get(id)!, position: i }));
    });
    try {
      await Promise.all(
        orderedIds.map((id, i) =>
          apiFetch(`/projects/${projectId}/status-columns/${id}`, {
            method: "PATCH",
            body: JSON.stringify({ position: i }),
          }),
        ),
      );
    } catch {
      setError("列の並び替えに失敗しました");
      load();
    }
  }

  // 列名の変更。既定列（未対応/対応中/対応済）は名前も固定表示のまま変更不可とし、
  // カスタム列（is_default=false）かつ責任者(isManager)のみ編集可能にする（削除制限と同じ線引き）。
  async function handleRenameColumn(columnId: string) {
    const name = editingColumnName.trim();
    if (!name) return;
    try {
      await apiFetch(`/projects/${projectId}/status-columns/${columnId}`, {
        method: "PATCH",
        body: JSON.stringify({ name }),
      });
      setEditingColumnId(null);
      load();
    } catch {
      setError("列名の変更に失敗しました");
    }
  }

  async function handleDeleteColumn(columnId: string) {
    try {
      await apiFetch(`/projects/${projectId}/status-columns/${columnId}`, { method: "DELETE" });
      load();
    } catch (err) {
      if (err instanceof ApiError && err.status === 400 && err.body && typeof err.body === "object" && "error" in err.body && (err.body as { error: string }).error === "tasks_present") {
        const target = window.prompt("この列にはタスクが残っています。移動先の列IDを入力してください:\n" + columns.filter((c) => c.id !== columnId).map((c) => `${c.id} (${c.name})`).join("\n"));
        if (target) {
          try {
            await apiFetch(`/projects/${projectId}/status-columns/${columnId}?target_column_id=${target}`, { method: "DELETE" });
            load();
          } catch {
            setError("列の削除に失敗しました");
          }
        }
      } else {
        setError("列の削除に失敗しました");
      }
    }
  }

  async function handleRenameProject() {
    const name = nameDraft.trim();
    if (!name || !project) return;
    try {
      await apiFetch(`/projects/${projectId}`, { method: "PATCH", body: JSON.stringify({ name }) });
      setEditingName(false);
      reloadCurrentProject();
      refreshProjects();
    } catch {
      setError("プロジェクト名の変更に失敗しました");
    }
  }

  async function handleSaveDescription() {
    try {
      await apiFetch(`/projects/${projectId}`, {
        method: "PATCH",
        body: JSON.stringify({ description: descriptionDraft.trim() || null }),
      });
      setEditingDescription(false);
      reloadCurrentProject();
    } catch {
      setError("プロジェクト説明の変更に失敗しました");
    }
  }

  if (!project) {
    return <div className="px-6 py-10 text-sm text-muted-foreground">{error ?? "読み込み中..."}</div>;
  }

  const topLevelTasks = tasks.filter((t) => !t.parent_task_id);

  return (
    <div className="flex max-w-full flex-col gap-6 px-6 py-8 lg:px-10">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          {editingName ? (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                handleRenameProject();
              }}
              className="flex items-center gap-1.5"
            >
              <Input
                autoFocus
                value={nameDraft}
                onChange={(e) => setNameDraft(e.target.value)}
                className="max-w-sm text-lg font-semibold"
              />
              <IconButton size="sm" type="submit" title="保存">
                <Check className="h-4 w-4" />
              </IconButton>
              <IconButton size="sm" type="button" title="取消" onClick={() => setEditingName(false)}>
                <X className="h-4 w-4" />
              </IconButton>
            </form>
          ) : (
            <h1 className="group flex items-center gap-2 text-2xl font-semibold tracking-tight text-foreground">
              {project.name}
              {isManager && (
                <button
                  type="button"
                  onClick={() => {
                    setNameDraft(project.name);
                    setEditingName(true);
                  }}
                  title="プロジェクト名を編集"
                >
                  <Pencil className="h-4 w-4 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
                </button>
              )}
            </h1>
          )}
          {editingDescription ? (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                handleSaveDescription();
              }}
              className="mt-1 flex items-start gap-1.5"
            >
              <Textarea
                autoFocus
                value={descriptionDraft}
                onChange={(e) => setDescriptionDraft(e.target.value)}
                placeholder="説明（任意）"
                rows={2}
                className="max-w-md text-sm"
              />
              <IconButton size="sm" type="submit" title="保存">
                <Check className="h-4 w-4" />
              </IconButton>
              <IconButton size="sm" type="button" title="取消" onClick={() => setEditingDescription(false)}>
                <X className="h-4 w-4" />
              </IconButton>
            </form>
          ) : (
            <div className="group mt-1 flex items-start gap-1.5">
              {project.description ? (
                <p className="text-sm text-muted-foreground">{project.description}</p>
              ) : (
                isManager && <p className="text-sm text-muted-foreground/50">説明はありません</p>
              )}
              {isManager && (
                <button
                  type="button"
                  onClick={() => {
                    setDescriptionDraft(project.description ?? "");
                    setEditingDescription(true);
                  }}
                  title="説明を編集"
                >
                  <Pencil className="h-3.5 w-3.5 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
                </button>
              )}
            </div>
          )}
        </div>
      </header>

      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

      <KanbanBoard
        columns={columns.map((c) => ({ id: c.id, label: c.name }))}
        itemsByColumn={Object.fromEntries(
          columns.map((c) => [c.id, topLevelTasks.filter((t) => t.status_column_id === c.id)]),
        )}
        getItemId={(t) => t.id}
        onDrop={handleDrop}
        onReorderColumns={isManager ? handleReorderColumns : undefined}
        renderColumnLabel={(columnId, label) => {
          const col = columns.find((c) => c.id === columnId);
          if (editingColumnId === columnId) {
            return (
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  handleRenameColumn(columnId);
                }}
                className="flex flex-1 items-center gap-1"
              >
                <Input
                  autoFocus
                  value={editingColumnName}
                  onChange={(e) => setEditingColumnName(e.target.value)}
                  className="h-7 px-2 py-0 text-xs"
                />
                <IconButton size="sm" type="submit" title="保存">
                  <Check className="h-3.5 w-3.5" />
                </IconButton>
                <IconButton size="sm" type="button" title="取消" onClick={() => setEditingColumnId(null)}>
                  <X className="h-3.5 w-3.5" />
                </IconButton>
              </form>
            );
          }
          if (col && !col.is_default && isManager) {
            return (
              <button
                type="button"
                onClick={() => {
                  setEditingColumnId(columnId);
                  setEditingColumnName(label);
                }}
                title="クリックして列名を編集"
                className="group flex flex-1 items-center gap-1.5 text-left text-sm font-semibold text-foreground"
              >
                {label}
                <Pencil className="h-3 w-3 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
              </button>
            );
          }
          return <span className="flex-1 text-sm font-semibold text-foreground">{label}</span>;
        }}
        renderColumnHeaderExtra={(columnId) => {
          const col = columns.find((c) => c.id === columnId);
          const count = topLevelTasks.filter((t) => t.status_column_id === columnId).length;
          return (
            <div className="flex items-center gap-1">
              <span className="rounded-full bg-surface px-1.5 py-0.5 text-xs text-muted-foreground">{count}</span>
              {col && !col.is_default && isManager && (
                <IconButton size="sm" title="列を削除" onClick={() => handleDeleteColumn(columnId)}>
                  <Trash2 className="h-3.5 w-3.5 hover:text-red-600" />
                </IconButton>
              )}
            </div>
          );
        }}
        renderCard={(t) => (
          <button type="button" onClick={() => openTask(t.id)} className="flex w-full flex-col items-start gap-2 text-left">
            <div className="flex w-full items-start justify-between gap-2">
              <span className="text-sm font-medium text-foreground">{t.title}</span>
              {t.priority && (
                <span className={clsx("mt-1.5 h-2 w-2 shrink-0 rounded-full", PRIORITY_DOT[t.priority])} />
              )}
            </div>
            {(t.due_date || (t.assignee_ids && t.assignee_ids.length > 0)) && (
              <div className="flex w-full items-center justify-between gap-2">
                {t.due_date ? (
                  <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                    <Calendar className="h-3 w-3" />
                    {t.due_date}
                  </span>
                ) : (
                  <span />
                )}
                {t.assignee_ids && t.assignee_ids.length > 0 && (
                  <div className="flex -space-x-1.5">
                    {t.assignee_ids.slice(0, 3).map((id) => (
                      <Avatar key={id} size="sm" name={nameFor(id)} seed={id} />
                    ))}
                  </div>
                )}
              </div>
            )}
          </button>
        )}
        renderColumnFooter={(columnId) =>
          addingToColumn === columnId ? (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                handleAddTask(columnId);
              }}
              className="flex flex-col gap-1.5"
            >
              <Input
                autoFocus
                value={newTaskTitle}
                onChange={(e) => setNewTaskTitle(e.target.value)}
                placeholder="タスク名"
                className="py-1.5 text-sm"
              />
              <div className="flex gap-1.5">
                <Button type="submit" variant="primary" size="sm">
                  追加
                </Button>
                <Button type="button" variant="ghost" size="sm" onClick={() => setAddingToColumn(null)}>
                  キャンセル
                </Button>
              </div>
            </form>
          ) : (
            <button
              onClick={() => {
                setAddingToColumn(columnId);
                setNewTaskTitle("");
              }}
              className="flex items-center gap-1 rounded-lg px-1 py-1.5 text-left text-xs text-muted-foreground transition-colors hover:bg-surface hover:text-foreground"
            >
              <Plus className="h-3.5 w-3.5" />
              タスク追加
            </button>
          )
        }
        trailingColumn={
          isManager ? (
            <form
              onSubmit={handleAddColumn}
              className="flex w-72 shrink-0 flex-col gap-2 rounded-xl border border-dashed border-border p-3"
            >
              <Input
                value={newColumnName}
                onChange={(e) => setNewColumnName(e.target.value)}
                placeholder="新しい列名"
              />
              <Select value={newColumnMapsTo} onChange={(e) => setNewColumnMapsTo(e.target.value)}>
                {MAPS_TO_STATUS_OPTIONS.map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </Select>
              <Button type="submit" variant="primary" size="sm">
                <Plus className="h-3.5 w-3.5" />
                列を追加
              </Button>
            </form>
          ) : undefined
        }
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
