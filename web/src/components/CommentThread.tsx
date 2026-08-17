"use client";

import { useEffect, useRef, useState } from "react";
import { Send } from "lucide-react";

import { apiFetch } from "@/lib/apiClient";
import { getSupabaseClient } from "@/lib/supabaseClient";
import Avatar from "@/components/ui/Avatar";
import Button from "@/components/ui/Button";
import { Textarea } from "@/components/ui/fields";

type Comment = {
  id: string;
  task_id: string;
  user_id: string;
  body: string;
  user_name?: string;
  created_at: string;
};

type Member = {
  id: string;
  user_id: string;
  name: string;
  email: string;
};

// タスク詳細に埋め込むコメントスレッド。初回一覧はREST APIから取得し、
// 以降の新着はSupabase Realtimeの直接購読で反映する（Goバックエンド非経由）。
// docs/aibo/m3-implementation-plan.md参照。
export default function CommentThread({ taskId }: { taskId: string }) {
  const [comments, setComments] = useState<Comment[]>([]);
  const [body, setBody] = useState("");
  const [mentionable, setMentionable] = useState<Member[]>([]);
  const [showMentions, setShowMentions] = useState(false);
  const [mentionedIds, setMentionedIds] = useState<string[]>([]);
  // Realtime経由のINSERTペイロードにはJOIN結果のuser_nameが含まれない
  // （生のcommentsテーブル行のみ）。参照可能メンバー一覧から名前を補完するため、
  // 常に最新値を読めるようrefにも保持する（effectのクロージャが古い値を
  // 参照してしまうのを避けるため）。
  const mentionableRef = useRef<Member[]>([]);

  useEffect(() => {
    let ignore = false;
    apiFetch<Comment[]>(`/tasks/${taskId}/comments`)
      .then((list) => {
        if (!ignore) setComments(list);
      })
      .catch(() => {});
    apiFetch<Member[]>(`/tasks/${taskId}/mentionable-members`)
      .then((list) => {
        if (!ignore) {
          setMentionable(list);
          mentionableRef.current = list;
        }
      })
      .catch(() => {});
    return () => {
      ignore = true;
    };
  }, [taskId]);

  useEffect(() => {
    const supabase = getSupabaseClient();
    const channel = supabase
      .channel(`comments:${taskId}`)
      .on(
        "postgres_changes",
        { event: "INSERT", schema: "public", table: "comments", filter: `task_id=eq.${taskId}` },
        (payload) => {
          const row = payload.new as Comment;
          const author = mentionableRef.current.find((m) => m.user_id === row.user_id);
          const enriched = author ? { ...row, user_name: author.name } : row;
          setComments((prev) => (prev.some((c) => c.id === row.id) ? prev : [...prev, enriched]));
        },
      )
      .subscribe();
    return () => {
      void supabase.removeChannel(channel);
    };
  }, [taskId]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!body.trim()) return;
    try {
      await apiFetch(`/tasks/${taskId}/comments`, {
        method: "POST",
        body: JSON.stringify({ body: body.trim(), mentioned_user_ids: mentionedIds }),
      });
      setBody("");
      setMentionedIds([]);
    } catch {
      // 送信失敗時は入力内容を残す
    }
  }

  function handleBodyChange(value: string) {
    setBody(value);
    setShowMentions(value.endsWith("@"));
  }

  function selectMention(m: Member) {
    setBody((prev) => prev + m.name + " ");
    setMentionedIds((prev) => (prev.includes(m.user_id) ? prev : [...prev, m.user_id]));
    setShowMentions(false);
  }

  return (
    <div className="flex flex-col gap-3 border-t border-border pt-4">
      <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">コメント</p>
      <ul className="flex max-h-64 flex-col gap-3 overflow-y-auto">
        {comments.map((c) => (
          <li key={c.id} className="flex items-start gap-2">
            <Avatar name={c.user_name ?? "?"} seed={c.user_id} size="sm" />
            <div className="min-w-0 flex-1 rounded-lg bg-surface-muted px-3 py-2">
              <div className="flex items-baseline gap-2">
                <span className="text-xs font-medium text-foreground">{c.user_name ?? "?"}</span>
                <span className="text-[11px] text-muted-foreground">{new Date(c.created_at).toLocaleString()}</span>
              </div>
              <p className="whitespace-pre-wrap text-sm text-foreground">{c.body}</p>
            </div>
          </li>
        ))}
      </ul>
      <form onSubmit={handleSubmit} className="relative flex flex-col gap-2">
        <Textarea
          value={body}
          onChange={(e) => handleBodyChange(e.target.value)}
          placeholder="コメントを入力（@でメンション）"
          rows={2}
          className="text-sm"
        />
        {showMentions && mentionable.length > 0 && (
          <ul className="absolute bottom-full z-10 mb-1 max-h-32 w-full overflow-y-auto rounded-lg border border-border bg-surface text-xs shadow-lg">
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
        <Button type="submit" variant="primary" size="sm" className="self-start">
          <Send className="h-3.5 w-3.5" />
          送信
        </Button>
      </form>
    </div>
  );
}
