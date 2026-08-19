"use client";

import { useEffect, useState } from "react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";

type Mode = "off" | "tag" | "dependency" | "subtask";

const OPTIONS: { value: Mode; label: string; description: string }[] = [
  { value: "off", label: "しない", description: "ホバーしても何も強調しない" },
  { value: "tag", label: "タグが一致するカード", description: "同じタグを持つカードを強調する" },
  { value: "dependency", label: "先行/後続タスクのカード", description: "依存関係にあるカードを強調する" },
  { value: "subtask", label: "親タスク/子タスクのカード", description: "親子関係にあるカードを強調する" },
];

// プロジェクトのカンバンでタスクカードをホバーした際の強調表示モード（個人設定、全メンバー）。
export default function HoverSettingsSection() {
  const [mode, setMode] = useState<Mode>("off");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    apiFetch<{ mode: Mode }>("/me/hover-settings")
      .then((res) => setMode(res.mode))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  async function handleChange(next: Mode) {
    const prev = mode;
    setMode(next);
    setError(null);
    try {
      await apiFetch("/me/hover-settings", { method: "PATCH", body: JSON.stringify({ mode: next }) });
    } catch {
      setMode(prev);
      setError("ホバー強調設定の更新に失敗しました");
    }
  }

  return (
    <section className="rounded-xl border border-border bg-surface p-4">
      <h2 className="mb-1 text-sm font-semibold text-foreground">ホバー強調</h2>
      <p className="mb-3 text-xs text-muted-foreground">
        プロジェクトのカンバンでタスクカードにマウスを乗せたとき、関連するカードを強調表示します。
      </p>
      {error && <p className="mb-2 text-sm text-red-600 dark:text-red-400">{error}</p>}
      {loading ? (
        <p className="text-sm text-muted-foreground">読み込み中...</p>
      ) : (
        <div className="flex flex-col gap-1.5">
          {OPTIONS.map((opt) => (
            <label
              key={opt.value}
              className={clsx(
                "flex cursor-pointer items-start gap-2 rounded-lg px-2 py-1.5 text-sm transition-colors",
                mode === opt.value ? "bg-indigo-50 dark:bg-indigo-500/10" : "hover:bg-surface-muted",
              )}
            >
              <input
                type="radio"
                name="hover-highlight-mode"
                checked={mode === opt.value}
                onChange={() => handleChange(opt.value)}
                className="mt-1"
              />
              <span>
                <span className="block text-foreground">{opt.label}</span>
                <span className="block text-xs text-muted-foreground">{opt.description}</span>
              </span>
            </label>
          ))}
        </div>
      )}
    </section>
  );
}
