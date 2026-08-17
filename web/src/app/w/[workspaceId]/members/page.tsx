"use client";

import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";

import { apiFetch, ApiError } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";

type Member = {
  id: string;
  user_id: string;
  name: string;
  email: string;
  avatar_url: string | null;
  role: "owner" | "member";
};

export default function MembersPage() {
  const { user } = useAuth();
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;

  const [members, setMembers] = useState<Member[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteStatus, setInviteStatus] = useState<string | null>(null);
  const [inviting, setInviting] = useState(false);

  const load = useCallback(() => {
    if (!workspaceId) return;
    apiFetch<Member[]>(`/workspaces/${workspaceId}/members`)
      .then(setMembers)
      .catch(() => setError("メンバー一覧の取得に失敗しました"))
      .finally(() => setLoading(false));
  }, [workspaceId]);

  useEffect(() => {
    load();
  }, [load]);

  const me = members.find((m) => m.user_id === user?.id);
  const isOwner = me?.role === "owner";

  async function handleInvite(e: React.FormEvent) {
    e.preventDefault();
    if (!inviteEmail.trim()) return;
    setInviting(true);
    setInviteStatus(null);
    try {
      await apiFetch(`/workspaces/${workspaceId}/members/invite`, {
        method: "POST",
        body: JSON.stringify({ email: inviteEmail.trim() }),
      });
      setInviteStatus("招待しました");
      setInviteEmail("");
      load();
    } catch (err) {
      if (err instanceof ApiError && err.body && typeof err.body === "object" && "error" in err.body) {
        const code = (err.body as { error: string }).error;
        if (code === "already_member") setInviteStatus("既にメンバーです");
        else if (code === "already_invited") setInviteStatus("既に招待済みです");
        else setInviteStatus("招待に失敗しました");
      } else {
        setInviteStatus("招待に失敗しました");
      }
    } finally {
      setInviting(false);
    }
  }

  async function handleRoleChange(memberId: string, role: "owner" | "member") {
    setError(null);
    try {
      await apiFetch(`/workspaces/${workspaceId}/members/${memberId}`, {
        method: "PATCH",
        body: JSON.stringify({ role }),
      });
      load();
    } catch (err) {
      if (err instanceof ApiError && err.body && typeof err.body === "object" && "error" in err.body) {
        const code = (err.body as { error: string }).error;
        setError(code === "last_owner" ? "最後の1人のOwnerは降格できません" : "ロール変更に失敗しました");
      } else {
        setError("ロール変更に失敗しました");
      }
    }
  }

  async function handleRemove(memberId: string) {
    if (!window.confirm("このメンバーを削除しますか？")) return;
    setError(null);
    try {
      await apiFetch(`/workspaces/${workspaceId}/members/${memberId}`, { method: "DELETE" });
      load();
    } catch (err) {
      if (err instanceof ApiError && err.body && typeof err.body === "object" && "error" in err.body) {
        const code = (err.body as { error: string }).error;
        setError(code === "last_owner" ? "最後の1人のOwnerは削除できません" : "削除に失敗しました");
      } else {
        setError("削除に失敗しました");
      }
    }
  }

  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-8 px-4 py-10">
      <h1 className="text-xl font-semibold">メンバー</h1>

      {error && <p className="text-sm text-red-600">{error}</p>}

      {loading ? (
        <p className="text-sm text-zinc-500">読み込み中...</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {members.map((m) => (
            <li
              key={m.id}
              className="flex items-center justify-between rounded-md border border-zinc-200 px-4 py-3 dark:border-zinc-800"
            >
              <div className="flex flex-col">
                <span className="text-sm font-medium">{m.name}</span>
                <span className="text-xs text-zinc-500">{m.email}</span>
              </div>
              {isOwner ? (
                <div className="flex items-center gap-3">
                  <select
                    value={m.role}
                    onChange={(e) => handleRoleChange(m.id, e.target.value as "owner" | "member")}
                    className="rounded-md border border-zinc-300 px-2 py-1 text-sm dark:border-zinc-700 dark:bg-zinc-900"
                  >
                    <option value="owner">Owner</option>
                    <option value="member">Member</option>
                  </select>
                  <button
                    onClick={() => handleRemove(m.id)}
                    className="text-xs text-red-600 hover:underline"
                  >
                    削除
                  </button>
                </div>
              ) : (
                <span className="text-xs text-zinc-500">{m.role === "owner" ? "Owner" : "Member"}</span>
              )}
            </li>
          ))}
        </ul>
      )}

      {isOwner && (
        <section>
          <h2 className="mb-2 text-sm font-medium text-zinc-600 dark:text-zinc-400">メンバーを招待</h2>
          <form onSubmit={handleInvite} className="flex gap-2">
            <input
              type="email"
              value={inviteEmail}
              onChange={(e) => setInviteEmail(e.target.value)}
              placeholder="メールアドレス"
              className="flex-1 rounded-md border border-zinc-300 px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
            />
            <button
              type="submit"
              disabled={inviting}
              className="rounded-md bg-black px-4 py-2 text-sm font-medium text-white disabled:opacity-50 dark:bg-white dark:text-black"
            >
              招待
            </button>
          </form>
          {inviteStatus && <p className="mt-2 text-sm text-zinc-500">{inviteStatus}</p>}
        </section>
      )}
    </div>
  );
}
