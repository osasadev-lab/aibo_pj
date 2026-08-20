"use client";

import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";
import { ChevronLeft, ChevronRight, Users } from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import MemberPicker from "@/components/MemberPicker";
import TaskDetailPanel from "@/components/TaskDetailPanel";
import Avatar from "@/components/ui/Avatar";
import Button from "@/components/ui/Button";
import IconButton from "@/components/ui/IconButton";
import { Select } from "@/components/ui/fields";
import { useProjects } from "@/lib/workspace/ProjectsContext";
import type { Tag } from "@/lib/types";

type Task = {
  id: string;
  title: string;
  status: "not_started" | "in_progress" | "done" | "on_hold";
  project_id: string | null;
  due_date: string | null;
  tags?: Tag[];
};

type WatchedMember = { user_id: string; name: string; email: string };

const WEEKDAYS = ["日", "月", "火", "水", "木", "金", "土"];

const STATUS_LABELS: Record<Task["status"], string> = {
  not_started: "未対応",
  in_progress: "対応中",
  done: "対応済",
  on_hold: "保留",
};

// カレンダー画面（spec.md 4.4）。月タイル表示で各日のタスク概要を表示する。
// 既定は自分のタスクのみ、「表示するメンバー」で他メンバーを追加すると
// そのメンバーのタスクも同じカレンダーに重ねて表示される（M5追加要件）。
export default function CalendarPage() {
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const openTaskId = searchParams.get("task");
  const { projects } = useProjects();

  const today = new Date();
  const [viewYear, setViewYear] = useState(today.getFullYear());
  const [viewMonth, setViewMonth] = useState(today.getMonth() + 1);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [watched, setWatched] = useState<WatchedMember[]>([]);
  const [showPicker, setShowPicker] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // ステータス・タグ絞り込み（M5追加要望）。月のタスクは既に全件取得済みのため、
  // 絞り込みはクライアント側で行い再取得は発生させない（ホバー強調等と同じ方針）。
  const [statusFilter, setStatusFilter] = useState("");
  const [tagFilter, setTagFilter] = useState("");
  const [availableTags, setAvailableTags] = useState<Tag[]>([]);

  const monthParam = `${viewYear}-${String(viewMonth).padStart(2, "0")}`;

  const loadTasks = useCallback(() => {
    if (!workspaceId) return;
    apiFetch<Task[]>(`/workspaces/${workspaceId}/calendar?month=${monthParam}`)
      .then(setTasks)
      .catch(() => setError("カレンダーの取得に失敗しました"));
  }, [workspaceId, monthParam]);

  const loadWatched = useCallback(() => {
    if (!workspaceId) return;
    apiFetch<WatchedMember[]>(`/workspaces/${workspaceId}/calendar-watched-users`)
      .then(setWatched)
      .catch(() => {});
  }, [workspaceId]);

  useEffect(() => {
    loadTasks();
  }, [loadTasks]);

  useEffect(() => {
    loadWatched();
  }, [loadWatched]);

  async function handleWatchedChange(userIds: string[]) {
    setError(null);
    try {
      await apiFetch(`/workspaces/${workspaceId}/calendar-watched-users`, {
        method: "PUT",
        body: JSON.stringify({ user_ids: userIds }),
      });
      loadWatched();
      loadTasks();
    } catch {
      setError("表示メンバーの更新に失敗しました");
    }
  }

  // タグ絞り込みの選択肢：ワークスペース共通タグ＋自分が閲覧できる各プロジェクトの
  // 専用タグ（`projects`はProjectsContext由来でpublic全件+private参画分のみ、
  // ＝閲覧可能なプロジェクトの集合と一致）。この月のタスクに現れているかは問わず、
  // 使用可能なタグは常に選べるようにする（ユーザー指摘：以前は月内で実際に
  // 使われたタグしか選べなかった）。
  useEffect(() => {
    if (!workspaceId) return;
    let ignore = false;
    Promise.all([
      apiFetch<Tag[]>(`/workspaces/${workspaceId}/common-tags`).catch(() => [] as Tag[]),
      ...projects.map((p) => apiFetch<Tag[]>(`/projects/${p.id}/tags`).catch(() => [] as Tag[])),
    ]).then((lists) => {
      if (ignore) return;
      const map = new Map<string, Tag>();
      for (const list of lists) {
        for (const tag of list) map.set(tag.id, tag);
      }
      setAvailableTags(Array.from(map.values()).sort((a, b) => a.name.localeCompare(b.name)));
    });
    return () => {
      ignore = true;
    };
  }, [workspaceId, projects]);

  const filteredTasks = useMemo(() => {
    return tasks.filter((t) => {
      if (statusFilter && t.status !== statusFilter) return false;
      if (tagFilter && !(t.tags ?? []).some((tag) => tag.id === tagFilter)) return false;
      return true;
    });
  }, [tasks, statusFilter, tagFilter]);

  const tasksByDate = useMemo(() => {
    const map: Record<string, Task[]> = {};
    for (const t of filteredTasks) {
      if (!t.due_date) continue;
      (map[t.due_date] ??= []).push(t);
    }
    return map;
  }, [filteredTasks]);

  function goPrevMonth() {
    if (viewMonth === 1) {
      setViewYear((y) => y - 1);
      setViewMonth(12);
    } else {
      setViewMonth((m) => m - 1);
    }
  }

  function goNextMonth() {
    if (viewMonth === 12) {
      setViewYear((y) => y + 1);
      setViewMonth(1);
    } else {
      setViewMonth((m) => m + 1);
    }
  }

  function openTask(id: string) {
    const p = new URLSearchParams(searchParams);
    p.set("task", id);
    router.replace(`${pathname}?${p.toString()}`);
  }

  function closeTaskPanel() {
    const p = new URLSearchParams(searchParams);
    p.delete("task");
    const qs = p.toString();
    router.replace(qs ? `${pathname}?${qs}` : pathname);
  }

  // new Date(year, monthIndex, day)はローカル時刻で直接組み立てるコンストラクタ
  // （DatePicker.tsxと同じ理由でタイムゾーンずれを避けられる）。
  const firstWeekday = new Date(viewYear, viewMonth - 1, 1).getDay();
  const numDays = new Date(viewYear, viewMonth, 0).getDate();
  const cells: (number | null)[] = [];
  for (let i = 0; i < firstWeekday; i++) cells.push(null);
  for (let d = 1; d <= numDays; d++) cells.push(d);

  const isToday = (day: number) =>
    viewYear === today.getFullYear() && viewMonth === today.getMonth() + 1 && day === today.getDate();

  const watchedIds = watched.map((w) => w.user_id);

  return (
    <div className="flex flex-col gap-6 px-6 py-8 lg:px-10">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight text-foreground">カレンダー</h1>
        <div className="relative">
          <Button variant="secondary" size="sm" onClick={() => setShowPicker((v) => !v)}>
            <Users className="h-3.5 w-3.5" />
            表示するメンバー
          </Button>
          {showPicker && (
            <div className="absolute right-0 z-20 mt-2 w-72 rounded-lg border border-border bg-surface p-3 shadow-lg">
              <p className="mb-2 text-xs text-muted-foreground">
                追加したメンバーのタスクも自分のカレンダーに重ねて表示します。
              </p>
              <MemberPicker workspaceId={workspaceId} selected={watchedIds} onChange={handleWatchedChange} />
            </div>
          )}
        </div>
      </div>

      {watched.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs text-muted-foreground">表示中:</span>
          {watched.map((w) => (
            <span
              key={w.user_id}
              className="flex items-center gap-1.5 rounded-full border border-border bg-surface px-2 py-1 text-xs text-foreground"
            >
              <Avatar name={w.name} seed={w.user_id} size="sm" />
              {w.name}
            </span>
          ))}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <Select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} className="w-auto max-w-[10rem]">
          <option value="">すべてのステータス</option>
          {(Object.keys(STATUS_LABELS) as Task["status"][]).map((s) => (
            <option key={s} value={s}>
              {STATUS_LABELS[s]}
            </option>
          ))}
        </Select>
        <Select value={tagFilter} onChange={(e) => setTagFilter(e.target.value)} className="w-auto max-w-[10rem]">
          <option value="">すべてのタグ</option>
          {availableTags.map((tag) => (
            <option key={tag.id} value={tag.id}>
              {tag.name}
            </option>
          ))}
        </Select>
      </div>

      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

      <div className="flex items-center justify-center gap-4">
        <IconButton onClick={goPrevMonth} title="前の月">
          <ChevronLeft className="h-4 w-4" />
        </IconButton>
        <span className="text-lg font-medium text-foreground">
          {viewYear}年{viewMonth}月
        </span>
        <IconButton onClick={goNextMonth} title="次の月">
          <ChevronRight className="h-4 w-4" />
        </IconButton>
      </div>

      <div className="grid grid-cols-7 gap-px overflow-hidden rounded-xl border border-border bg-border">
        {WEEKDAYS.map((w) => (
          <div key={w} className="bg-surface-muted py-2 text-center text-xs font-medium text-muted-foreground">
            {w}
          </div>
        ))}
        {cells.map((day, i) => {
          if (day === null) {
            return <div key={`empty-${i}`} className="min-h-24 bg-surface" />;
          }
          const dateStr = `${viewYear}-${String(viewMonth).padStart(2, "0")}-${String(day).padStart(2, "0")}`;
          const dayTasks = tasksByDate[dateStr] ?? [];
          return (
            <div key={day} className="flex min-h-24 flex-col gap-1 bg-surface p-1.5">
              <span
                className={clsx(
                  "text-xs",
                  isToday(day) ? "font-semibold text-indigo-600 dark:text-indigo-400" : "text-muted-foreground",
                )}
              >
                {day}
              </span>
              <div className="flex flex-col gap-0.5">
                {dayTasks.slice(0, 3).map((t) => (
                  <button
                    key={t.id}
                    type="button"
                    onClick={() => openTask(t.id)}
                    className="truncate rounded bg-indigo-50 px-1.5 py-0.5 text-left text-[11px] text-indigo-700 hover:bg-indigo-100 dark:bg-indigo-500/10 dark:text-indigo-300 dark:hover:bg-indigo-500/20"
                  >
                    {t.title}
                  </button>
                ))}
                {dayTasks.length > 3 && (
                  <span className="px-1.5 text-[11px] text-muted-foreground">+{dayTasks.length - 3}件</span>
                )}
              </div>
            </div>
          );
        })}
      </div>

      {openTaskId && (
        <TaskDetailPanel
          key={openTaskId}
          taskId={openTaskId}
          workspaceId={workspaceId}
          onClose={closeTaskPanel}
          onChanged={loadTasks}
        />
      )}
    </div>
  );
}
