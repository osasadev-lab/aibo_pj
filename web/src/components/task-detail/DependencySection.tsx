"use client";

import { AlertTriangle, Plus, Trash2 } from "lucide-react";

import Button from "@/components/ui/Button";
import IconButton from "@/components/ui/IconButton";
import { Select } from "@/components/ui/fields";

export type DependencyTask = {
  id: string;
  title: string;
  status: "not_started" | "in_progress" | "done" | "on_hold";
  project_id: string | null;
};

export type DependencyEntry = { id: string; task: DependencyTask };

export type Dependencies = { predecessors: DependencyEntry[]; successors: DependencyEntry[] };

type Props = {
  dependencies: Dependencies;
  candidateTasks: DependencyTask[];
  newDependencyId: string;
  onNewDependencyIdChange: (value: string) => void;
  error: string | null;
  onAdd: (e: React.FormEvent) => void;
  onRemove: (dependencyId: string) => void;
};

// タスク詳細の「先行タスク」区画。状態・追加/解除の実処理はTaskDetailPanel側が
// 持ち、ここは表示に徹する（純粋な見た目の切り出しのみで、ロジック・挙動は変更していない）。
export default function DependencySection({
  dependencies,
  candidateTasks,
  newDependencyId,
  onNewDependencyIdChange,
  error,
  onAdd,
  onRemove,
}: Props) {
  return (
    <div className="flex flex-col gap-2 border-t border-border pt-4">
      <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">先行タスク</p>
      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
      {dependencies.predecessors.length > 0 && (
        <ul className="flex flex-col gap-1">
          {dependencies.predecessors.map((d) => (
            <li
              key={d.id}
              className="flex items-center justify-between gap-2 rounded-lg px-2 py-1.5 text-sm hover:bg-surface-muted"
            >
              <span className="flex min-w-0 items-center gap-1.5">
                {d.task.status !== "done" && <AlertTriangle className="h-3.5 w-3.5 shrink-0 text-amber-500" />}
                <span className="truncate text-foreground">{d.task.title}</span>
              </span>
              <IconButton size="sm" onClick={() => onRemove(d.id)} title="解除">
                <Trash2 className="h-3.5 w-3.5" />
              </IconButton>
            </li>
          ))}
        </ul>
      )}
      <form onSubmit={onAdd} className="flex gap-1.5">
        <Select value={newDependencyId} onChange={(e) => onNewDependencyIdChange(e.target.value)} className="text-sm">
          <option value="">先行タスクを選択...</option>
          {candidateTasks
            .filter((t) => !dependencies.predecessors.some((d) => d.task.id === t.id))
            .map((t) => (
              <option key={t.id} value={t.id}>
                {t.title}
              </option>
            ))}
        </Select>
        <Button type="submit" variant="secondary" size="sm">
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </form>
    </div>
  );
}
