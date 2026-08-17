import clsx from "clsx";
import type { ReactNode } from "react";

type Tone = "zinc" | "red" | "amber" | "green" | "indigo";

const tones: Record<Tone, string> = {
  zinc: "bg-surface-muted text-muted-foreground",
  red: "bg-red-50 text-red-700 dark:bg-red-950/50 dark:text-red-300",
  amber: "bg-amber-50 text-amber-700 dark:bg-amber-950/50 dark:text-amber-300",
  green: "bg-emerald-50 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300",
  indigo: "bg-indigo-50 text-indigo-700 dark:bg-indigo-950/50 dark:text-indigo-300",
};

// 優先度・ステータス等の小さなピル表示。
export default function Badge({ tone = "zinc", children }: { tone?: Tone; children: ReactNode }) {
  return (
    <span className={clsx("inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium", tones[tone])}>
      {children}
    </span>
  );
}
