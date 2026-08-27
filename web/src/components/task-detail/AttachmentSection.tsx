"use client";

import { Paperclip, Trash2, Upload } from "lucide-react";

import IconButton from "@/components/ui/IconButton";

export type Attachment = {
  id: string;
  file_name: string;
  size_bytes: number;
  content_type: string;
};

function formatFileSize(bytes: number): string {
  if (bytes < 1024 * 1024) return `${Math.ceil(bytes / 1024)}KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
}

type Props = {
  attachments: Attachment[];
  error: string | null;
  uploading: boolean;
  storageEnabled: boolean;
  onUpload: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onDelete: (attachmentId: string) => void;
};

// タスク詳細の「添付ファイル」区画。状態・アップロード/削除の実処理は
// TaskDetailPanel側が持ち、ここは表示に徹する（純粋な見た目の切り出しのみで、
// ロジック・挙動は変更していない）。
export default function AttachmentSection({ attachments, error, uploading, storageEnabled, onUpload, onDelete }: Props) {
  return (
    <div className="flex flex-col gap-2 border-t border-border pt-4">
      <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">添付ファイル</p>
      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
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
              <IconButton size="sm" onClick={() => onDelete(a.id)} title="削除">
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
          <input type="file" className="hidden" onChange={onUpload} disabled={uploading} />
        </label>
      ) : (
        attachments.length === 0 && (
          <p className="text-sm text-muted-foreground/70">添付ファイル機能は現在無効化されています</p>
        )
      )}
    </div>
  );
}
