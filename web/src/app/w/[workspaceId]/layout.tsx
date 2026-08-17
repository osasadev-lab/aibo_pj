"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useEffect, useState, type ReactNode } from "react";

import { apiFetch } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";

type Workspace = {
  id: string;
  name: string;
  role: "owner" | "member";
};

export default function WorkspaceLayout({ children }: { children: ReactNode }) {
  const { user, loading, logout } = useAuth();
  const router = useRouter();
  const params = useParams<{ workspaceId: string }>();

  const [workspace, setWorkspace] = useState<Workspace | null>(null);

  useEffect(() => {
    if (!loading && !user) {
      router.replace("/login");
    }
  }, [loading, user, router]);

  useEffect(() => {
    if (!user || !params.workspaceId) return;
    apiFetch<Workspace>(`/workspaces/${params.workspaceId}`)
      .then(setWorkspace)
      .catch(() => setWorkspace(null));
  }, [user, params.workspaceId]);

  if (loading || !user) {
    return <div className="flex min-h-screen items-center justify-center text-zinc-500">読み込み中...</div>;
  }

  return (
    <div className="flex min-h-screen flex-col">
      <header className="flex items-center justify-between border-b border-zinc-200 px-6 py-3 dark:border-zinc-800">
        <div className="flex items-center gap-4">
          <Link href="/workspaces" className="text-sm text-zinc-500 hover:underline">
            ワークスペース切替
          </Link>
          <span className="font-medium">{workspace?.name ?? "..."}</span>
          <Link href={`/w/${params.workspaceId}/projects`} className="text-sm text-zinc-500 hover:underline">
            プロジェクト
          </Link>
          <Link href={`/w/${params.workspaceId}/members`} className="text-sm text-zinc-500 hover:underline">
            メンバー
          </Link>
        </div>
        <div className="flex items-center gap-3 text-sm text-zinc-500">
          <span>{user.name}</span>
          <button onClick={logout} className="underline hover:text-zinc-800 dark:hover:text-zinc-200">
            ログアウト
          </button>
        </div>
      </header>
      <main className="flex-1">{children}</main>
    </div>
  );
}
