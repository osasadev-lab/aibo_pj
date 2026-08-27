"use client";

import { useState } from "react";
import clsx from "clsx";

type Theme = "dark" | "light";

const STORAGE_KEY = "aisu-theme";

const OPTIONS: { value: Theme; label: string; description: string }[] = [
  { value: "dark", label: "ダーク", description: "既定のテーマ" },
  { value: "light", label: "ライト", description: "明るい配色に切り替える" },
];

function readStoredTheme(): Theme {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === "light" ? "light" : "dark";
  } catch {
    return "dark";
  }
}

// ダーク/ライトの切替（個人設定、M8）。サーバー保存はせずlocalStorageのみで永続化する
// （docs/aibo/m8-implementation-plan.md 設計判断4）。このコンポーネントは認証ゲート済みの
// ワークスペースレイアウト配下でのみマウントされる（=既にクライアント側での再レンダリング後）
// ため、遅延初期化でのwindow参照はハイドレーション不整合を起こさない
// （PushNotificationSection.tsxの既存コメントと同じ理由）。
export default function ThemeSection() {
  const [theme, setTheme] = useState<Theme>(readStoredTheme);

  function handleChange(next: Theme) {
    setTheme(next);
    document.documentElement.classList.toggle("dark", next === "dark");
    try {
      window.localStorage.setItem(STORAGE_KEY, next);
    } catch {
      // プライベートブラウジング等でlocalStorageが使えない場合は、この
      // セッション中の見た目切り替えだけ反映し、永続化は諦める。
    }
  }

  return (
    <section className="rounded-xl border border-border bg-surface p-4">
      <h2 className="mb-1 text-sm font-semibold text-foreground">テーマ</h2>
      <p className="mb-3 text-xs text-muted-foreground">
        この端末・ブラウザでの表示テーマを切り替えます（他の端末には反映されません）。
      </p>
      <div className="flex flex-col gap-1.5">
        {OPTIONS.map((opt) => (
          <label
            key={opt.value}
            className={clsx(
              "flex cursor-pointer items-start gap-2 rounded-lg px-2 py-1.5 text-sm transition-colors",
              theme === opt.value ? "bg-indigo-50 dark:bg-indigo-500/10" : "hover:bg-surface-muted",
            )}
          >
            <input
              type="radio"
              name="theme"
              checked={theme === opt.value}
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
    </section>
  );
}
