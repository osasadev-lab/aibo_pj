"use client";

import { useEffect, useRef, useState } from "react";
import {
  Calendar,
  Check,
  ChevronDown,
  Flag,
  Folder,
  ExternalLink,
  Link as LinkIcon,
  Pin,
  PinOff,
  Loader2,
  Tag as TagIcon,
  Trash2,
  Users,
} from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";
import MemberPicker from "@/components/MemberPicker";
import TagPicker from "@/components/TagPicker";
import CommentThread from "@/components/CommentThread";
import TaskMemoSection from "@/components/TaskMemoSection";
import AttachmentSection, { type Attachment } from "@/components/task-detail/AttachmentSection";
import SubtaskSection from "@/components/task-detail/SubtaskSection";
import DependencySection, {
  type DependencyTask,
  type Dependencies,
} from "@/components/task-detail/DependencySection";
import Badge from "@/components/ui/Badge";
import type { Tag } from "@/lib/types";
import Button from "@/components/ui/Button";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import IconButton from "@/components/ui/IconButton";
import DateTimeField from "@/components/ui/DateTimeField";
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
  start_date: string | null;
  due_date: string | null;
  assignee_ids?: string[];
  mentioned_user_ids?: string[];
  tags?: Tag[];
  // M8追加：呼び出しユーザー視点のピン留め状態。
  is_pinned?: boolean;
};

const MAX_ATTACHMENT_BYTES = 25 * 1024 * 1024;

type Props = {
  taskId: string;
  workspaceId: string;
  onClose: () => void;
  // 呼び出し元一覧の再取得が完了するまで保存ボタンにスピナーを出したいため、
  // Promiseを返せるようにする（同期のvoidでも動くよう両方許容、ユーザーフィードバック）。
  onChanged?: () => void | Promise<void>;
  // M8追加：右サイドパネル（既定）か、ポップアウト用の素のページ表示かを切り替える
  // （docs/aibo/m8-implementation-plan.md スコープ追加C）。
  variant?: "panel" | "page";
  // 通知等、他画面から特定のコメントを指してこのタスクへ遷移してきた場合に渡す
  // （2026-08-27追加）。CommentThreadへそのまま転送し、該当コメントを表示・
  // ハイライトする。
  initialCommentId?: string;
};

// プロジェクト詳細・マイタスクで共用する右サイドバー形式のタスク詳細パネル。
// taskIdだけを受け取り、詳細（担当者・子タスクを含む）は自前で取得する
// （呼び出し元の一覧が持つタスクの形が画面ごとに異なるため）。
export default function TaskDetailPanel({
  taskId,
  workspaceId,
  onClose,
  onChanged,
  variant = "panel",
  initialCommentId,
}: Props) {
  const { user } = useAuth();
  const { projects } = useProjects();
  const storageEnabled = user?.storage_enabled ?? false;
  const [task, setTask] = useState<Task | null>(null);
  const [subtasks, setSubtasks] = useState<Task[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [pinned, setPinned] = useState(false);
  const [pinBusy, setPinBusy] = useState(false);

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState("");
  const [startDate, setStartDate] = useState("");
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
  const [saving, setSaving] = useState(false);
  // 「担当者やコメントなど全項目が揃うまで表示せずクルクルを出したい」との要望対応。
  // detailReadyは自分自身が持つ初回フェッチ（タスク本体・依存関係・添付ファイル・
  // メンション候補・先行タスク候補）が全て完了したことを、commentsReadyは埋め込みの
  // CommentThreadが自身の初回フェッチを終えたことを示す。両方揃うまでは中身を
  // 描画自体はする（CommentThreadを早期にマウントして並行でフェッチさせるため）が
  // hiddenクラスで非表示にし、揃った時点で一括表示する。
  const [detailReady, setDetailReady] = useState(false);
  const [commentsReady, setCommentsReady] = useState(false);
  const ready = detailReady && commentsReady;
  const descriptionRef = useRef<HTMLTextAreaElement | null>(null);

  // 戻り値のPromiseはマウント時のdetailReady判定用。保存後等の再取得（fire-and-forget
  // で呼ぶ箇所）ではPromiseはそのまま無視して構わない。
  function loadTask() {
    return apiFetch<Task>(`/tasks/${taskId}`)
      .then((t) => {
        setTask(t);
        setTitle(t.title);
        setDescription(t.description ?? "");
        setPriority(t.priority ?? "");
        setStartDate(t.start_date ?? "");
        setDueDate(t.due_date ?? "");
        setAssigneeIds(t.assignee_ids ?? []);
        setMentionedIds(t.mentioned_user_ids ?? []);
        setTagIds((t.tags ?? []).map((tag) => tag.id));
        setPinned(t.is_pinned ?? false);
        if (!t.parent_task_id) {
          return apiFetch<Task[]>(`/tasks/${taskId}/subtasks`).then(setSubtasks).catch(() => {});
        }
        setSubtasks([]);
      })
      .catch(() => setError("タスクの取得に失敗しました"));
  }

  function loadDependencies() {
    return apiFetch<Dependencies>(`/tasks/${taskId}/dependencies`)
      .then(setDependencies)
      .catch(() => {});
  }

  function loadAttachments() {
    return apiFetch<Attachment[]>(`/tasks/${taskId}/attachments`)
      .then(setAttachments)
      .catch(() => {});
  }

  // taskIdごとに呼び出し側が key={taskId} を付けて別インスタンスとしてマウントする
  // 想定（TaskDetailPanelはtaskId変更時の再利用を考慮しない）ため、
  // マウント時に1回だけ取得すればよい。
  useEffect(() => {
    Promise.allSettled([
      loadTask(),
      loadDependencies(),
      loadAttachments(),
      apiFetch<MemberSummary[]>(`/tasks/${taskId}/mentionable-members`).then(setMentionable).catch(() => {}),
      // 先行タスク選択の候補一覧（ワークスペース全体のタスク）。
      apiFetch<DependencyTask[]>(`/workspaces/${workspaceId}/tasks`)
        .then((list) => setCandidateTasks(list.filter((t) => t.id !== taskId)))
        .catch(() => {}),
    ]).then(() => setDetailReady(true));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const incompletePredecessors = dependencies.predecessors.filter((d) => d.task.status !== "done");

  // タイトル/説明/優先度/期限/メンションに加え、担当者・タグもこの「保存」1つに
  // まとめる（ユーザー要望）。それぞれ別APIのため順に呼び、途中で失敗したら
  // どの更新で失敗したか分かるようエラーメッセージを出し分ける。
  // 3つのPUT/PATCHを直列にawaitしていたため、保存完了（＝onChanged呼び出し・一覧再取得）
  // までにネットワーク往復3回分の待ち時間があり体感が遅かった。相互に独立した更新
  // （タスク本体/担当者/タグ）なので並列実行に変更し、待ち時間を最も遅い1回分に短縮する
  // （ユーザーフィードバックにより変更。1つが失敗しても他の成功分はそのまま反映される
  // 点が直列時と異なるが、フォームの意図した保存内容が部分的にでも残る方が望ましいと判断）。
  // 「反映されるまでクルクルを表示してほしい」との要望で、保存ボタン押下から
  // onChanged（呼び出し元一覧の再取得）が完了するまでをsavingで覆う。onChangedが
  // Promiseを返さない呼び出し元でも await はそのまま解決するだけなので害はない。
  async function handleSave() {
    setError(null);
    // 開始日時・期限を範囲として設定している場合、逆転していたら送信前に弾く
    // （サーバー側にも同じ検証があるが、往復せず即座にフィードバックするため）。
    if (startDate && dueDate && startDate > dueDate) {
      setError("開始日時は期限より前に設定してください");
      return;
    }
    setSaving(true);
    try {
      const [taskResult, assigneesResult, tagsResult] = await Promise.allSettled([
        apiFetch(`/tasks/${taskId}`, {
          method: "PATCH",
          body: JSON.stringify({
            title,
            description: description || null,
            priority: priority || null,
            start_date: startDate,
            due_date: dueDate,
            mentioned_user_ids: mentionedIds,
          }),
        }),
        apiFetch(`/tasks/${taskId}/assignees`, {
          method: "PUT",
          body: JSON.stringify({ user_ids: assigneeIds }),
        }),
        apiFetch(`/tasks/${taskId}/tags`, {
          method: "PUT",
          body: JSON.stringify({ tag_ids: tagIds }),
        }),
      ]);

      if (taskResult.status === "rejected") {
        setError("タスクの更新に失敗しました");
        return;
      }
      if (assigneesResult.status === "rejected") {
        setError("担当者の更新に失敗しました");
        return;
      }
      if (tagsResult.status === "rejected") {
        setError("タグの更新に失敗しました");
        return;
      }
      await onChanged?.();
      onClose();
    } finally {
      setSaving(false);
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
      onChanged?.();
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
      onChanged?.();
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

  // タスクのピン留め切替（M8）。楽観的更新し、失敗時は元に戻す。
  async function handleTogglePin() {
    const next = !pinned;
    setPinned(next);
    setPinBusy(true);
    try {
      await apiFetch(`/tasks/${taskId}/pin`, { method: next ? "POST" : "DELETE" });
    } catch {
      setPinned(!next);
      setError("ピン留めの更新に失敗しました");
    } finally {
      setPinBusy(false);
    }
  }

  // ポップアウト（M8）：このタスクを別ウィンドウで開き、自パネルは閉じる
  // （「切り離す」という意図的な挙動、docs/aibo/m8-implementation-plan.md スコープ追加C）。
  function handlePopout() {
    window.open(`/w/${workspaceId}/tasks/${taskId}`, "_blank", "noopener,width=640,height=840");
    onClose();
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

  const content = (
    <>
      {error && (
        <p className="mb-3 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-600 dark:bg-red-950/40 dark:text-red-400">
          {error}
        </p>
      )}

      {!ready && (
        <p className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          読み込み中...
        </p>
      )}

      {task && (
        <fieldset
          disabled={saving}
          className={clsx("m-0 flex min-w-0 flex-col gap-4 border-0 p-0", !ready && "hidden")}
        >
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
            <IconButton
              onClick={handleTogglePin}
              disabled={pinBusy}
              title={pinned ? "ピン留めを解除" : "ピン留め"}
            >
              {pinned ? (
                <PinOff className="h-4 w-4 text-indigo-600 dark:text-indigo-400" />
              ) : (
                <Pin className="h-4 w-4" />
              )}
            </IconButton>
            {variant === "panel" && (
              <IconButton onClick={handlePopout} title="ポップアウト">
                <ExternalLink className="h-4 w-4" />
              </IconButton>
            )}
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

          <AttachmentSection
            attachments={attachments}
            error={attachmentError}
            uploading={uploading}
            storageEnabled={storageEnabled}
            onUpload={handleFileSelected}
            onDelete={handleDeleteAttachment}
          />

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
            {/* 開始日時・期限（2026-08-27追加、日時範囲対応）。ラベルを横に並べると
                パネル幅（SidePanelのmax-w-xl）では時刻入力とぶつかって日付ボタンが
                潰れるため、ラベルは上に置く縦積みレイアウトにする。 */}
            <div className="flex flex-col gap-1">
              <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <Calendar className="h-3.5 w-3.5" />
                開始日時
              </span>
              <DateTimeField value={startDate} onChange={setStartDate} placeholder="開始日時を設定" />
            </div>
            <div className="flex flex-col gap-1">
              <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <Calendar className="h-3.5 w-3.5" />
                期限
              </span>
              <DateTimeField value={dueDate} onChange={setDueDate} placeholder="期限を設定" />
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
              </div>
            )}
          </div>

          {!task.parent_task_id && (
            <SubtaskSection
              subtasks={subtasks}
              subtaskTitle={subtaskTitle}
              onSubtaskTitleChange={setSubtaskTitle}
              onAdd={handleAddSubtask}
              onDelete={handleDeleteSubtask}
            />
          )}

          <DependencySection
            dependencies={dependencies}
            candidateTasks={candidateTasks}
            newDependencyId={newDependencyId}
            onNewDependencyIdChange={setNewDependencyId}
            error={dependencyError}
            onAdd={handleAddDependency}
            onRemove={handleRemoveDependency}
          />

          <div className="flex flex-wrap items-center gap-2 border-t border-border pt-4">
            <Button variant="primary" size="sm" disabled={saving} onClick={handleSave}>
              {saving && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
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

          <CommentThread
            taskId={taskId}
            initialCommentId={initialCommentId}
            onLoaded={() => setCommentsReady(true)}
          />

          <TaskMemoSection taskId={taskId} storageEnabled={storageEnabled} />
        </fieldset>
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
    </>
  );

  if (variant === "page") {
    return <div className="mx-auto max-w-3xl px-6 py-8">{content}</div>;
  }
  return (
    <SidePanel title="タスク詳細" onClose={onClose}>
      {content}
    </SidePanel>
  );
}
