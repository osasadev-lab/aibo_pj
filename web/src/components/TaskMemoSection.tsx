"use client";

import { useEffect, useRef, useState } from "react";
import { Loader2, Lock, Paperclip, Trash2, Upload } from "lucide-react";

import { apiFetch } from "@/lib/apiClient";
import Button from "@/components/ui/Button";
import IconButton from "@/components/ui/IconButton";
import MarkdownToolbar from "@/components/ui/MarkdownToolbar";
import { Textarea } from "@/components/ui/fields";

type MemoAttachment = { id: string; file_name: string; size_bytes: number; content_type: string };

type Memo = { task_id: string; body: string | null; attachments: MemoAttachment[] };

const MAX_ATTACHMENT_BYTES = 25 * 1024 * 1024;

function formatFileSize(bytes: number): string {
  if (bytes < 1024 * 1024) return `${Math.ceil(bytes / 1024)}KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
}

type Props = {
  taskId: string;
  storageEnabled: boolean;
};

// タスク詳細に埋め込む「自分にだけ表示」の個人メモ（M8.5）。他のメンバーには
// 一切表示されない専用データのため、CommentThreadと違いリアルタイム購読は無く、
// 明示的な保存ボタン方式にする（description欄と同じ考え方）。
export default function TaskMemoSection({ taskId, storageEnabled }: Props) {
  const [body, setBody] = useState("");
  const [attachments, setAttachments] = useState<MemoAttachment[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);

  function load() {
    return apiFetch<Memo>(`/tasks/${taskId}/memo`)
      .then((m) => {
        setBody(m.body ?? "");
        setAttachments(m.attachments);
      })
      .catch(() => setError("メモの取得に失敗しました"))
      .finally(() => setLoaded(true));
  }

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskId]);

  async function handleSave() {
    setError(null);
    setSaving(true);
    try {
      await apiFetch(`/tasks/${taskId}/memo`, { method: "PUT", body: JSON.stringify({ body }) });
      setSaved(true);
      setTimeout(() => setSaved(false), 1500);
    } catch {
      setError("メモの保存に失敗しました");
    } finally {
      setSaving(false);
    }
  }

  async function handleFileSelected(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;

    setError(null);
    if (file.size > MAX_ATTACHMENT_BYTES) {
      setError("ファイルサイズが25MBを超えています");
      return;
    }

    setUploading(true);
    try {
      const created = await apiFetch<MemoAttachment & { upload_url: string }>(`/tasks/${taskId}/memo/attachments`, {
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
      load();
    } catch {
      setError("添付ファイルのアップロードに失敗しました");
    } finally {
      setUploading(false);
    }
  }

  async function handleDeleteAttachment(attachmentId: string) {
    if (!window.confirm("この添付ファイルを削除しますか？")) return;
    try {
      await apiFetch(`/memo-attachments/${attachmentId}`, { method: "DELETE" });
      load();
    } catch {
      setError("添付ファイルの削除に失敗しました");
    }
  }

  if (!loaded) {
    return (
      <p className="flex items-center gap-2 border-t border-border pt-4 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        メモを読み込み中...
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-2 border-t border-border pt-4">
      <p className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        <Lock className="h-3 w-3" />
        メモ（あなたにだけ表示されます）
      </p>
      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
      <div>
        <MarkdownToolbar textareaRef={textareaRef} value={body} onChange={setBody} />
        <Textarea
          ref={textareaRef}
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="自分だけのメモを残せます..."
          rows={4}
          className="text-sm"
        />
      </div>

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
      {storageEnabled && (
        <label className="flex w-fit cursor-pointer items-center gap-1.5 rounded-lg border border-border bg-surface px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-surface-muted hover:text-foreground">
          <Upload className="h-3.5 w-3.5" />
          {uploading ? "アップロード中..." : "ファイルを添付（25MBまで）"}
          <input type="file" className="hidden" onChange={handleFileSelected} disabled={uploading} />
        </label>
      )}

      <Button variant="secondary" size="sm" className="self-start" disabled={saving} onClick={handleSave}>
        {saving && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
        {saved ? "保存しました" : "メモを保存"}
      </Button>
    </div>
  );
}
