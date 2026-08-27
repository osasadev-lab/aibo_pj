"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { SmilePlus } from "lucide-react";
import clsx from "clsx";

export type Reaction = { id: string; emoji: string; user_id: string; user_name?: string };

// 定番セット方式（ユーザー確認済み。フル絵文字ピッカーではなくこの6種のみ）。
export const REACTION_EMOJIS = ["👍", "❤️", "😂", "😮", "😢", "🎉"];

type Props = {
  reactions?: Reaction[];
  currentUserId?: string;
  onToggle: (emoji: string) => void;
  className?: string;
};

// 説明欄・コメント・DMメッセージ共通のリアクションUI（2026-08-28追加）。
// 絵文字ごとにグループ化して人数バッジを表示し、hoverで誰が押したか（名前一覧）を
// tooltip表示する。同じ絵文字を自分がクリック済みなら強調表示し、再クリックで
// トグルOFF（onToggleは常にトグル、追加/解除の判定はAPI側で行う）。
export default function ReactionBar({ reactions = [], currentUserId, onToggle, className }: Props) {
  const [pickerOpen, setPickerOpen] = useState(false);
  // DMの自分側メッセージ等、要素が画面右端に近い場合はピッカーが右にはみ出す
  // （横スクロールバーが出てしまう不具合があった）ため、開く直前に空きスペースを
  // 見て左右どちらに展開するか決める。
  const [alignRight, setAlignRight] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  function togglePicker() {
    if (!pickerOpen && rootRef.current) {
      const rect = rootRef.current.getBoundingClientRect();
      setAlignRight(window.innerWidth - rect.left < 260);
    }
    setPickerOpen((v) => !v);
  }

  useEffect(() => {
    if (!pickerOpen) return;
    function handleClick(e: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setPickerOpen(false);
      }
    }
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") setPickerOpen(false);
    }
    window.addEventListener("mousedown", handleClick);
    window.addEventListener("keydown", handleKey);
    return () => {
      window.removeEventListener("mousedown", handleClick);
      window.removeEventListener("keydown", handleKey);
    };
  }, [pickerOpen]);

  const grouped = useMemo(() => {
    const map = new Map<string, Reaction[]>();
    for (const r of reactions) {
      const list = map.get(r.emoji) ?? [];
      list.push(r);
      map.set(r.emoji, list);
    }
    return Array.from(map.entries());
  }, [reactions]);

  return (
    <div className={clsx("relative flex flex-wrap items-center gap-1", className)} ref={rootRef}>
      {grouped.map(([emoji, list]) => {
        const mine = list.some((r) => r.user_id === currentUserId);
        const names = list.map((r) => r.user_name ?? "?").join("、");
        return (
          <button
            key={emoji}
            type="button"
            title={names}
            onClick={() => onToggle(emoji)}
            className={clsx(
              "flex items-center gap-1 rounded-full border px-1.5 py-0.5 text-xs transition-colors",
              mine
                ? "border-indigo-400 bg-indigo-50 text-indigo-700 dark:border-indigo-500 dark:bg-indigo-950/40 dark:text-indigo-300"
                : "border-border bg-surface-muted text-foreground hover:bg-surface",
            )}
          >
            <span>{emoji}</span>
            <span>{list.length}</span>
          </button>
        );
      })}
      <button
        type="button"
        onClick={togglePicker}
        className="flex h-5 items-center justify-center rounded-full border border-border bg-surface-muted px-1.5 text-muted-foreground transition-colors hover:bg-surface hover:text-foreground"
        title="リアクションを追加"
      >
        <SmilePlus className="h-3 w-3" />
      </button>
      {pickerOpen && (
        <div
          className={clsx(
            "absolute bottom-full z-10 mb-1 flex gap-1 rounded-lg border border-border bg-surface p-1 shadow-lg",
            alignRight ? "right-0" : "left-0",
          )}
        >
          {REACTION_EMOJIS.map((emoji) => (
            <button
              key={emoji}
              type="button"
              onClick={() => {
                onToggle(emoji);
                setPickerOpen(false);
              }}
              className="rounded px-1.5 py-1 text-base leading-none hover:bg-surface-muted"
            >
              {emoji}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
