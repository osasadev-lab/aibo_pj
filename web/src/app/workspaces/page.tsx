"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { apiFetch } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";

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
    return <div className="flex min-h-screen items-center justify-center text-zinc-500">読み込み中...</div>;
  }

  return (
    <div className="mx-auto flex min-h-screen max-w-2xl flex-col gap-8 px-4 py-10">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">ワークスペース</h1>
        <div className="flex items-center gap-3 text-sm text-zinc-500">
          <span>{user.name}</span>
          <button onClick={logout} className="underline hover:text-zinc-800 dark:hover:text-zinc-200">
            ログアウト
          </button>
        </div>
      </header>

      {error && <p className="text-sm text-red-600">{error}</p>}

      <section className="flex flex-col gap-2">
        {listLoading ? (
          <p className="text-sm text-zinc-500">読み込み中...</p>
        ) : workspaces.length === 0 ? (
          <p className="text-sm text-zinc-500">所属しているワークスペースはまだありません。</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {workspaces.map((ws) => (
              <li key={ws.id}>
                <Link
                  href={`/w/${ws.id}/members`}
                  className="flex items-center justify-between rounded-md border border-zinc-200 px-4 py-3 hover:bg-zinc-50 dark:border-zinc-800 dark:hover:bg-zinc-900"
                >
                  <span>{ws.name}</span>
                  <span className="text-xs text-zinc-500">{ws.role === "owner" ? "Owner" : "Member"}</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section>
        <h2 className="mb-2 text-sm font-medium text-zinc-600 dark:text-zinc-400">新規ワークスペース作成</h2>
        <form onSubmit={handleCreate} className="flex gap-2">
          <input
            type="text"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder="ワークスペース名"
            className="flex-1 rounded-md border border-zinc-300 px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
          />
          <button
            type="submit"
            disabled={creating}
            className="rounded-md bg-black px-4 py-2 text-sm font-medium text-white disabled:opacity-50 dark:bg-white dark:text-black"
          >
            作成
          </button>
        </form>
      </section>
    </div>
  );
}
