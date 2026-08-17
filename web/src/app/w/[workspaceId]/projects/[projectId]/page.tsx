"use client";

import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";

import { apiFetch, ApiError } from "@/lib/apiClient";
import MemberPicker from "@/components/MemberPicker";

type Project = {
  id: string;
  workspace_id: string;
  name: string;
  description: string | null;
  visibility: "public" | "private";
};

type StatusColumn = {
  id: string;
  name: string;
  position: number;
  maps_to_status: "not_started" | "in_progress" | "done" | "on_hold";
};

type Task = {
  id: string;
  parent_task_id: string | null;
  status_column_id: string | null;
  title: string;
  description: string | null;
  priority: "low" | "medium" | "high" | null;
  due_date: string | null;
};

const MAPS_TO_STATUS_OPTIONS = [
  { value: "not_started", label: "未対応" },
  { value: "in_progress", label: "着手中" },
  { value: "done", label: "対応済" },
  { value: "on_hold", label: "保留" },
] as const;

export default function ProjectDetailPage() {
  const params = useParams<{ workspaceId: string; projectId: string }>();
  const { workspaceId, projectId } = params;

  const [project, setProject] = useState<Project | null>(null);
  const [columns, setColumns] = useState<StatusColumn[]>([]);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [expandedTaskId, setExpandedTaskId] = useState<string | null>(null);
  const [addingToColumn, setAddingToColumn] = useState<string | null>(null);
  const [newTaskTitle, setNewTaskTitle] = useState("");
  const [newColumnName, setNewColumnName] = useState("");
  const [newColumnMapsTo, setNewColumnMapsTo] = useState<string>("not_started");
  const [editingMembers, setEditingMembers] = useState(false);
  const [memberIds, setMemberIds] = useState<string[]>([]);

  const load = useCallback(() => {
    if (!projectId || !workspaceId) return;
    apiFetch<Project>(`/projects/${projectId}`)
      .then(setProject)
      .catch(() => setError("プロジェクトの取得に失敗しました（参画メンバーでない可能性があります）"));
    apiFetch<StatusColumn[]>(`/projects/${projectId}/status-columns`).then(setColumns);
    apiFetch<Task[]>(`/workspaces/${workspaceId}/tasks?project_id=${projectId}`).then(setTasks);
  }, [projectId, workspaceId]);

  useEffect(() => {
    load();
  }, [load]);

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

  async function handleUpdateTask(taskId: string, patch: Record<string, unknown>) {
    try {
      await apiFetch(`/tasks/${taskId}`, { method: "PATCH", body: JSON.stringify(patch) });
      load();
    } catch {
      setError("タスクの更新に失敗しました");
    }
  }

  async function handleDeleteTask(taskId: string) {
    if (!window.confirm("このタスクを削除しますか？（子タスクも削除されます）")) return;
    try {
      await apiFetch(`/tasks/${taskId}`, { method: "DELETE" });
      setExpandedTaskId(null);
      load();
    } catch {
      setError("タスクの削除に失敗しました");
    }
  }

  async function handleAddSubtask(parentId: string, title: string) {
    if (!title.trim()) return;
    try {
      await apiFetch(`/tasks/${parentId}/subtasks`, {
        method: "POST",
        body: JSON.stringify({ title: title.trim() }),
      });
      load();
    } catch {
      setError("子タスクの作成に失敗しました");
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

  async function handleSaveMembers() {
    try {
      await apiFetch(`/projects/${projectId}/members`, {
        method: "PUT",
        body: JSON.stringify({ member_ids: memberIds }),
      });
      setEditingMembers(false);
    } catch {
      setError("参画メンバーの更新に失敗しました");
    }
  }

  if (!project) {
    return <div className="px-4 py-10 text-sm text-zinc-500">{error ?? "読み込み中..."}</div>;
  }

  const topLevelTasks = tasks.filter((t) => !t.parent_task_id);
  const subtasksOf = (taskId: string) => tasks.filter((t) => t.parent_task_id === taskId);

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-6 px-4 py-10">
      <header>
        <h1 className="text-xl font-semibold">{project.name}</h1>
        {project.description && <p className="text-sm text-zinc-500">{project.description}</p>}
        <p className="text-xs text-zinc-500">{project.visibility === "private" ? "Private" : "Public"}</p>
      </header>

      {error && <p className="text-sm text-red-600">{error}</p>}

      <section>
        <button onClick={() => setEditingMembers((v) => !v)} className="text-sm underline">
          参画メンバーを編集
        </button>
        {editingMembers && (
          <div className="mt-2 flex flex-col gap-2">
            <MemberPicker workspaceId={workspaceId} selected={memberIds} onChange={setMemberIds} />
            <button
              onClick={handleSaveMembers}
              className="self-start rounded-md bg-black px-3 py-1.5 text-sm text-white dark:bg-white dark:text-black"
            >
              保存
            </button>
          </div>
        )}
      </section>

      <div className="flex gap-4 overflow-x-auto pb-4">
        {columns.map((col) => (
          <div key={col.id} className="flex w-64 shrink-0 flex-col gap-2 rounded-md border border-zinc-200 p-3 dark:border-zinc-800">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">{col.name}</span>
              <button onClick={() => handleDeleteColumn(col.id)} className="text-xs text-red-600 hover:underline">
                削除
              </button>
            </div>

            <ul className="flex flex-col gap-2">
              {topLevelTasks
                .filter((t) => t.status_column_id === col.id)
                .map((t) => (
                  <li key={t.id} className="rounded-md border border-zinc-200 p-2 text-sm dark:border-zinc-700">
                    <button className="text-left font-medium" onClick={() => setExpandedTaskId(expandedTaskId === t.id ? null : t.id)}>
                      {t.title}
                    </button>

                    {expandedTaskId === t.id && (
                      <TaskEditor
                        task={t}
                        subtasks={subtasksOf(t.id)}
                        onUpdate={(patch) => handleUpdateTask(t.id, patch)}
                        onDelete={() => handleDeleteTask(t.id)}
                        onAddSubtask={(title) => handleAddSubtask(t.id, title)}
                        onDeleteSubtask={handleDeleteTask}
                      />
                    )}
                  </li>
                ))}
            </ul>

            {addingToColumn === col.id ? (
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  handleAddTask(col.id);
                }}
                className="flex flex-col gap-1"
              >
                <input
                  autoFocus
                  value={newTaskTitle}
                  onChange={(e) => setNewTaskTitle(e.target.value)}
                  placeholder="タスク名"
                  className="rounded-md border border-zinc-300 px-2 py-1 text-sm dark:border-zinc-700 dark:bg-zinc-900"
                />
                <div className="flex gap-1">
                  <button type="submit" className="rounded-md bg-black px-2 py-1 text-xs text-white dark:bg-white dark:text-black">
                    追加
                  </button>
                  <button type="button" onClick={() => setAddingToColumn(null)} className="text-xs text-zinc-500">
                    キャンセル
                  </button>
                </div>
              </form>
            ) : (
              <button
                onClick={() => {
                  setAddingToColumn(col.id);
                  setNewTaskTitle("");
                }}
                className="text-left text-xs text-zinc-500 hover:underline"
              >
                ＋タスク追加
              </button>
            )}
          </div>
        ))}

        <form onSubmit={handleAddColumn} className="flex w-64 shrink-0 flex-col gap-2 rounded-md border border-dashed border-zinc-300 p-3 dark:border-zinc-700">
          <input
            value={newColumnName}
            onChange={(e) => setNewColumnName(e.target.value)}
            placeholder="新しい列名"
            className="rounded-md border border-zinc-300 px-2 py-1 text-sm dark:border-zinc-700 dark:bg-zinc-900"
          />
          <select
            value={newColumnMapsTo}
            onChange={(e) => setNewColumnMapsTo(e.target.value)}
            className="rounded-md border border-zinc-300 px-2 py-1 text-sm dark:border-zinc-700 dark:bg-zinc-900"
          >
            {MAPS_TO_STATUS_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
          <button type="submit" className="rounded-md bg-black px-2 py-1 text-xs text-white dark:bg-white dark:text-black">
            列を追加
          </button>
        </form>
      </div>
    </div>
  );
}

function TaskEditor({
  task,
  subtasks,
  onUpdate,
  onDelete,
  onAddSubtask,
  onDeleteSubtask,
}: {
  task: Task;
  subtasks: Task[];
  onUpdate: (patch: Record<string, unknown>) => void;
  onDelete: () => void;
  onAddSubtask: (title: string) => void;
  onDeleteSubtask: (id: string) => void;
}) {
  const [title, setTitle] = useState(task.title);
  const [description, setDescription] = useState(task.description ?? "");
  const [priority, setPriority] = useState(task.priority ?? "");
  const [dueDate, setDueDate] = useState(task.due_date ?? "");
  const [subtaskTitle, setSubtaskTitle] = useState("");

  return (
    <div className="mt-2 flex flex-col gap-2 border-t border-zinc-200 pt-2 dark:border-zinc-700">
      <input
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        className="rounded-md border border-zinc-300 px-2 py-1 text-sm dark:border-zinc-700 dark:bg-zinc-900"
      />
      <textarea
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        placeholder="説明"
        rows={2}
        className="rounded-md border border-zinc-300 px-2 py-1 text-sm dark:border-zinc-700 dark:bg-zinc-900"
      />
      <div className="flex gap-2">
        <select
          value={priority}
          onChange={(e) => setPriority(e.target.value)}
          className="rounded-md border border-zinc-300 px-2 py-1 text-xs dark:border-zinc-700 dark:bg-zinc-900"
        >
          <option value="">優先度なし</option>
          <option value="low">低</option>
          <option value="medium">中</option>
          <option value="high">高</option>
        </select>
        <input
          type="date"
          value={dueDate}
          onChange={(e) => setDueDate(e.target.value)}
          className="rounded-md border border-zinc-300 px-2 py-1 text-xs dark:border-zinc-700 dark:bg-zinc-900"
        />
      </div>
      <div className="flex gap-2">
        <button
          onClick={() =>
            onUpdate({
              title,
              description: description || null,
              priority: priority || null,
              due_date: dueDate || null,
            })
          }
          className="rounded-md bg-black px-2 py-1 text-xs text-white dark:bg-white dark:text-black"
        >
          保存
        </button>
        <button onClick={onDelete} className="text-xs text-red-600 hover:underline">
          削除
        </button>
      </div>

      {!task.parent_task_id && (
        <div className="mt-2 flex flex-col gap-1 border-t border-zinc-200 pt-2 dark:border-zinc-700">
          <p className="text-xs font-medium text-zinc-500">子タスク</p>
          <ul className="flex flex-col gap-1">
            {subtasks.map((st) => (
              <li key={st.id} className="flex items-center justify-between text-xs">
                <span>{st.title}</span>
                <button onClick={() => onDeleteSubtask(st.id)} className="text-red-600 hover:underline">
                  削除
                </button>
              </li>
            ))}
          </ul>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              onAddSubtask(subtaskTitle);
              setSubtaskTitle("");
            }}
            className="flex gap-1"
          >
            <input
              value={subtaskTitle}
              onChange={(e) => setSubtaskTitle(e.target.value)}
              placeholder="子タスク名"
              className="flex-1 rounded-md border border-zinc-300 px-2 py-1 text-xs dark:border-zinc-700 dark:bg-zinc-900"
            />
            <button type="submit" className="rounded-md border border-zinc-300 px-2 py-1 text-xs dark:border-zinc-700">
              ＋子タスクを追加
            </button>
          </form>
        </div>
      )}
    </div>
  );
}
