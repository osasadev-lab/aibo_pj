"use client";

import { useEffect, useState } from "react";
import { Check } from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import Badge from "@/components/ui/Badge";
import type { Tag } from "@/lib/types";

type Props = {
  taskId: string;
  selected: string[];
  onChange: (tagIds: string[]) => void;
};

// タスクに付与可能なタグ（所属プロジェクトの専用タグ＋ワークスペース共通タグ、
// 単体タスクなら共通タグのみ）のチェックボックス選択UI。MemberPickerと同じ構造。
export default function TagPicker({ taskId, selected, onChange }: Props) {
  const [tags, setTags] = useState<Tag[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let ignore = false;
    apiFetch<Tag[]>(`/tasks/${taskId}/assignable-tags`)
      .then((list) => {
        if (!ignore) setTags(list);
      })
      .finally(() => {
        if (!ignore) setLoading(false);
      });
    return () => {
      ignore = true;
    };
  }, [taskId]);

  function toggle(tagId: string) {
    if (selected.includes(tagId)) {
      onChange(selected.filter((id) => id !== tagId));
    } else {
      onChange([...selected, tagId]);
    }
  }

  if (loading) {
    return <p className="text-sm text-muted-foreground">読み込み中...</p>;
  }

  if (tags.length === 0) {
    return <p className="text-sm text-muted-foreground/70">付与できるタグがありません（「設定」から作成できます）</p>;
  }

  return (
    <div className="flex flex-wrap gap-1.5">
      {tags.map((tag) => {
        const active = selected.includes(tag.id);
        return (
          <button
            key={tag.id}
            type="button"
            onClick={() => toggle(tag.id)}
            className={clsx("rounded-full transition-opacity", !active && "opacity-40 hover:opacity-70")}
          >
            <Badge tone={tag.color as "zinc" | "red" | "amber" | "green" | "indigo"}>
              {active && <Check className="h-3 w-3" />}
              {tag.name}
            </Badge>
          </button>
        );
      })}
    </div>
  );
}
