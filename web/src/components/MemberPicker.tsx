"use client";

import { useEffect, useState } from "react";

import { apiFetch } from "@/lib/apiClient";

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
    return <p className="text-sm text-zinc-500">読み込み中...</p>;
  }

  return (
    <ul className="flex max-h-48 flex-col gap-1 overflow-y-auto rounded-md border border-zinc-300 p-2 dark:border-zinc-700">
      {members.map((m) => (
        <li key={m.user_id}>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={selected.includes(m.user_id)}
              onChange={() => toggle(m.user_id)}
            />
            <span>{m.name}</span>
            <span className="text-xs text-zinc-500">{m.email}</span>
          </label>
        </li>
      ))}
    </ul>
  );
}
