"use client";

import { useEffect, useRef, useState } from "react";
import {
  AlertTriangle,
  Calendar,
  Check,
  ChevronDown,
  Flag,
  Folder,
  Link as LinkIcon,
  Paperclip,
  Plus,
  Tag as TagIcon,
  Trash2,
  Upload,
  Users,
} from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";
import MemberPicker from "@/components/MemberPicker";
import TagPicker from "@/components/TagPicker";
import CommentThread from "@/components/CommentThread";
import Badge from "@/components/ui/Badge";
import type { Tag } from "@/lib/types";
import Button from "@/components/ui/Button";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import IconButton from "@/components/ui/IconButton";
import DatePicker from "@/components/ui/DatePicker";
import MarkdownToolbar from "@/components/ui/MarkdownToolbar";
import SidePanel from "@/components/ui/SidePanel";
import { Input, Select, Textarea } from "@/components/ui/fields";
import { useProjects } from "@/lib/workspace/ProjectsContext";
import type { MemberSummary } from "@/lib/types";

type Task = {
  id: string;
  parent_task_id: string | null;
  project_id: string | null;
  status: "not_started" | "in_progress" | "done" | "on_hold";
  title: string;
  description: string | null;
  priority: "low" | "medium" | "high" | null;
  due_date: string | null;
  assignee_ids?: string[];
  mentioned_user_ids?: string[];
  tags?: Tag[];
};

type DependencyTask = {
  id: string;
  title: string;
  status: "not_started" | "in_progress" | "done" | "on_hold";
  project_id: string | null;
};

type DependencyEntry = { id: string; task: DependencyTask };

type Dependencies = { predecessors: DependencyEntry[]; successors: DependencyEntry[] };

type Attachment = {
  id: string;
  file_name: string;
  size_bytes: number;
  content_type: string;
};

const MAX_ATTACHMENT_BYTES = 25 * 1024 * 1024;

function formatFileSize(bytes: number): string {
  if (bytes < 1024 * 1024) return `${Math.ceil(bytes / 1024)}KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
}

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
  const { user } = useAuth();
  const { projects } = useProjects();
  const storageEnabled = user?.storage_enabled ?? false;
  const [task, setTask] = useState<Task | null>(null);
  const [subtasks, setSubtasks] = useState<Task[]>([]);
  const [error, setError] = useState<string | null>(null);

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState("");
  const [dueDate, setDueDate] = useState("");
  const [assigneeIds, setAssigneeIds] = useState<string[]>([]);
  const [showAssignees, setShowAssignees] = useState(false);
  const [tagIds, setTagIds] = useState<string[]>([]);
  const [showTags, setShowTags] = useState(false);
  const [subtaskTitle, setSubtaskTitle] = useState("");
  const [mentionedIds, setMentionedIds] = useState<string[]>([]);
  const [mentionable, setMentionable] = useState<MemberSummary[]>([]);
  const [showMentions, setShowMentions] = useState(false);
  const [dependencies, setDependencies] = useState<Dependencies>({ predecessors: [], successors: [] });
  const [candidateTasks, setCandidateTasks] = useState<DependencyTask[]>([]);
  const [newDependencyId, setNewDependencyId] = useState("");
  const [dependencyError, setDependencyError] = useState<string | null>(null);
  const [showDoneConfirm, setShowDoneConfirm] = useState(false);
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [uploading, setUploading] = useState(false);
  const [attachmentError, setAttachmentError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const descriptionRef = useRef<HTMLTextAreaElement | null>(null);

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
        setTagIds((t.tags ?? []).map((tag) => tag.id));
        if (!t.parent_task_id) {
          apiFetch<Task[]>(`/tasks/${taskId}/subtasks`).then(setSubtasks).catch(() => {});
        } else {
          setSubtasks([]);
        }
      })
      .catch(() => setError("タスクの取得に失敗しました"));
  }

  function loadDependencies() {
    apiFetch<Dependencies>(`/tasks/${taskId}/dependencies`)
      .then(setDependencies)
      .catch(() => {});
  }

  function loadAttachments() {
    apiFetch<Attachment[]>(`/tasks/${taskId}/attachments`)
      .then(setAttachments)
      .catch(() => {});
  }

  // taskIdごとに呼び出し側が key={taskId} を付けて別インスタンスとしてマウントする
  // 想定（TaskDetailPanelはtaskId変更時の再利用を考慮しない）ため、
  // マウント時に1回だけ取得すればよい。
  useEffect(() => {
    loadTask();
    loadDependencies();
    loadAttachments();
    apiFetch<MemberSummary[]>(`/tasks/${taskId}/mentionable-members`)
      .then(setMentionable)
      .catch(() => {});
    // 先行タスク選択の候補一覧（ワークスペース全体のタスク）。
    apiFetch<DependencyTask[]>(`/workspaces/${workspaceId}/tasks`)
      .then((list) => setCandidateTasks(list.filter((t) => t.id !== taskId)))
      .catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const incompletePredecessors = dependencies.predecessors.filter((d) => d.task.status !== "done");

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

  async function markDone() {
    try {
      await apiFetch(`/tasks/${taskId}`, { method: "PATCH", body: JSON.stringify({ status: "done" }) });
      loadTask();
      onChanged?.();
    } catch {
      setError("タスクの更新に失敗しました");
    }
  }

  function handleMarkDone() {
    if (incompletePredecessors.length > 0) {
      setShowDoneConfirm(true);
      return;
    }
    markDone();
  }

  async function handleAddDependency(e: React.FormEvent) {
    e.preventDefault();
    if (!newDependencyId) return;
    setDependencyError(null);
    try {
      await apiFetch(`/tasks/${taskId}/dependencies`, {
        method: "POST",
        body: JSON.stringify({ depends_on_task_id: newDependencyId }),
      });
      setNewDependencyId("");
      loadDependencies();
    } catch (err) {
      const code =
        err && typeof err === "object" && "body" in err
          ? (err as { body?: { error?: string } }).body?.error
          : undefined;
      if (code === "circular_dependency") setDependencyError("循環依存になるため追加できません");
      else if (code === "already_depends_on") setDependencyError("既に先行タスクとして登録されています");
      else setDependencyError("先行タスクの追加に失敗しました");
    }
  }

  async function handleRemoveDependency(dependencyId: string) {
    try {
      await apiFetch(`/tasks/${taskId}/dependencies/${dependencyId}`, { method: "DELETE" });
      loadDependencies();
    } catch {
      setDependencyError("先行タスクの解除に失敗しました");
    }
  }

  async function handleCopyLink() {
    try {
      await navigator.clipboard.writeText(window.location.href);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setError("リンクのコピーに失敗しました");
    }
  }

  async function handleFileSelected(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;

    setAttachmentError(null);
    if (file.size > MAX_ATTACHMENT_BYTES) {
      setAttachmentError("ファイルサイズが25MBを超えています");
      return;
    }

    setUploading(true);
    try {
      const created = await apiFetch<Attachment & { upload_url: string }>(`/tasks/${taskId}/attachments`, {
        method: "POST",
        body: JSON.stringify({
          file_name: file.name,
          content_type: file.type || "application/octet-stream",
          size_bytes: file.size,
        }),
      });
      const putRes = await fetch(created.upload_url, {
        method: "PUT",
        headers: { "Content-Type": file.type || "application/octet-stream" },
        body: file,
      });
      if (!putRes.ok) throw new Error("upload failed");
      loadAttachments();
    } catch {
      setAttachmentError("添付ファイルのアップロードに失敗しました");
    } finally {
      setUploading(false);
    }
  }

  async function handleDeleteAttachment(attachmentId: string) {
    if (!window.confirm("この添付ファイルを削除しますか？")) return;
    try {
      await apiFetch(`/attachments/${attachmentId}`, { method: "DELETE" });
      loadAttachments();
    } catch {
      setAttachmentError("添付ファイルの削除に失敗しました");
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

  async function handleUpdateTags() {
    try {
      await apiFetch(`/tasks/${taskId}/tags`, {
        method: "PUT",
        body: JSON.stringify({ tag_ids: tagIds }),
      });
      loadTask();
      onChanged?.();
    } catch {
      setError("タグの更新に失敗しました");
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
          {task.project_id && (
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <Folder className="h-3 w-3" />
              {projects.find((p) => p.id === task.project_id)?.name ?? "..."}
            </div>
          )}
          <div className="flex items-center gap-1.5">
            <Input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              className="text-base font-medium"
              placeholder="タスク名"
            />
            <IconButton onClick={handleCopyLink} title="リンクをコピー">
              {copied ? <Check className="h-4 w-4 text-emerald-500" /> : <LinkIcon className="h-4 w-4" />}
            </IconButton>
          </div>
          <div className="relative">
            <MarkdownToolbar textareaRef={descriptionRef} value={description} onChange={handleDescriptionChange} />
            <Textarea
              ref={descriptionRef}
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

          <div className="flex flex-col gap-2">
            <button
              type="button"
              onClick={() => setShowTags((v) => !v)}
              className="flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
            >
              <TagIcon className="h-4 w-4" />
              タグ（{tagIds.length}件）
              <ChevronDown className={clsx("h-3.5 w-3.5 transition-transform", showTags && "rotate-180")} />
            </button>
            {!showTags && (task.tags?.length ?? 0) > 0 && (
              <div className="flex flex-wrap gap-1.5 pl-6">
                {task.tags!.map((tag) => (
                  <Badge key={tag.id} tone={tag.color as "zinc" | "red" | "amber" | "green" | "indigo"}>
                    {tag.name}
                  </Badge>
                ))}
              </div>
            )}
            {showTags && (
              <div className="flex flex-col gap-2 pl-6">
                <TagPicker taskId={taskId} selected={tagIds} onChange={setTagIds} />
                <Button variant="secondary" size="sm" className="self-start" onClick={handleUpdateTags}>
                  タグを保存
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

          <div className="flex flex-col gap-2 border-t border-border pt-4">
            <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">先行タスク</p>
            {dependencyError && <p className="text-sm text-red-600 dark:text-red-400">{dependencyError}</p>}
            {dependencies.predecessors.length > 0 && (
              <ul className="flex flex-col gap-1">
                {dependencies.predecessors.map((d) => (
                  <li
                    key={d.id}
                    className="flex items-center justify-between gap-2 rounded-lg px-2 py-1.5 text-sm hover:bg-surface-muted"
                  >
                    <span className="flex min-w-0 items-center gap-1.5">
                      {d.task.status !== "done" && (
                        <AlertTriangle className="h-3.5 w-3.5 shrink-0 text-amber-500" />
                      )}
                      <span className="truncate text-foreground">{d.task.title}</span>
                    </span>
                    <IconButton size="sm" onClick={() => handleRemoveDependency(d.id)} title="解除">
                      <Trash2 className="h-3.5 w-3.5" />
                    </IconButton>
                  </li>
                ))}
              </ul>
            )}
            <form onSubmit={handleAddDependency} className="flex gap-1.5">
              <Select
                value={newDependencyId}
                onChange={(e) => setNewDependencyId(e.target.value)}
                className="text-sm"
              >
                <option value="">先行タスクを選択...</option>
                {candidateTasks
                  .filter((t) => !dependencies.predecessors.some((d) => d.task.id === t.id))
                  .map((t) => (
                    <option key={t.id} value={t.id}>
                      {t.title}
                    </option>
                  ))}
              </Select>
              <Button type="submit" variant="secondary" size="sm">
                <Plus className="h-3.5 w-3.5" />
              </Button>
            </form>
          </div>

          <div className="flex flex-col gap-2 border-t border-border pt-4">
            <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">添付ファイル</p>
            {attachmentError && <p className="text-sm text-red-600 dark:text-red-400">{attachmentError}</p>}
            {attachments.length > 0 && (
              <ul className="flex flex-col gap-1">
                {attachments.map((a) => (
                  <li
                    key={a.id}
                    className="flex items-center justify-between gap-2 rounded-lg px-2 py-1.5 text-sm hover:bg-surface-muted"
                  >
                    <span className="flex min-w-0 items-center gap-1.5">
                      <Paperclip className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                      <span className="truncate text-foreground">{a.file_name}</span>
                      <span className="shrink-0 text-xs text-muted-foreground">{formatFileSize(a.size_bytes)}</span>
                    </span>
                    <IconButton size="sm" onClick={() => handleDeleteAttachment(a.id)} title="削除">
                      <Trash2 className="h-3.5 w-3.5" />
                    </IconButton>
                  </li>
                ))}
              </ul>
            )}
            {storageEnabled ? (
              <label className="flex w-fit cursor-pointer items-center gap-1.5 rounded-lg border border-border bg-surface px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-surface-muted hover:text-foreground">
                <Upload className="h-3.5 w-3.5" />
                {uploading ? "アップロード中..." : "ファイルを添付（25MBまで）"}
                <input type="file" className="hidden" onChange={handleFileSelected} disabled={uploading} />
              </label>
            ) : (
              attachments.length === 0 && (
                <p className="text-sm text-muted-foreground/70">
                  添付ファイル機能は現在無効化されています
                </p>
              )
            )}
          </div>

          <CommentThread taskId={taskId} />
        </div>
      )}

      <ConfirmDialog
        open={showDoneConfirm}
        title="未完了の先行タスクがあります"
        message="先行タスクが未完了のままですが、このタスクを対応済にしますか？"
        confirmLabel="対応済にする"
        onConfirm={() => {
          setShowDoneConfirm(false);
          markDone();
        }}
        onCancel={() => setShowDoneConfirm(false)}
      />
    </SidePanel>
  );
}
