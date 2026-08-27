"use client";

import { useEffect, useRef, useState } from "react";
import { ArrowLeft, Check, LogOut, Paperclip, Pencil, Send, Trash2, UserPlus, Users, X } from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import { getSupabaseClient } from "@/lib/supabaseClient";
import { renderMarkdown } from "@/lib/markdown";
import { useAuth } from "@/lib/auth/useAuth";
import Avatar from "@/components/ui/Avatar";
import IconButton from "@/components/ui/IconButton";
import MarkdownToolbar from "@/components/ui/MarkdownToolbar";
import MemberPicker from "@/components/MemberPicker";
import ReactionBar, { type Reaction } from "@/components/ui/ReactionBar";
import { Input, Textarea } from "@/components/ui/fields";

type DMAttachment = { id: string; file_name: string; size_bytes: number; content_type: string };

type DMMessage = {
  id: string;
  channel_id: string;
  user_id: string;
  body: string | null;
  user_name?: string;
  created_at: string;
  updated_at: string;
  attachments: DMAttachment[];
  reactions: Reaction[];
};

type ChannelDetail = {
  id: string;
  is_group: boolean;
  name: string | null;
  display_name: string;
  members: { user_id: string; name: string; avatar_url: string | null }[];
};

const MAX_ATTACHMENT_BYTES = 25 * 1024 * 1024;

function formatFileSize(bytes: number): string {
  if (bytes < 1024 * 1024) return `${Math.ceil(bytes / 1024)}KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
}

type Props = {
  workspaceId: string;
  channelId: string;
  onLeft: () => void;
  onChannelUpdated: () => void;
  // lg未満（一覧とスレッドを同時に表示できない幅）でのみ表示する「一覧に戻る」導線。
  // page.tsxがselectedIdをクリアする形で渡す（M8.5後の追加改修、モバイルUX）。
  onBack: () => void;
};

// メッセージスレッド（右ペイン、M8.5）。自分のメッセージは右寄せ・紫系
// （テーマの indigo）、相手のメッセージは左寄せ・surface-muted（LINE風UI）。
// 初回一覧はREST、以降の新着はSupabase Realtime購読（commentsと同じ方式）。
export default function DMThread({ workspaceId, channelId, onLeft, onChannelUpdated, onBack }: Props) {
  const { user } = useAuth();
  const storageEnabled = user?.storage_enabled ?? false;
  const [channel, setChannel] = useState<ChannelDetail | null>(null);
  const [messages, setMessages] = useState<DMMessage[]>([]);
  const [hasMoreOlder, setHasMoreOlder] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [body, setBody] = useState("");
  const [pendingFiles, setPendingFiles] = useState<File[]>([]);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [renaming, setRenaming] = useState(false);
  const [nameDraft, setNameDraft] = useState("");
  const [showAddMembers, setShowAddMembers] = useState(false);
  const [addMemberIds, setAddMemberIds] = useState<string[]>([]);
  const [editingMessageId, setEditingMessageId] = useState<string | null>(null);
  const [editMessageBody, setEditMessageBody] = useState("");
  const composerRef = useRef<HTMLTextAreaElement | null>(null);
  const editComposerRef = useRef<HTMLTextAreaElement | null>(null);
  const bottomRef = useRef<HTMLDivElement | null>(null);

  function loadChannel() {
    apiFetch<ChannelDetail>(`/dm/channels/${channelId}`)
      .then(setChannel)
      .catch(() => setError("チャンネルの取得に失敗しました"));
  }

  function loadMessages() {
    return apiFetch<{ items: DMMessage[]; has_more_older: boolean }>(`/dm/channels/${channelId}/messages`)
      .then((res) => {
        setMessages(res.items);
        setHasMoreOlder(res.has_more_older);
      })
      .catch(() => setError("メッセージの取得に失敗しました"));
  }

  // 呼び出し元（page.tsx）がkey={channelId}でマウントするため、channelId切替のたびに
  // このコンポーネント自体が作り直される＝channel/messages/errorは常に初期値から
  // 始まる。そのため明示的なリセットは不要（setState-in-effectのカスケード
  // 再レンダリングも避けられる）。
  useEffect(() => {
    loadChannel();
    loadMessages().then(() => {
      requestAnimationFrame(() => bottomRef.current?.scrollIntoView());
    });
    apiFetch(`/dm/channels/${channelId}/read`, { method: "PATCH" })
      .then(onChannelUpdated)
      .catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelId]);

  useEffect(() => {
    const supabase = getSupabaseClient();
    const channelSub = supabase
      .channel(`dm_messages:${channelId}`)
      .on(
        "postgres_changes",
        { event: "INSERT", schema: "public", table: "dm_messages", filter: `channel_id=eq.${channelId}` },
        (payload) => {
          const row = payload.new as DMMessage;
          const author = channel?.members.find((m) => m.user_id === row.user_id);
          const enriched = {
            ...row,
            user_name: author?.name,
            attachments: row.attachments ?? [],
            reactions: row.reactions ?? [],
          };
          setMessages((prev) => (prev.some((m) => m.id === row.id) ? prev : [...prev, enriched]));
          requestAnimationFrame(() => bottomRef.current?.scrollIntoView({ behavior: "smooth" }));
          apiFetch(`/dm/channels/${channelId}/read`, { method: "PATCH" })
            .then(onChannelUpdated)
            .catch(() => {});
        },
      )
      .on(
        "postgres_changes",
        { event: "UPDATE", schema: "public", table: "dm_messages", filter: `channel_id=eq.${channelId}` },
        (payload) => {
          const row = payload.new as DMMessage;
          setMessages((prev) =>
            prev.map((m) => (m.id === row.id ? { ...m, body: row.body, updated_at: row.updated_at } : m)),
          );
        },
      )
      .on(
        "postgres_changes",
        { event: "DELETE", schema: "public", table: "dm_messages", filter: `channel_id=eq.${channelId}` },
        (payload) => {
          const row = payload.old as Partial<DMMessage>;
          if (!row.id) return;
          setMessages((prev) => prev.filter((m) => m.id !== row.id));
        },
      )
      .subscribe();
    return () => {
      void supabase.removeChannel(channelSub);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [channelId, channel?.members]);

  // リアクション用の別チャンネル（2026-08-28追加）。reactionsテーブルはdm_channel_idを
  // 非正規化して持たせてあるので、チャンネル単位で一括購読する。
  useEffect(() => {
    const supabase = getSupabaseClient();
    const channelSub = supabase
      .channel(`reactions:dm:${channelId}`)
      .on(
        "postgres_changes",
        { event: "INSERT", schema: "public", table: "reactions", filter: `dm_channel_id=eq.${channelId}` },
        (payload) => {
          const row = payload.new as { id: string; target_type: string; target_id: string; user_id: string; emoji: string };
          if (row.target_type !== "dm_message") return;
          const author = channel?.members.find((m) => m.user_id === row.user_id);
          const newReaction: Reaction = { id: row.id, emoji: row.emoji, user_id: row.user_id, user_name: author?.name };
          setMessages((prev) =>
            prev.map((m) =>
              m.id === row.target_id && !m.reactions.some((r) => r.id === newReaction.id)
                ? { ...m, reactions: [...m.reactions, newReaction] }
                : m,
            ),
          );
        },
      )
      .on(
        "postgres_changes",
        { event: "DELETE", schema: "public", table: "reactions", filter: `dm_channel_id=eq.${channelId}` },
        (payload) => {
          // DELETEイベントのpayload.oldはREPLICA IDENTITY FULLでも主キー（id）しか
          // 含まれない。dm_channel_id列はdm_messageへのリアクションにしか設定され
          // ないため、この購読フィルタ自体が対象を一意に絞り込んでおり、
          // target_type等のチェックは不要（全メッセージのreactions配列を探して
          // 該当idを取り除く）。
          const row = payload.old as { id?: string };
          if (!row.id) return;
          setMessages((prev) =>
            prev.map((m) =>
              m.reactions.some((r) => r.id === row.id)
                ? { ...m, reactions: m.reactions.filter((r) => r.id !== row.id) }
                : m,
            ),
          );
        },
      )
      .subscribe();
    return () => {
      void supabase.removeChannel(channelSub);
    };
  }, [channelId, channel?.members]);

  async function handleToggleReaction(messageId: string, emoji: string) {
    try {
      await apiFetch(`/dm/channels/${channelId}/messages/${messageId}/reactions`, {
        method: "POST",
        body: JSON.stringify({ emoji }),
      });
    } catch {
      // 失敗時は何もしない（Realtimeで反映されなければ再クリックで再試行できる）
    }
  }

  async function handleLoadOlder() {
    if (messages.length === 0 || loadingOlder) return;
    setLoadingOlder(true);
    try {
      const oldestId = messages[0].id;
      const res = await apiFetch<{ items: DMMessage[]; has_more_older: boolean }>(
        `/dm/channels/${channelId}/messages?before=${encodeURIComponent(oldestId)}`,
      );
      setMessages((prev) => [...res.items, ...prev]);
      setHasMoreOlder(res.has_more_older);
    } catch {
      // 失敗時はhasMoreOlderをtrueのまま残し、再試行できるようにする。
    } finally {
      setLoadingOlder(false);
    }
  }

  function addFiles(files: FileList | null) {
    if (!files) return;
    const oversized = Array.from(files).some((f) => f.size > MAX_ATTACHMENT_BYTES);
    if (oversized) {
      setError("ファイルサイズが25MBを超えています");
      return;
    }
    setPendingFiles((prev) => [...prev, ...Array.from(files)]);
  }

  async function handleSend(e: React.FormEvent) {
    e.preventDefault();
    if (!body.trim() && pendingFiles.length === 0) return;
    setSending(true);
    setError(null);
    try {
      const created = await apiFetch<DMMessage>(`/dm/channels/${channelId}/messages`, {
        method: "POST",
        body: JSON.stringify({ body: body.trim() }),
      });
      for (const file of pendingFiles) {
        const att = await apiFetch<DMAttachment & { upload_url: string }>(
          `/dm/messages/${created.id}/attachments`,
          {
            method: "POST",
            body: JSON.stringify({
              file_name: file.name,
              content_type: file.type || "application/octet-stream",
              size_bytes: file.size,
            }),
          },
        );
        const putRes = await fetch(att.upload_url, {
          method: "PUT",
          headers: { "Content-Type": file.type || "application/octet-stream" },
          body: file,
        });
        if (!putRes.ok) throw new Error("upload failed");
      }
      setBody("");
      setPendingFiles([]);
      // Realtime購読でも自分の投稿は届くが、添付付きメッセージ（添付は後追いで
      // 個別作成される）を確実に最新状態で表示するため、この場では明示的に再取得する。
      loadMessages().then(() => requestAnimationFrame(() => bottomRef.current?.scrollIntoView()));
    } catch {
      setError("メッセージの送信に失敗しました");
    } finally {
      setSending(false);
    }
  }

  async function handleRename() {
    if (!nameDraft.trim()) return;
    try {
      await apiFetch(`/dm/channels/${channelId}`, { method: "PATCH", body: JSON.stringify({ name: nameDraft.trim() }) });
      setRenaming(false);
      loadChannel();
      onChannelUpdated();
    } catch {
      setError("グループ名の変更に失敗しました");
    }
  }

  async function handleAddMembers() {
    if (addMemberIds.length === 0) return;
    try {
      await apiFetch(`/dm/channels/${channelId}/members`, {
        method: "POST",
        body: JSON.stringify({ user_ids: addMemberIds }),
      });
      setAddMemberIds([]);
      setShowAddMembers(false);
      loadChannel();
    } catch {
      setError("メンバーの追加に失敗しました");
    }
  }

  async function handleLeave() {
    if (!window.confirm("このグループから退出しますか？")) return;
    try {
      await apiFetch(`/dm/channels/${channelId}/members/me`, { method: "DELETE" });
      onChannelUpdated();
      onLeft();
    } catch {
      setError("退出に失敗しました");
    }
  }

  function startEditMessage(m: DMMessage) {
    setEditingMessageId(m.id);
    setEditMessageBody(m.body ?? "");
  }

  function cancelEditMessage() {
    setEditingMessageId(null);
    setEditMessageBody("");
  }

  async function handleUpdateMessage(messageId: string) {
    if (!editMessageBody.trim()) return;
    try {
      const updated = await apiFetch<DMMessage>(`/dm/channels/${channelId}/messages/${messageId}`, {
        method: "PATCH",
        body: JSON.stringify({ body: editMessageBody.trim() }),
      });
      setMessages((prev) => prev.map((m) => (m.id === messageId ? { ...m, ...updated } : m)));
      cancelEditMessage();
    } catch {
      setError("メッセージの更新に失敗しました");
    }
  }

  async function handleDeleteMessage(messageId: string) {
    if (!window.confirm("このメッセージを削除しますか？")) return;
    try {
      await apiFetch(`/dm/channels/${channelId}/messages/${messageId}`, { method: "DELETE" });
      setMessages((prev) => prev.filter((m) => m.id !== messageId));
    } catch {
      setError("メッセージの削除に失敗しました");
    }
  }

  if (!channel) {
    return <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">読み込み中...</div>;
  }

  return (
    <div className="flex h-full min-w-0 flex-1 flex-col">
      <div className="flex items-center gap-2 border-b border-border px-4 py-3">
        <IconButton size="sm" title="一覧に戻る" onClick={onBack} className="lg:hidden">
          <ArrowLeft className="h-4 w-4" />
        </IconButton>
        {channel.is_group ? (
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-surface-muted text-muted-foreground">
            <Users className="h-4 w-4" />
          </span>
        ) : (
          <Avatar name={channel.display_name} seed={channel.id} />
        )}
        {renaming ? (
          <div className="flex min-w-0 flex-1 items-center gap-1.5">
            <Input value={nameDraft} onChange={(e) => setNameDraft(e.target.value)} className="h-8 text-sm" autoFocus />
            <IconButton size="sm" onClick={handleRename} title="保存">
              <Check className="h-3.5 w-3.5" />
            </IconButton>
            <IconButton size="sm" onClick={() => setRenaming(false)} title="取消">
              <X className="h-3.5 w-3.5" />
            </IconButton>
          </div>
        ) : (
          <span className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">
            {channel.display_name}
            {channel.is_group && <span className="ml-1.5 text-xs text-muted-foreground">（{channel.members.length}人）</span>}
          </span>
        )}
        {channel.is_group && !renaming && (
          <>
            <IconButton
              size="sm"
              title="グループ名を変更"
              onClick={() => {
                setNameDraft(channel.name ?? "");
                setRenaming(true);
              }}
            >
              <Pencil className="h-3.5 w-3.5" />
            </IconButton>
            <IconButton size="sm" title="メンバーを追加" onClick={() => setShowAddMembers((v) => !v)}>
              <UserPlus className="h-3.5 w-3.5" />
            </IconButton>
            <IconButton size="sm" title="退出" onClick={handleLeave}>
              <LogOut className="h-3.5 w-3.5 hover:text-red-600" />
            </IconButton>
          </>
        )}
      </div>

      {showAddMembers && (
        <div className="flex flex-col gap-2 border-b border-border bg-surface-muted/50 px-4 py-3">
          <MemberPicker workspaceId={workspaceId} selected={addMemberIds} onChange={setAddMemberIds} />
          <button
            type="button"
            onClick={handleAddMembers}
            disabled={addMemberIds.length === 0}
            className="self-start rounded-lg bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-indigo-500 disabled:opacity-50 dark:bg-indigo-500"
          >
            追加する
          </button>
        </div>
      )}

      {error && (
        <p className="mx-4 mt-2 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-600 dark:bg-red-950/40 dark:text-red-400">
          {error}
        </p>
      )}

      <div className="flex flex-1 flex-col gap-3 overflow-y-auto px-4 py-4">
        {hasMoreOlder && (
          <button
            type="button"
            onClick={handleLoadOlder}
            disabled={loadingOlder}
            className="self-center text-xs text-indigo-600 hover:underline disabled:opacity-50 dark:text-indigo-400"
          >
            古いメッセージを読み込む
          </button>
        )}
        {messages.map((m) => {
          const isMine = m.user_id === user?.id;
          return (
            <div key={m.id} className={clsx("flex items-end gap-2", isMine && "flex-row-reverse")}>
              {!isMine && <Avatar name={m.user_name ?? "?"} seed={m.user_id} size="sm" />}
              <div
                className={clsx(
                  "flex min-w-0 flex-col gap-1",
                  isMine ? "items-end" : "items-start",
                  editingMessageId === m.id ? "w-full flex-1" : "max-w-[75%]",
                )}
              >
                {!isMine && <span className="text-[11px] text-muted-foreground">{m.user_name ?? "?"}</span>}
                {editingMessageId === m.id ? (
                  <div className="flex w-full flex-col gap-1.5">
                    <div>
                      <MarkdownToolbar
                        textareaRef={editComposerRef}
                        value={editMessageBody}
                        onChange={setEditMessageBody}
                      />
                      <Textarea
                        ref={editComposerRef}
                        value={editMessageBody}
                        onChange={(e) => setEditMessageBody(e.target.value)}
                        rows={3}
                        className="text-sm"
                        autoFocus
                      />
                    </div>
                    <div className="flex items-center gap-2 self-end">
                      <button
                        type="button"
                        onClick={() => handleUpdateMessage(m.id)}
                        className="rounded-lg bg-indigo-600 px-2.5 py-1 text-xs font-medium text-white hover:bg-indigo-500 dark:bg-indigo-500"
                      >
                        保存
                      </button>
                      <button
                        type="button"
                        onClick={cancelEditMessage}
                        className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
                      >
                        <X className="h-3 w-3" />
                        キャンセル
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className={clsx("group flex items-center gap-1.5", isMine && "flex-row-reverse")}>
                    {m.body && (
                      <div
                        className={clsx(
                          "rounded-2xl px-3.5 py-2 text-sm",
                          isMine
                            ? "rounded-br-sm bg-indigo-600 text-white dark:bg-indigo-500"
                            : "rounded-bl-sm bg-surface-muted text-foreground",
                        )}
                      >
                        {renderMarkdown(m.body)}
                      </div>
                    )}
                    {isMine && (
                      <span className="flex shrink-0 items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
                        <button
                          type="button"
                          onClick={() => startEditMessage(m)}
                          className="text-muted-foreground hover:text-foreground"
                          aria-label="編集"
                        >
                          <Pencil className="h-3 w-3" />
                        </button>
                        <button
                          type="button"
                          onClick={() => handleDeleteMessage(m.id)}
                          className="text-muted-foreground hover:text-red-500"
                          aria-label="削除"
                        >
                          <Trash2 className="h-3 w-3" />
                        </button>
                      </span>
                    )}
                  </div>
                )}
                {editingMessageId !== m.id && (
                  <ReactionBar
                    reactions={m.reactions}
                    currentUserId={user?.id}
                    onToggle={(emoji) => handleToggleReaction(m.id, emoji)}
                  />
                )}
                {m.attachments.length > 0 && (
                  <div className="flex flex-col gap-1">
                    {m.attachments.map((a) => (
                      <span
                        key={a.id}
                        className={clsx(
                          "flex items-center gap-1.5 rounded-xl px-3 py-1.5 text-xs",
                          isMine ? "bg-indigo-600/80 text-white dark:bg-indigo-500/80" : "bg-surface-muted text-foreground",
                        )}
                      >
                        <Paperclip className="h-3 w-3 shrink-0" />
                        <span className="truncate">{a.file_name}</span>
                        <span className="shrink-0 opacity-70">{formatFileSize(a.size_bytes)}</span>
                      </span>
                    ))}
                  </div>
                )}
                <span className="text-[10px] text-muted-foreground">
                  {new Date(m.created_at).toLocaleString()}
                  {m.updated_at !== m.created_at && "（編集済み）"}
                </span>
              </div>
            </div>
          );
        })}
        <div ref={bottomRef} />
      </div>

      <form onSubmit={handleSend} className="flex flex-col gap-2 border-t border-border px-4 py-3">
        {pendingFiles.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {pendingFiles.map((f, i) => (
              <span
                key={i}
                className="flex items-center gap-1 rounded-lg bg-surface-muted px-2 py-1 text-xs text-foreground"
              >
                <Paperclip className="h-3 w-3" />
                {f.name}
                <button
                  type="button"
                  onClick={() => setPendingFiles((prev) => prev.filter((_, idx) => idx !== i))}
                  className="text-muted-foreground hover:text-foreground"
                >
                  <X className="h-3 w-3" />
                </button>
              </span>
            ))}
          </div>
        )}
        <div>
          <MarkdownToolbar textareaRef={composerRef} value={body} onChange={setBody} />
          <Textarea
            ref={composerRef}
            value={body}
            onChange={(e) => setBody(e.target.value)}
            placeholder="メッセージを入力..."
            rows={2}
            className="text-sm"
          />
        </div>
        <div className="flex items-center justify-between">
          {/* 添付ファイル機能はタスク詳細（AttachmentSection.tsx）と同じ条件で無効化する
              （R2未設定時、GET /auth/meのstorage_enabledがfalse。M8.5後の追加対応）。 */}
          {storageEnabled ? (
            <label className="flex cursor-pointer items-center gap-1.5 rounded-lg border border-border bg-surface px-2.5 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-surface-muted hover:text-foreground">
              <Paperclip className="h-3.5 w-3.5" />
              添付
              <input type="file" multiple className="hidden" onChange={(e) => addFiles(e.target.files)} />
            </label>
          ) : (
            <span />
          )}
          <button
            type="submit"
            disabled={sending || (!body.trim() && pendingFiles.length === 0)}
            className="flex items-center gap-1.5 rounded-lg bg-indigo-600 px-3.5 py-1.5 text-sm font-medium text-white transition-colors hover:bg-indigo-500 disabled:opacity-50 dark:bg-indigo-500 dark:hover:bg-indigo-400"
          >
            <Send className="h-3.5 w-3.5" />
            送信
          </button>
        </div>
      </form>
    </div>
  );
}
