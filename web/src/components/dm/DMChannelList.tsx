"use client";

import { Plus, Users } from "lucide-react";
import clsx from "clsx";

import Avatar from "@/components/ui/Avatar";

export type DMChannel = {
  id: string;
  is_group: boolean;
  name: string | null;
  display_name: string;
  members: { user_id: string; name: string; avatar_url: string | null }[];
  last_message: { body: string; sender_name: string; created_at: string } | null;
  unread: boolean;
  updated_at: string;
};

type Props = {
  channels: DMChannel[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  onNewDM: () => void;
};

// チャンネル一覧（左ペイン、M8.5）。LINE風に未読は太字＋インジケータドットで表す。
export default function DMChannelList({ channels, selectedId, onSelect, onNewDM }: Props) {
  return (
    <div className="flex h-full w-full flex-col border-r border-border">
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
        <h2 className="text-sm font-semibold text-foreground">DM</h2>
        <button
          type="button"
          onClick={onNewDM}
          className="flex items-center gap-1 rounded-lg bg-indigo-600 px-2.5 py-1.5 text-xs font-medium text-white transition-colors hover:bg-indigo-500 dark:bg-indigo-500 dark:hover:bg-indigo-400"
        >
          <Plus className="h-3.5 w-3.5" />
          新規
        </button>
      </div>
      <ul className="flex-1 overflow-y-auto">
        {channels.length === 0 && (
          <li className="px-4 py-8 text-center text-sm text-muted-foreground">
            まだDMがありません。「新規」からメンバーを選んで始めましょう。
          </li>
        )}
        {channels.map((c) => (
          <li key={c.id}>
            <button
              type="button"
              onClick={() => onSelect(c.id)}
              className={clsx(
                "flex w-full items-center gap-2.5 border-b border-border/60 px-4 py-3 text-left transition-colors",
                selectedId === c.id ? "bg-indigo-50 dark:bg-indigo-500/10" : "hover:bg-surface-muted",
              )}
            >
              {c.is_group ? (
                <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-surface-muted text-muted-foreground">
                  <Users className="h-4 w-4" />
                </span>
              ) : (
                <Avatar name={c.display_name} seed={c.id} />
              )}
              <span className="min-w-0 flex-1">
                <span
                  className={clsx(
                    "block truncate text-sm",
                    c.unread ? "font-semibold text-foreground" : "text-foreground",
                  )}
                >
                  {c.display_name}
                </span>
                <span className="block truncate text-xs text-muted-foreground">
                  {c.last_message ? c.last_message.body || "（添付ファイル）" : "まだメッセージはありません"}
                </span>
              </span>
              {c.unread && <span className="h-2 w-2 shrink-0 rounded-full bg-indigo-500" />}
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
