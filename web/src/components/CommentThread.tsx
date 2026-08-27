"use client";

import { useEffect, useRef, useState } from "react";
import { Loader2, Send } from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import { getSupabaseClient } from "@/lib/supabaseClient";
import { renderMarkdown } from "@/lib/markdown";
import Avatar from "@/components/ui/Avatar";
import Button from "@/components/ui/Button";
import MarkdownToolbar from "@/components/ui/MarkdownToolbar";
import { Textarea } from "@/components/ui/fields";
import type { MemberSummary } from "@/lib/types";

type Comment = {
  id: string;
  task_id: string;
  user_id: string;
  body: string;
  user_name?: string;
  created_at: string;
};

type ListResponse = {
  items: Comment[];
  has_more_older: boolean;
  has_more_newer: boolean;
};

const PAGE_SIZE = 30;

type Props = {
  taskId: string;
  onLoaded?: () => void;
  // 通知等、他画面から特定のコメントを指してこのタスクへ遷移してきた場合に渡す
  // （M8後の追加改修、2026-08-27）。指定時は最新30件ではなく、そのコメントを
  // 中心とした窓を取得し、表示後にスクロール＋一時ハイライトする。
  initialCommentId?: string;
};

// タスク詳細に埋め込むコメントスレッド。初回一覧はREST APIから取得し、
// 以降の新着はSupabase Realtimeの直接購読で反映する（Goバックエンド非経由）。
// docs/aibo/m3-implementation-plan.md参照。
// onLoadedは初回の一覧・メンション候補取得が両方完了した時点で1回呼ぶ
// （呼び出し元のTaskDetailPanelが「全項目が揃うまでクルクル」を実現するための
// 完了通知、ユーザーフィードバック）。
//
// ページネーション（2026-08-27追加）：既定は直近PAGE_SIZE件を表示し、上に
// スクロールする形の「もっと見る」で古いコメントを遡って読み込む。initialCommentId
// 指定時はその前後の窓を取得し、両端に「もっと見る」を出す。
export default function CommentThread({ taskId, onLoaded, initialCommentId }: Props) {
  const [comments, setComments] = useState<Comment[]>([]);
  const [hasMoreOlder, setHasMoreOlder] = useState(false);
  const [hasMoreNewer, setHasMoreNewer] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [loadingNewer, setLoadingNewer] = useState(false);
  const [highlightId, setHighlightId] = useState<string | null>(null);
  const [body, setBody] = useState("");
  const [mentionable, setMentionable] = useState<MemberSummary[]>([]);
  const [showMentions, setShowMentions] = useState(false);
  const [mentionedIds, setMentionedIds] = useState<string[]>([]);
  // Realtime経由のINSERTペイロードにはJOIN結果のuser_nameが含まれない
  // （生のcommentsテーブル行のみ）。参照可能メンバー一覧から名前を補完するため、
  // 常に最新値を読めるようrefにも保持する（effectのクロージャが古い値を
  // 参照してしまうのを避けるため）。
  const mentionableRef = useRef<MemberSummary[]>([]);
  const composerRef = useRef<HTMLTextAreaElement | null>(null);
  const scrolledToHighlightRef = useRef(false);

  useEffect(() => {
    let ignore = false;
    const query = initialCommentId
      ? `?around=${encodeURIComponent(initialCommentId)}`
      : `?limit=${PAGE_SIZE}`;
    const commentsPromise = apiFetch<ListResponse>(`/tasks/${taskId}/comments${query}`)
      .then((res) => {
        if (ignore) return;
        setComments(res.items);
        setHasMoreOlder(res.has_more_older);
        setHasMoreNewer(res.has_more_newer);
        if (initialCommentId) setHighlightId(initialCommentId);
      })
      .catch(() => {});
    const mentionablePromise = apiFetch<MemberSummary[]>(`/tasks/${taskId}/mentionable-members`)
      .then((list) => {
        if (!ignore) {
          setMentionable(list);
          mentionableRef.current = list;
        }
      })
      .catch(() => {});
    Promise.allSettled([commentsPromise, mentionablePromise]).then(() => {
      if (!ignore) onLoaded?.();
    });
    return () => {
      ignore = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskId]);

  // ハイライト対象コメントが描画されたら1回だけスクロールし、数秒後に強調を消す
  // （scrolledToHighlightRefで、以後のリアルタイム新着による再レンダーでは
  // 再スクロールしないようにする）。
  useEffect(() => {
    if (!highlightId || scrolledToHighlightRef.current) return;
    const el = document.getElementById(`comment-${highlightId}`);
    if (!el) return;
    scrolledToHighlightRef.current = true;
    el.scrollIntoView({ block: "center" });
    const timer = setTimeout(() => setHighlightId(null), 2500);
    return () => clearTimeout(timer);
  }, [highlightId, comments]);

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

  async function handleLoadOlder() {
    if (comments.length === 0 || loadingOlder) return;
    setLoadingOlder(true);
    try {
      const oldestId = comments[0].id;
      const res = await apiFetch<ListResponse>(
        `/tasks/${taskId}/comments?before=${encodeURIComponent(oldestId)}&limit=${PAGE_SIZE}`,
      );
      setComments((prev) => [...res.items, ...prev]);
      setHasMoreOlder(res.has_more_older);
    } catch {
      // 失敗時はhasMoreOlderをtrueのまま残し、再度ボタンから試せるようにする。
    } finally {
      setLoadingOlder(false);
    }
  }

  async function handleLoadNewer() {
    if (comments.length === 0 || loadingNewer) return;
    setLoadingNewer(true);
    try {
      const newestId = comments[comments.length - 1].id;
      const res = await apiFetch<ListResponse>(
        `/tasks/${taskId}/comments?after=${encodeURIComponent(newestId)}&limit=${PAGE_SIZE}`,
      );
      setComments((prev) => [...prev, ...res.items]);
      setHasMoreNewer(res.has_more_newer);
    } catch {
      // 失敗時はhasMoreNewerをtrueのまま残し、再度ボタンから試せるようにする。
    } finally {
      setLoadingNewer(false);
    }
  }

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

  function selectMention(m: MemberSummary) {
    setBody((prev) => prev + m.name + " ");
    setMentionedIds((prev) => (prev.includes(m.user_id) ? prev : [...prev, m.user_id]));
    setShowMentions(false);
  }

  return (
    <div className="flex flex-col gap-3 border-t border-border pt-4">
      <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">コメント</p>
      <ul className="flex max-h-[32rem] flex-col gap-3 overflow-y-auto">
        {hasMoreOlder && (
          <li className="flex justify-center">
            <button
              type="button"
              onClick={handleLoadOlder}
              disabled={loadingOlder}
              className="flex items-center gap-1.5 text-xs text-indigo-600 hover:underline disabled:opacity-50 dark:text-indigo-400"
            >
              {loadingOlder && <Loader2 className="h-3 w-3 animate-spin" />}
              古いコメントを読み込む
            </button>
          </li>
        )}
        {comments.map((c) => (
          <li
            key={c.id}
            id={`comment-${c.id}`}
            className={clsx(
              "flex items-start gap-2 rounded-lg transition-colors",
              highlightId === c.id && "ring-2 ring-indigo-400 dark:ring-indigo-500",
            )}
          >
            <Avatar name={c.user_name ?? "?"} seed={c.user_id} size="sm" />
            <div className="min-w-0 flex-1 rounded-lg bg-surface-muted px-3 py-2">
              <div className="flex items-baseline gap-2">
                <span className="text-xs font-medium text-foreground">{c.user_name ?? "?"}</span>
                <span className="text-[11px] text-muted-foreground">{new Date(c.created_at).toLocaleString()}</span>
              </div>
              <div className="text-sm text-foreground">{renderMarkdown(c.body)}</div>
            </div>
          </li>
        ))}
        {hasMoreNewer && (
          <li className="flex justify-center">
            <button
              type="button"
              onClick={handleLoadNewer}
              disabled={loadingNewer}
              className="flex items-center gap-1.5 text-xs text-indigo-600 hover:underline disabled:opacity-50 dark:text-indigo-400"
            >
              {loadingNewer && <Loader2 className="h-3 w-3 animate-spin" />}
              新しいコメントを読み込む
            </button>
          </li>
        )}
      </ul>
      <form onSubmit={handleSubmit} className="relative flex flex-col gap-2">
        <div>
          <MarkdownToolbar textareaRef={composerRef} value={body} onChange={handleBodyChange} />
          <Textarea
            ref={composerRef}
            value={body}
            onChange={(e) => handleBodyChange(e.target.value)}
            placeholder="コメントを入力（@でメンション）"
            rows={2}
            className="text-sm"
          />
        </div>
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
