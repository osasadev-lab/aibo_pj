"use client";

import { useEffect, useState } from "react";
import { Check } from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import Avatar from "@/components/ui/Avatar";

type Member = {
  id: string;
  user_id: string;
  name: string;
  email: string;
};

type Props = {
  workspaceId: string;
  selected: string[];
  onChange: (userIds: string[]) => void;
};

// ワークスペースメンバーのチェックボックス選択UI。
// プロジェクト作成フォーム・参画メンバー編集で共用する。
export default function MemberPicker({ workspaceId, selected, onChange }: Props) {
  const [members, setMembers] = useState<Member[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let ignore = false;
    apiFetch<Member[]>(`/workspaces/${workspaceId}/members`)
      .then((list) => {
        if (!ignore) setMembers(list);
      })
      .finally(() => {
        if (!ignore) setLoading(false);
      });
    return () => {
      ignore = true;
    };
  }, [workspaceId]);

  function toggle(userId: string) {
    if (selected.includes(userId)) {
      onChange(selected.filter((id) => id !== userId));
    } else {
      onChange([...selected, userId]);
    }
  }

  if (loading) {
    return <p className="text-sm text-muted-foreground">読み込み中...</p>;
  }

  return (
    <ul className="flex max-h-52 flex-col gap-0.5 overflow-y-auto rounded-lg border border-border p-1.5">
      {members.map((m) => {
        const active = selected.includes(m.user_id);
        return (
          <li key={m.user_id}>
            <button
              type="button"
              onClick={() => toggle(m.user_id)}
              className={clsx(
                "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors",
                active ? "bg-indigo-50 dark:bg-indigo-500/10" : "hover:bg-surface-muted",
              )}
            >
              <Avatar name={m.name} seed={m.user_id} size="sm" />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-foreground">{m.name}</span>
                <span className="block truncate text-xs text-muted-foreground">{m.email}</span>
              </span>
              {active && <Check className="h-4 w-4 shrink-0 text-indigo-600 dark:text-indigo-400" />}
            </button>
          </li>
        );
      })}
    </ul>
  );
}
