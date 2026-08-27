"use client";

import { useState } from "react";
import { Check } from "lucide-react";

import { apiFetch } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";
import Button from "@/components/ui/Button";
import { Input } from "@/components/ui/fields";

// 表示名の編集（個人設定、M8）。他の個人設定と違いサーバー保存（usersテーブル）で、
// 更新後はAuthContextのupdateUserでローカルのuser表示（サイドバーのアバター名等）にも
// 即座に反映する。
export default function ProfileSection() {
  const { user, updateUser } = useAuth();
  const [name, setName] = useState(user?.name ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const dirty = name.trim() !== "" && name.trim() !== user?.name;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed || trimmed === user?.name) return;
    setSaving(true);
    setError(null);
    setSaved(false);
    try {
      const res = await apiFetch<{ name: string }>("/me/profile", {
        method: "PATCH",
        body: JSON.stringify({ name: trimmed }),
      });
      setName(res.name);
      updateUser({ name: res.name });
      setSaved(true);
      setTimeout(() => setSaved(false), 1500);
    } catch {
      setError("表示名の更新に失敗しました");
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="rounded-xl border border-border bg-surface p-4">
      <h2 className="mb-1 text-sm font-semibold text-foreground">表示名</h2>
      <p className="mb-3 text-xs text-muted-foreground">
        ワークスペース内で他のメンバーに表示される名前です。
      </p>
      {error && <p className="mb-2 text-sm text-red-600 dark:text-red-400">{error}</p>}
      <form onSubmit={handleSubmit} className="flex items-center gap-2">
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          maxLength={100}
          className="max-w-sm"
          placeholder="表示名"
        />
        <Button variant="primary" size="sm" type="submit" disabled={!dirty || saving}>
          {saved ? <Check className="h-3.5 w-3.5" /> : saving ? "保存中..." : "保存"}
        </Button>
      </form>
    </section>
  );
}
