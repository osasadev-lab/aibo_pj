"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";

import { apiFetch } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";
import MemberPicker from "@/components/MemberPicker";

type Project = {
  id: string;
  name: string;
  description: string | null;
  visibility: "public" | "private";
};

export default function ProjectsPage() {
  const { user } = useAuth();
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;

  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [visibility, setVisibility] = useState<"public" | "private">("public");
  const [memberIds, setMemberIds] = useState<string[]>([]);
  const [creating, setCreating] = useState(false);

  const load = useCallback(() => {
    if (!workspaceId) return;
    apiFetch<Project[]>(`/workspaces/${workspaceId}/projects`)
      .then(setProjects)
      .catch(() => setError("プロジェクト一覧の取得に失敗しました"))
      .finally(() => setLoading(false));
  }, [workspaceId]);

  useEffect(() => {
    load();
  }, [load]);

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    setCreating(true);
    setError(null);
    try {
      await apiFetch(`/workspaces/${workspaceId}/projects`, {
        method: "POST",
        body: JSON.stringify({
          name: name.trim(),
          description: description.trim() || undefined,
          visibility,
          member_ids: memberIds,
        }),
      });
      setName("");
      setDescription("");
      setVisibility("public");
      setMemberIds([]);
      load();
    } catch {
      setError("プロジェクトの作成に失敗しました");
    } finally {
      setCreating(false);
    }
  }

  if (!user) return null;

  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-8 px-4 py-10">
      <h1 className="text-xl font-semibold">プロジェクト</h1>

      {error && <p className="text-sm text-red-600">{error}</p>}

      {loading ? (
        <p className="text-sm text-zinc-500">読み込み中...</p>
      ) : projects.length === 0 ? (
        <p className="text-sm text-zinc-500">プロジェクトはまだありません。</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {projects.map((p) => (
            <li key={p.id}>
              <Link
                href={`/w/${workspaceId}/projects/${p.id}`}
                className="flex items-center justify-between rounded-md border border-zinc-200 px-4 py-3 hover:bg-zinc-50 dark:border-zinc-800 dark:hover:bg-zinc-900"
              >
                <span>{p.name}</span>
                <span className="text-xs text-zinc-500">{p.visibility === "private" ? "Private" : "Public"}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}

      <section>
        <h2 className="mb-2 text-sm font-medium text-zinc-600 dark:text-zinc-400">新規プロジェクト作成</h2>
        <form onSubmit={handleCreate} className="flex flex-col gap-3">
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="プロジェクト名"
            className="rounded-md border border-zinc-300 px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
          />
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="説明（任意）"
            className="rounded-md border border-zinc-300 px-3 py-2 text-sm dark:border-zinc-700 dark:bg-zinc-900"
            rows={2}
          />
          <div className="flex items-center gap-4 text-sm">
            <label className="flex items-center gap-1">
              <input
                type="radio"
                checked={visibility === "public"}
                onChange={() => setVisibility("public")}
              />
              Public（ワークスペース全員が閲覧可）
            </label>
            <label className="flex items-center gap-1">
              <input
                type="radio"
                checked={visibility === "private"}
                onChange={() => setVisibility("private")}
              />
              Private（参画メンバーのみ）
            </label>
          </div>
          <div>
            <p className="mb-1 text-sm text-zinc-600 dark:text-zinc-400">参画メンバー</p>
            {workspaceId && (
              <MemberPicker workspaceId={workspaceId} selected={memberIds} onChange={setMemberIds} />
            )}
          </div>
          <button
            type="submit"
            disabled={creating}
            className="self-start rounded-md bg-black px-4 py-2 text-sm font-medium text-white disabled:opacity-50 dark:bg-white dark:text-black"
          >
            作成
          </button>
        </form>
      </section>
    </div>
  );
}
