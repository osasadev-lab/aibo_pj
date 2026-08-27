"use client";

import { useState } from "react";
import { MessageCircle } from "lucide-react";

import { apiFetch } from "@/lib/apiClient";
import Button from "@/components/ui/Button";
import MemberPicker from "@/components/MemberPicker";
import { Input } from "@/components/ui/fields";

type Props = {
  workspaceId: string;
  onClose: () => void;
  onCreated: (channelId: string) => void;
};

// 新規DM作成モーダル（M8.5）。選択した相手が1人なら1:1（既存チャンネルがあれば
// それを再利用）、2人以上なら常に新規グループを作成する（サーバー側の判定と同じ、
// docs/aibo/m8.5-implementation-plan.md）。
export default function NewDMDialog({ workspaceId, onClose, onCreated }: Props) {
  const [userIds, setUserIds] = useState<string[]>([]);
  const [groupName, setGroupName] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const isGroup = userIds.length >= 2;

  async function handleCreate() {
    if (userIds.length === 0) return;
    setCreating(true);
    setError(null);
    try {
      const res = await apiFetch<{ id: string }>(`/workspaces/${workspaceId}/dm/channels`, {
        method: "POST",
        body: JSON.stringify({
          user_ids: userIds,
          name: isGroup && groupName.trim() ? groupName.trim() : undefined,
        }),
      });
      onCreated(res.id);
    } catch {
      setError("チャンネルの作成に失敗しました");
    } finally {
      setCreating(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/50 backdrop-blur-[2px]" onClick={onClose} />
      <div className="relative flex w-full max-w-sm flex-col gap-3 rounded-xl border border-border bg-surface p-5 shadow-2xl">
        <div className="flex items-center gap-2">
          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-indigo-100 text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-400">
            <MessageCircle className="h-5 w-5" />
          </span>
          <h2 className="text-sm font-semibold text-foreground">新規DM</h2>
        </div>

        <MemberPicker workspaceId={workspaceId} selected={userIds} onChange={setUserIds} />

        {isGroup && (
          <Input
            value={groupName}
            onChange={(e) => setGroupName(e.target.value)}
            placeholder="グループ名（未入力ならメンバー名を表示）"
            className="text-sm"
          />
        )}

        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

        <div className="mt-1 flex justify-end gap-2">
          <Button variant="secondary" size="sm" onClick={onClose}>
            キャンセル
          </Button>
          <Button variant="primary" size="sm" disabled={userIds.length === 0 || creating} onClick={handleCreate}>
            {isGroup ? "グループを作成" : "DMを開始"}
          </Button>
        </div>
      </div>
    </div>
  );
}
