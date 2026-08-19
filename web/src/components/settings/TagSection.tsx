"use client";

import { useCallback, useEffect, useState } from "react";
import { Check, Pencil, Plus, Trash2, X } from "lucide-react";
import clsx from "clsx";

import { apiFetch, ApiError } from "@/lib/apiClient";
import Badge from "@/components/ui/Badge";
import Button from "@/components/ui/Button";
import IconButton from "@/components/ui/IconButton";
import { Input } from "@/components/ui/fields";
import type { Tag } from "@/lib/types";

const TAG_COLORS = ["zinc", "red", "amber", "green", "indigo"] as const;

const COLOR_DOT: Record<(typeof TAG_COLORS)[number], string> = {
  zinc: "bg-zinc-400",
  red: "bg-red-500",
  amber: "bg-amber-500",
  green: "bg-emerald-500",
  indigo: "bg-indigo-500",
};

type Props = {
  title: string;
  listUrl: string;
  createUrl: string;
  updateUrl: (tagId: string) => string;
  deleteUrl: (tagId: string) => string;
};

// タグ管理区画（共通タグ・プロジェクト専用タグの両方で使い回す）。
// 一覧・作成・編集・削除のCRUD一式を1コンポーネントに閉じ込める。
export default function TagSection({ title, listUrl, createUrl, updateUrl, deleteUrl }: Props) {
  const [tags, setTags] = useState<Tag[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [newName, setNewName] = useState("");
  const [newColor, setNewColor] = useState<(typeof TAG_COLORS)[number]>("zinc");

  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState("");
  const [editColor, setEditColor] = useState<(typeof TAG_COLORS)[number]>("zinc");

  const load = useCallback(() => {
    apiFetch<Tag[]>(listUrl)
      .then(setTags)
      .catch(() => setError("タグ一覧の取得に失敗しました"))
      .finally(() => setLoading(false));
  }, [listUrl]);

  useEffect(() => {
    load();
  }, [load]);

  function describeError(err: unknown, fallback: string): string {
    if (err instanceof ApiError && err.body && typeof err.body === "object" && "error" in err.body) {
      const code = (err.body as { error: string }).error;
      if (code === "duplicate_tag_name") return "同じ名前のタグが既にあります";
    }
    return fallback;
  }

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    if (!newName.trim()) return;
    setError(null);
    try {
      await apiFetch(createUrl, {
        method: "POST",
        body: JSON.stringify({ name: newName.trim(), color: newColor }),
      });
      setNewName("");
      setNewColor("zinc");
      load();
    } catch (err) {
      setError(describeError(err, "タグの作成に失敗しました"));
    }
  }

  function startEdit(tag: Tag) {
    setEditingId(tag.id);
    setEditName(tag.name);
    setEditColor((TAG_COLORS as readonly string[]).includes(tag.color) ? (tag.color as (typeof TAG_COLORS)[number]) : "zinc");
  }

  async function handleUpdate(tagId: string) {
    if (!editName.trim()) return;
    setError(null);
    try {
      await apiFetch(updateUrl(tagId), {
        method: "PATCH",
        body: JSON.stringify({ name: editName.trim(), color: editColor }),
      });
      setEditingId(null);
      load();
    } catch (err) {
      setError(describeError(err, "タグの更新に失敗しました"));
    }
  }

  async function handleDelete(tagId: string) {
    if (!window.confirm("このタグを削除しますか？（付与済みのタスクからも外れます）")) return;
    setError(null);
    try {
      await apiFetch(deleteUrl(tagId), { method: "DELETE" });
      load();
    } catch {
      setError("タグの削除に失敗しました");
    }
  }

  return (
    <section className="rounded-xl border border-border bg-surface p-4">
      <h2 className="mb-3 text-sm font-semibold text-foreground">{title}</h2>
      {error && <p className="mb-2 text-sm text-red-600 dark:text-red-400">{error}</p>}

      {loading ? (
        <p className="text-sm text-muted-foreground">読み込み中...</p>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {tags.map((tag) =>
            editingId === tag.id ? (
              <li key={tag.id} className="flex items-center gap-1.5">
                <div className="flex gap-1">
                  {TAG_COLORS.map((c) => (
                    <button
                      key={c}
                      type="button"
                      onClick={() => setEditColor(c)}
                      className={clsx(
                        "h-5 w-5 rounded-full",
                        COLOR_DOT[c],
                        editColor === c && "ring-2 ring-offset-2 ring-offset-surface ring-indigo-500",
                      )}
                      title={c}
                    />
                  ))}
                </div>
                <Input value={editName} onChange={(e) => setEditName(e.target.value)} className="h-8 py-1 text-sm" />
                <IconButton size="sm" onClick={() => handleUpdate(tag.id)} title="保存">
                  <Check className="h-3.5 w-3.5" />
                </IconButton>
                <IconButton size="sm" onClick={() => setEditingId(null)} title="取消">
                  <X className="h-3.5 w-3.5" />
                </IconButton>
              </li>
            ) : (
              <li
                key={tag.id}
                className="group flex items-center justify-between gap-2 rounded-lg px-1 py-1 hover:bg-surface-muted"
              >
                <Badge tone={tag.color as "zinc" | "red" | "amber" | "green" | "indigo"}>{tag.name}</Badge>
                <div className="flex shrink-0 items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
                  <IconButton size="sm" onClick={() => startEdit(tag)} title="編集">
                    <Pencil className="h-3.5 w-3.5" />
                  </IconButton>
                  <IconButton size="sm" onClick={() => handleDelete(tag.id)} title="削除">
                    <Trash2 className="h-3.5 w-3.5" />
                  </IconButton>
                </div>
              </li>
            ),
          )}
          {tags.length === 0 && <p className="text-sm text-muted-foreground/70">タグはまだありません</p>}
        </ul>
      )}

      <form onSubmit={handleCreate} className="mt-3 flex items-center gap-1.5 border-t border-border pt-3">
        <div className="flex gap-1">
          {TAG_COLORS.map((c) => (
            <button
              key={c}
              type="button"
              onClick={() => setNewColor(c)}
              className={clsx(
                "h-5 w-5 rounded-full",
                COLOR_DOT[c],
                newColor === c && "ring-2 ring-offset-2 ring-offset-surface ring-indigo-500",
              )}
              title={c}
            />
          ))}
        </div>
        <Input
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          placeholder="新しいタグ名"
          className="h-8 py-1 text-sm"
        />
        <Button type="submit" variant="secondary" size="sm">
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </form>
    </section>
  );
}
