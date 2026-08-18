"use client";

import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { Crown, Mail, Trash2, UserPlus } from "lucide-react";

import { apiFetch, ApiError } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";
import Avatar from "@/components/ui/Avatar";
import Badge from "@/components/ui/Badge";
import Button from "@/components/ui/Button";
import IconButton from "@/components/ui/IconButton";
import SidePanel from "@/components/ui/SidePanel";
import { Input, Select } from "@/components/ui/fields";
import type { WorkspaceMember } from "@/lib/types";

// ワークスペースメンバー一覧の右サイドバーパネル。左サイドバー「メンバー」から開く
// （現在のページを維持したまま重ねて表示するため、/members ルートへは遷移しない）。
// /w/[workspaceId]/members への直接アクセス用にpage.tsxからも使う。
export default function MembersPanel({ onClose }: { onClose: () => void }) {
  const { user } = useAuth();
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;

  const [members, setMembers] = useState<WorkspaceMember[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteStatus, setInviteStatus] = useState<string | null>(null);
  const [inviting, setInviting] = useState(false);

  const load = useCallback(() => {
    if (!workspaceId) return;
    apiFetch<WorkspaceMember[]>(`/workspaces/${workspaceId}/members`)
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
    <SidePanel title="メンバー" onClose={onClose}>
      <div className="flex flex-col gap-8">
        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

        {loading ? (
          <p className="text-sm text-muted-foreground">読み込み中...</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {members.map((m) => (
              <li
                key={m.id}
                className="flex items-center justify-between gap-3 rounded-xl border border-border bg-surface px-4 py-3"
              >
                <div className="flex min-w-0 items-center gap-3">
                  <Avatar name={m.name} seed={m.user_id} />
                  <div className="min-w-0">
                    <span className="flex items-center gap-1.5 truncate text-sm font-medium text-foreground">
                      {m.name}
                      {m.role === "owner" && <Crown className="h-3.5 w-3.5 shrink-0 text-amber-500" />}
                    </span>
                    <span className="block truncate text-xs text-muted-foreground">{m.email}</span>
                  </div>
                </div>
                {isOwner ? (
                  <div className="flex shrink-0 items-center gap-2">
                    <Select
                      value={m.role}
                      onChange={(e) => handleRoleChange(m.id, e.target.value as "owner" | "member")}
                      className="w-auto py-1 text-xs"
                    >
                      <option value="owner">Owner</option>
                      <option value="member">Member</option>
                    </Select>
                    <IconButton size="sm" onClick={() => handleRemove(m.id)} title="削除">
                      <Trash2 className="h-3.5 w-3.5" />
                    </IconButton>
                  </div>
                ) : (
                  <Badge tone={m.role === "owner" ? "amber" : "zinc"}>{m.role === "owner" ? "Owner" : "Member"}</Badge>
                )}
              </li>
            ))}
          </ul>
        )}

        {isOwner && (
          <section className="rounded-xl border border-border bg-surface p-4">
            <h2 className="mb-3 flex items-center gap-1.5 text-sm font-medium text-foreground">
              <UserPlus className="h-4 w-4" />
              メンバーを招待
            </h2>
            <form onSubmit={handleInvite} className="flex gap-2">
              <div className="relative flex-1">
                <Mail className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  type="email"
                  value={inviteEmail}
                  onChange={(e) => setInviteEmail(e.target.value)}
                  placeholder="メールアドレス"
                  className="pl-9"
                />
              </div>
              <Button type="submit" variant="primary" disabled={inviting}>
                招待
              </Button>
            </form>
            {inviteStatus && <p className="mt-2 text-sm text-muted-foreground">{inviteStatus}</p>}
          </section>
        )}
      </div>
    </SidePanel>
  );
}
