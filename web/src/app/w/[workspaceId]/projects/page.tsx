"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { ChevronRight, FolderPlus, Globe, Lock } from "lucide-react";

import { apiFetch } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";
import MemberPicker from "@/components/MemberPicker";
import Badge from "@/components/ui/Badge";
import Button from "@/components/ui/Button";
import { Input, Textarea } from "@/components/ui/fields";

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
    <div className="mx-auto flex max-w-2xl flex-col gap-8 px-6 py-8">
      <h1 className="text-2xl font-semibold tracking-tight text-foreground">プロジェクト</h1>

      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

      {loading ? (
        <p className="text-sm text-muted-foreground">読み込み中...</p>
      ) : projects.length === 0 ? (
        <p className="text-sm text-muted-foreground">プロジェクトはまだありません。</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {projects.map((p) => (
            <li key={p.id}>
              <Link
                href={`/w/${workspaceId}/projects/${p.id}`}
                className="group flex items-center justify-between gap-3 rounded-xl border border-border bg-surface px-4 py-3.5 transition-colors hover:border-indigo-300 hover:bg-surface-muted dark:hover:border-indigo-500/50"
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium text-foreground">{p.name}</span>
                  {p.description && (
                    <span className="block truncate text-xs text-muted-foreground">{p.description}</span>
                  )}
                </span>
                <Badge tone={p.visibility === "private" ? "amber" : "green"}>
                  {p.visibility === "private" ? (
                    <Lock className="h-3 w-3" />
                  ) : (
                    <Globe className="h-3 w-3" />
                  )}
                  {p.visibility === "private" ? "Private" : "Public"}
                </Badge>
                <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground/50 transition-transform group-hover:translate-x-0.5" />
              </Link>
            </li>
          ))}
        </ul>
      )}

      <section className="rounded-xl border border-border bg-surface p-4">
        <h2 className="mb-3 flex items-center gap-1.5 text-sm font-medium text-foreground">
          <FolderPlus className="h-4 w-4" />
          新規プロジェクト作成
        </h2>
        <form onSubmit={handleCreate} className="flex flex-col gap-3">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="プロジェクト名" />
          <Textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="説明（任意）"
            rows={2}
          />
          <div className="flex items-center gap-4 text-sm text-foreground">
            <label className="flex items-center gap-1.5">
              <input
                type="radio"
                checked={visibility === "public"}
                onChange={() => setVisibility("public")}
                className="accent-indigo-600"
              />
              Public（ワークスペース全員が閲覧可）
            </label>
            <label className="flex items-center gap-1.5">
              <input
                type="radio"
                checked={visibility === "private"}
                onChange={() => setVisibility("private")}
                className="accent-indigo-600"
              />
              Private（参画メンバーのみ）
            </label>
          </div>
          <div>
            <p className="mb-1.5 text-sm text-muted-foreground">参画メンバー</p>
            {workspaceId && (
              <MemberPicker workspaceId={workspaceId} selected={memberIds} onChange={setMemberIds} />
            )}
          </div>
          <Button type="submit" variant="primary" disabled={creating} className="self-start">
            作成
          </Button>
        </form>
      </section>
    </div>
  );
}
