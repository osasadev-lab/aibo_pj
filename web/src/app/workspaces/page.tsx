"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { ChevronRight, LogOut, Plus } from "lucide-react";

import { apiFetch } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";
import Avatar from "@/components/ui/Avatar";
import Badge from "@/components/ui/Badge";
import Button from "@/components/ui/Button";
import IconButton from "@/components/ui/IconButton";
import { Input } from "@/components/ui/fields";

type Workspace = {
  id: string;
  name: string;
  role: "owner" | "member";
};

export default function WorkspacesPage() {
  const { user, loading, logout } = useAuth();
  const router = useRouter();

  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [listLoading, setListLoading] = useState(true);
  const [newName, setNewName] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!loading && !user) {
      router.replace("/login");
    }
  }, [loading, user, router]);

  useEffect(() => {
    if (!user) return;
    apiFetch<Workspace[]>("/workspaces")
      .then(setWorkspaces)
      .catch(() => setError("ワークスペース一覧の取得に失敗しました"))
      .finally(() => setListLoading(false));
  }, [user]);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    if (!newName.trim()) return;
    setCreating(true);
    setError(null);
    try {
      const ws = await apiFetch<Workspace>("/workspaces", {
        method: "POST",
        body: JSON.stringify({ name: newName.trim() }),
      });
      setWorkspaces((prev) => [...prev, ws]);
      setNewName("");
    } catch {
      setError("ワークスペースの作成に失敗しました");
    } finally {
      setCreating(false);
    }
  }

  if (loading || !user) {
    return <div className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">読み込み中...</div>;
  }

  return (
    <div className="mx-auto flex min-h-screen max-w-2xl flex-col gap-8 px-6 py-10">
      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight text-foreground">ワークスペース</h1>
        <div className="flex items-center gap-2">
          <Avatar name={user.name} seed={user.id} size="sm" />
          <span className="text-sm text-muted-foreground">{user.name}</span>
          <IconButton size="sm" onClick={logout} title="ログアウト">
            <LogOut className="h-4 w-4" />
          </IconButton>
        </div>
      </header>

      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

      <section className="flex flex-col gap-2">
        {listLoading ? (
          <p className="text-sm text-muted-foreground">読み込み中...</p>
        ) : workspaces.length === 0 ? (
          <p className="text-sm text-muted-foreground">所属しているワークスペースはまだありません。</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {workspaces.map((ws) => (
              <li key={ws.id}>
                <Link
                  href={`/w/${ws.id}/projects`}
                  className="group flex items-center gap-3 rounded-xl border border-border bg-surface px-4 py-3.5 transition-colors hover:border-indigo-300 hover:bg-surface-muted dark:hover:border-indigo-500/50"
                >
                  <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-indigo-600 text-sm font-semibold text-white">
                    {ws.name.slice(0, 1)}
                  </span>
                  <span className="min-w-0 flex-1 truncate font-medium text-foreground">{ws.name}</span>
                  <Badge tone={ws.role === "owner" ? "amber" : "zinc"}>{ws.role === "owner" ? "Owner" : "Member"}</Badge>
                  <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground/50 transition-transform group-hover:translate-x-0.5" />
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="rounded-xl border border-border bg-surface p-4">
        <h2 className="mb-3 flex items-center gap-1.5 text-sm font-medium text-foreground">
          <Plus className="h-4 w-4" />
          新規ワークスペース作成
        </h2>
        <form onSubmit={handleCreate} className="flex gap-2">
          <Input
            type="text"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="ワークスペース名"
          />
          <Button type="submit" variant="primary" disabled={creating}>
            作成
          </Button>
        </form>
      </section>
    </div>
  );
}
