"use client";

import { useState } from "react";
import { usePathname, useParams } from "next/navigation";
import { Check } from "lucide-react";

import { apiFetch } from "@/lib/apiClient";
import Button from "@/components/ui/Button";
import SidePanel from "@/components/ui/SidePanel";
import { Textarea } from "@/components/ui/fields";

// 左サイドバー「フィードバック」から開く送信フォーム（M8）。DBに保存した上で、
// バックエンド側がコミット後にosasadev@gmail.com宛へメール通知する
// （docs/aibo/m8-implementation-plan.md スコープ追加D）。
export default function FeedbackPanel({ onClose }: { onClose: () => void }) {
  const pathname = usePathname();
  const params = useParams<{ workspaceId: string }>();

  const [body, setBody] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!body.trim()) return;
    setSending(true);
    setError(null);
    try {
      await apiFetch("/feedback", {
        method: "POST",
        body: JSON.stringify({
          body: body.trim(),
          workspace_id: params.workspaceId || undefined,
          page_path: pathname,
        }),
      });
      setSent(true);
      setBody("");
    } catch {
      setError("送信に失敗しました。時間をおいて再度お試しください");
    } finally {
      setSending(false);
    }
  }

  return (
    <SidePanel title="フィードバック" onClose={onClose}>
      {sent ? (
        <div className="flex flex-col items-center gap-2 py-16 text-center">
          <Check className="h-6 w-6 text-emerald-500" />
          <p className="text-sm text-foreground">送信しました。ありがとうございます！</p>
          <Button variant="secondary" size="sm" onClick={() => setSent(false)}>
            続けて送る
          </Button>
        </div>
      ) : (
        <form onSubmit={handleSubmit} className="flex flex-col gap-3">
          <p className="text-xs text-muted-foreground">
            ご意見・不具合報告など、お気づきの点をお知らせください。
          </p>
          {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
          <Textarea
            autoFocus
            rows={8}
            value={body}
            onChange={(e) => setBody(e.target.value)}
            placeholder="内容を入力してください"
          />
          <Button variant="primary" type="submit" disabled={sending || !body.trim()}>
            {sending ? "送信中..." : "送信する"}
          </Button>
        </form>
      )}
    </SidePanel>
  );
}
