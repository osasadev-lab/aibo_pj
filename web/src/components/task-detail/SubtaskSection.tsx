"use client";

import { Plus, Trash2 } from "lucide-react";

import Button from "@/components/ui/Button";
import IconButton from "@/components/ui/IconButton";
import { Input } from "@/components/ui/fields";

type SubtaskEntry = { id: string; title: string };

type Props = {
  subtasks: SubtaskEntry[];
  subtaskTitle: string;
  onSubtaskTitleChange: (value: string) => void;
  onAdd: (e: React.FormEvent) => void;
  onDelete: (id: string) => void;
};

// タスク詳細の「子タスク」区画。状態・作成/削除の実処理はTaskDetailPanel側が
// 持ち、ここは表示に徹する（純粋な見た目の切り出しのみで、ロジック・挙動は変更していない）。
export default function SubtaskSection({ subtasks, subtaskTitle, onSubtaskTitleChange, onAdd, onDelete }: Props) {
  return (
    <div className="flex flex-col gap-2 border-t border-border pt-4">
      <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">子タスク</p>
      {subtasks.length > 0 && (
        <ul className="flex flex-col gap-1">
          {subtasks.map((st) => (
            <li
              key={st.id}
              className="flex items-center justify-between rounded-lg px-2 py-1.5 text-sm hover:bg-surface-muted"
            >
              <span className="text-foreground">{st.title}</span>
              <IconButton size="sm" onClick={() => onDelete(st.id)} title="削除">
                <Trash2 className="h-3.5 w-3.5" />
              </IconButton>
            </li>
          ))}
        </ul>
      )}
      <form onSubmit={onAdd} className="flex gap-1.5">
        <Input
          value={subtaskTitle}
          onChange={(e) => onSubtaskTitleChange(e.target.value)}
          placeholder="子タスク名"
          className="text-sm"
        />
        <Button type="submit" variant="secondary" size="sm">
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </form>
    </div>
  );
}
