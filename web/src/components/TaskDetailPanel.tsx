"use client";

import { useEffect, useState } from "react";
import { Calendar, Check, ChevronDown, Flag, Plus, Trash2, Users } from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import MemberPicker from "@/components/MemberPicker";
import CommentThread from "@/components/CommentThread";
import Button from "@/components/ui/Button";
import IconButton from "@/components/ui/IconButton";
import DatePicker from "@/components/ui/DatePicker";
import SidePanel from "@/components/ui/SidePanel";
import { Input, Select, Textarea } from "@/components/ui/fields";
import type { MemberSummary } from "@/lib/types";

type Task = {
  id: string;
  parent_task_id: string | null;
  status: "not_started" | "in_progress" | "done" | "on_hold";
  title: string;
  description: string | null;
  priority: "low" | "medium" | "high" | null;
  due_date: string | null;
  assignee_ids?: string[];
  mentioned_user_ids?: string[];
};

type Props = {
  taskId: string;
  workspaceId: string;
  onClose: () => void;
  onChanged?: () => void;
};

// プロジェクト詳細・マイタスクで共用する右サイドバー形式のタスク詳細パネル。
// taskIdだけを受け取り、詳細（担当者・子タスクを含む）は自前で取得する
// （呼び出し元の一覧が持つタスクの形が画面ごとに異なるため）。
export default function TaskDetailPanel({ taskId, workspaceId, onClose, onChanged }: Props) {
  const [task, setTask] = useState<Task | null>(null);
  const [subtasks, setSubtasks] = useState<Task[]>([]);
  const [error, setError] = useState<string | null>(null);

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState("");
  const [dueDate, setDueDate] = useState("");
  const [assigneeIds, setAssigneeIds] = useState<string[]>([]);
  const [showAssignees, setShowAssignees] = useState(false);
  const [subtaskTitle, setSubtaskTitle] = useState("");
  const [mentionedIds, setMentionedIds] = useState<string[]>([]);
  const [mentionable, setMentionable] = useState<MemberSummary[]>([]);
  const [showMentions, setShowMentions] = useState(false);

  function loadTask() {
    apiFetch<Task>(`/tasks/${taskId}`)
      .then((t) => {
        setTask(t);
        setTitle(t.title);
        setDescription(t.description ?? "");
        setPriority(t.priority ?? "");
        setDueDate(t.due_date ?? "");
        setAssigneeIds(t.assignee_ids ?? []);
        setMentionedIds(t.mentioned_user_ids ?? []);
        if (!t.parent_task_id) {
          apiFetch<Task[]>(`/tasks/${taskId}/subtasks`).then(setSubtasks).catch(() => {});
        } else {
          setSubtasks([]);
        }
      })
      .catch(() => setError("タスクの取得に失敗しました"));
  }

  // taskIdごとに呼び出し側が key={taskId} を付けて別インスタンスとしてマウントする
  // 想定（TaskDetailPanelはtaskId変更時の再利用を考慮しない）ため、
  // マウント時に1回だけ取得すればよい。
  useEffect(() => {
    loadTask();
    apiFetch<MemberSummary[]>(`/tasks/${taskId}/mentionable-members`)
      .then(setMentionable)
      .catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function handleSave() {
    try {
      await apiFetch(`/tasks/${taskId}`, {
        method: "PATCH",
        body: JSON.stringify({
          title,
          description: description || null,
          priority: priority || null,
          due_date: dueDate || null,
          mentioned_user_ids: mentionedIds,
        }),
      });
      loadTask();
      onChanged?.();
    } catch {
      setError("タスクの更新に失敗しました");
    }
  }

  function handleDescriptionChange(value: string) {
    setDescription(value);
    setShowMentions(value.endsWith("@"));
  }

  function selectMention(m: MemberSummary) {
    setDescription((prev) => prev + m.name + " ");
    setMentionedIds((prev) => (prev.includes(m.user_id) ? prev : [...prev, m.user_id]));
    setShowMentions(false);
  }

  async function handleMarkDone() {
    try {
      await apiFetch(`/tasks/${taskId}`, { method: "PATCH", body: JSON.stringify({ status: "done" }) });
      loadTask();
      onChanged?.();
    } catch {
      setError("タスクの更新に失敗しました");
    }
  }

  async function handleUpdateAssignees() {
    try {
      await apiFetch(`/tasks/${taskId}/assignees`, {
        method: "PUT",
        body: JSON.stringify({ user_ids: assigneeIds }),
      });
      loadTask();
      onChanged?.();
    } catch {
      setError("担当者の更新に失敗しました");
    }
  }

  async function handleDelete() {
    if (!window.confirm("このタスクを削除しますか？（子タスクも削除されます）")) return;
    try {
      await apiFetch(`/tasks/${taskId}`, { method: "DELETE" });
      onChanged?.();
      onClose();
    } catch {
      setError("タスクの削除に失敗しました");
    }
  }

  async function handleAddSubtask(e: React.FormEvent) {
    e.preventDefault();
    if (!subtaskTitle.trim()) return;
    try {
      await apiFetch(`/tasks/${taskId}/subtasks`, {
        method: "POST",
        body: JSON.stringify({ title: subtaskTitle.trim() }),
      });
      setSubtaskTitle("");
      loadTask();
      onChanged?.();
    } catch {
      setError("子タスクの作成に失敗しました");
    }
  }

  async function handleDeleteSubtask(id: string) {
    if (!window.confirm("このタスクを削除しますか？（子タスクも削除されます）")) return;
    try {
      await apiFetch(`/tasks/${id}`, { method: "DELETE" });
      loadTask();
      onChanged?.();
    } catch {
      setError("タスクの削除に失敗しました");
    }
  }

  return (
    <SidePanel title="タスク詳細" onClose={onClose}>
      {error && (
        <p className="mb-3 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-600 dark:bg-red-950/40 dark:text-red-400">
          {error}
        </p>
      )}

      {!task ? (
        <p className="text-sm text-muted-foreground">読み込み中...</p>
      ) : (
        <div className="flex flex-col gap-4">
          <Input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            className="text-base font-medium"
            placeholder="タスク名"
          />
          <div className="relative">
            <Textarea
              value={description}
              onChange={(e) => handleDescriptionChange(e.target.value)}
              placeholder="説明を追加...（@でメンション）"
              rows={7}
              className="text-sm"
            />
            {showMentions && mentionable.length > 0 && (
              <ul className="absolute z-10 mt-1 max-h-32 w-full overflow-y-auto rounded-lg border border-border bg-surface text-xs shadow-lg">
                {mentionable.map((m) => (
                  <li key={m.user_id}>
                    <button
                      type="button"
                      onClick={() => selectMention(m)}
                      className="block w-full px-3 py-1.5 text-left hover:bg-surface-muted"
                    >
                      {m.name}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="flex flex-col gap-2">
            <div className="flex items-center gap-2">
              <Flag className="h-4 w-4 shrink-0 text-muted-foreground" />
              <Select value={priority} onChange={(e) => setPriority(e.target.value)} className="text-sm">
                <option value="">優先度なし</option>
                <option value="low">低</option>
                <option value="medium">中</option>
                <option value="high">高</option>
              </Select>
            </div>
            <div className="flex items-center gap-2">
              <Calendar className="h-4 w-4 shrink-0 text-muted-foreground" />
              <DatePicker value={dueDate} onChange={setDueDate} placeholder="期限を設定" />
            </div>
          </div>

          <div className="flex flex-col gap-2">
            <button
              type="button"
              onClick={() => setShowAssignees((v) => !v)}
              className="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
            >
              <Users className="h-4 w-4" />
              担当者（{assigneeIds.length}人）
              <ChevronDown className={clsx("h-3.5 w-3.5 transition-transform", showAssignees && "rotate-180")} />
            </button>
            {showAssignees && (
              <div className="flex flex-col gap-2 pl-6">
                <MemberPicker workspaceId={workspaceId} selected={assigneeIds} onChange={setAssigneeIds} />
                <Button variant="secondary" size="sm" className="self-start" onClick={handleUpdateAssignees}>
                  担当者を保存
                </Button>
              </div>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-2 border-t border-border pt-4">
            <Button variant="primary" size="sm" onClick={handleSave}>
              保存
            </Button>
            {task.status !== "done" && (
              <Button variant="secondary" size="sm" onClick={handleMarkDone}>
                <Check className="h-3.5 w-3.5" />
                対応済にする
              </Button>
            )}
            <Button variant="danger" size="sm" className="ml-auto" onClick={handleDelete}>
              <Trash2 className="h-3.5 w-3.5" />
              削除
            </Button>
          </div>

          {!task.parent_task_id && (
            <div className="flex flex-col gap-2 border-t border-border pt-4">
              <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">子タスク</p>
              {subtasks.length > 0 && (
                <ul className="flex flex-col gap-1">
                  {subtasks.map((st) => (
                    <li
                      key={st.id}
                      className="flex items-center justify-between rounded-lg px-2 py-1.5 text-sm hover:bg-surface-muted"
                    >
                      <span className="text-foreground">{st.title}</span>
                      <IconButton size="sm" onClick={() => handleDeleteSubtask(st.id)} title="削除">
                        <Trash2 className="h-3.5 w-3.5" />
                      </IconButton>
                    </li>
                  ))}
                </ul>
              )}
              <form onSubmit={handleAddSubtask} className="flex gap-1.5">
                <Input
                  value={subtaskTitle}
                  onChange={(e) => setSubtaskTitle(e.target.value)}
                  placeholder="子タスク名"
                  className="text-sm"
                />
                <Button type="submit" variant="secondary" size="sm">
                  <Plus className="h-3.5 w-3.5" />
                </Button>
              </form>
            </div>
          )}

          <CommentThread taskId={taskId} />
        </div>
      )}
    </SidePanel>
  );
}
