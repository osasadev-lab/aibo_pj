"use client";

import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { AlertTriangle, ChevronLeft, ChevronRight, GanttChartSquare } from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import TaskDetailPanel from "@/components/TaskDetailPanel";
import Avatar from "@/components/ui/Avatar";
import IconButton from "@/components/ui/IconButton";
import { Select } from "@/components/ui/fields";
import { clampDayIndex, datePart, formatDateRangeLabel } from "@/lib/dateRange";
import { useProjects } from "@/lib/workspace/ProjectsContext";
import type { MemberSummary } from "@/lib/types";

type Task = {
  id: string;
  title: string;
  status: "not_started" | "in_progress" | "done" | "on_hold";
  project_id: string | null;
  start_date: string | null;
  due_date: string | null;
  assignee_ids?: string[];
  has_incomplete_dependencies?: boolean;
};

const STATUS_LABELS: Record<Task["status"], string> = {
  not_started: "未対応",
  in_progress: "対応中",
  done: "対応済",
  on_hold: "保留",
};

// 進捗画面（progress/page.tsx）のSTATUS_COLORSと同系統の配色に揃える
// （アプリ全体でステータス=色の対応を統一するため）。
const STATUS_BAR_COLORS: Record<Task["status"], string> = {
  not_started: "bg-zinc-400 dark:bg-zinc-500",
  in_progress: "bg-indigo-500 dark:bg-indigo-400",
  done: "bg-emerald-500 dark:bg-emerald-400",
  on_hold: "bg-amber-500 dark:bg-amber-400",
};

const DAY_COL_WIDTH = 32; // px
const LABEL_COL_WIDTH = 224; // px（w-56相当、sticky列の幅として数値でも参照するため定数化）

function pad2(n: number): string {
  return String(n).padStart(2, "0");
}

function toDateStr(y: number, m: number, d: number): string {
  return `${y}-${pad2(m)}-${pad2(d)}`;
}

function daysInMonth(y: number, m: number): number {
  return new Date(y, m, 0).getDate();
}


// ガントチャート／タイムライン表示（execution-plan.md M9後の運用フィードバックにより
// 2026-08-27追加）。既存のdue_date/start_date/依存関係データをそのまま可視化に転用し、
// 新規バックエンドAPIは追加していない（既存のGET /workspaces/:id/tasksを再利用）。
// 対象は開始日時・期限のどちらか一方でも設定されているタスク（2026-08-27、
// 「日次範囲が片方だけでも表示されてほしい」との要望により拡大。カレンダー画面は
// 引き続き期限のみが対象）。片方しか設定されていない場合はもう片方も同じ日付として
// 扱い、その日1日分のみのバーとして表示する。日時はタスク名の下にラベル表示する。
// スコープ：読み取り専用の表示（バーのドラッグでの日程変更や、依存関係を結ぶ線の描画は
// 今回のスコープ外。先行タスク未完了は既存の警告アイコンで示すのみ）。
export default function GanttPage() {
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const openTaskId = searchParams.get("task");
  const openCommentId = searchParams.get("comment") ?? undefined;
  const { projects } = useProjects();

  const today = new Date();
  const [viewYear, setViewYear] = useState(today.getFullYear());
  const [viewMonth, setViewMonth] = useState(today.getMonth() + 1); // 1-12
  const [projectFilter, setProjectFilter] = useState("");
  const [tasks, setTasks] = useState<Task[]>([]);
  const [members, setMembers] = useState<MemberSummary[]>([]);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!workspaceId) return Promise.resolve();
    const query = projectFilter ? `?project_id=${projectFilter}` : "";
    return apiFetch<Task[]>(`/workspaces/${workspaceId}/tasks${query}`)
      .then(setTasks)
      .catch(() => setError("タスクの取得に失敗しました"));
  }, [workspaceId, projectFilter]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (!workspaceId) return;
    apiFetch<MemberSummary[]>(`/workspaces/${workspaceId}/members`)
      .then(setMembers)
      .catch(() => {});
  }, [workspaceId]);

  const totalDays = daysInMonth(viewYear, viewMonth);
  const monthStart = toDateStr(viewYear, viewMonth, 1);
  const monthEnd = toDateStr(viewYear, viewMonth, totalDays);

  // 表示月に懸かる（開始日または期限日が月内、または月をまたいで覆っている）タスクを
  // 対象にする。開始日時・期限のどちらか一方しか設定されていないタスクも表示する
  // （2026-08-27、ユーザー要望：「日次範囲が片方だけでも表示されてほしい」。
  // 片方のみの場合はもう片方も同じ日付として扱い、その日1日分のバーになる）。
  // 期限日（無ければ開始日時）順に並べる。件数が多くなりにくい画面のため、useMemoは
  // 使わず素朴に毎レンダー計算する（React Compilerがsort()の配列変異を伴うuseMemoの
  // 最適化を見送るため、素朴な計算のままにして不要な警告を避ける）。
  const visibleTasks = tasks
    .filter((t) => {
      if (!t.start_date && !t.due_date) return false;
      // start_date/due_dateは時刻付き（"YYYY-MM-DD HH:MM"）になったため、月境界の
      // 日付のみの文字列（monthStart/monthEnd）と比較する前に日付部分だけを取り出す
      // （そのままだと同じ日の日付同士でも文字列長の違いで比較が狂う）。
      const start = datePart(t.start_date ?? t.due_date!);
      const due = datePart(t.due_date ?? t.start_date!);
      return start <= monthEnd && due >= monthStart;
    })
    .toSorted((a, b) => (a.due_date ?? a.start_date ?? "").localeCompare(b.due_date ?? b.start_date ?? ""));

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

  const isCurrentMonth = viewYear === today.getFullYear() && viewMonth === today.getMonth() + 1;
  const todayDay = today.getDate();

  return (
    <div className="flex flex-col gap-6 px-6 py-8 lg:px-10">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="flex items-center gap-2 text-2xl font-semibold tracking-tight text-foreground">
          <GanttChartSquare className="h-6 w-6 text-muted-foreground" />
          ガントチャート
        </h1>
        <Select value={projectFilter} onChange={(e) => setProjectFilter(e.target.value)} className="w-auto max-w-xs">
          <option value="">全プロジェクト＋単体タスク</option>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </Select>
      </div>

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

      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

      <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
        {(Object.keys(STATUS_LABELS) as Task["status"][]).map((s) => (
          <span key={s} className="flex items-center gap-1.5">
            <span className={clsx("h-2.5 w-2.5 rounded-full", STATUS_BAR_COLORS[s])} />
            {STATUS_LABELS[s]}
          </span>
        ))}
      </div>

      {/* タスク名列（LABEL_COL_WIDTH）はsticky left-0で横スクロールしても固定表示する
          （日付グリッド部分だけがoverflow-x-autoでスクロールする）。sticky対象には
          不透明な背景色が必須（無いとスクロールしてきたセルが透けて重なって見える）。
          今日の列には全行を貫く縦線を重ねて表示し、月をまたぐバーの位置と併せて
          「今どこにいるか」を分かりやすくする。 */}
      <div className="overflow-x-auto rounded-xl border border-border">
        <div className="relative min-w-max">
          {isCurrentMonth && (
            <div
              className="pointer-events-none absolute inset-y-0 z-10 w-px bg-indigo-400 dark:bg-indigo-500"
              style={{ left: LABEL_COL_WIDTH + (todayDay - 1) * DAY_COL_WIDTH }}
            />
          )}
          {/* ヘッダー行：左はタスク列見出し（固定）、右は日番号（今日はハイライト）。 */}
          <div className="flex border-b border-border bg-surface-muted">
            <div
              style={{ width: LABEL_COL_WIDTH }}
              className="sticky left-0 z-20 shrink-0 border-r border-border bg-surface-muted px-3 py-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground"
            >
              タスク
            </div>
            <div className="flex">
              {Array.from({ length: totalDays }, (_, i) => i + 1).map((day) => (
                <div
                  key={day}
                  style={{ width: DAY_COL_WIDTH }}
                  className={clsx(
                    "shrink-0 border-r border-border/60 py-2 text-center text-[10px]",
                    isCurrentMonth && day === todayDay
                      ? "font-semibold text-indigo-700 dark:text-indigo-300"
                      : "text-muted-foreground",
                  )}
                >
                  {day}
                </div>
              ))}
            </div>
          </div>

          {/* 本体：1タスク1行。バーはグリッド列でstart_date〜due_dateの範囲に配置する。
              どちらか一方しか設定されていない場合は、もう片方も同じ日付として扱い
              その日1日分のバーにする（2026-08-27、日次範囲が片方だけでも表示する対応）。 */}
          {visibleTasks.length === 0 ? (
            <div className="flex flex-col items-center gap-2 py-16 text-center">
              <p className="text-sm text-muted-foreground">この月に開始日時・期限があるタスクはありません。</p>
            </div>
          ) : (
            visibleTasks.map((t) => {
              const startStr = t.start_date ?? t.due_date!;
              const dueStr = t.due_date ?? t.start_date!;
              const startCol = clampDayIndex(startStr, viewYear, viewMonth, totalDays, false);
              const endCol = clampDayIndex(dueStr, viewYear, viewMonth, totalDays, true);
              const span = Math.max(1, endCol - startCol + 1);
              const assignees = (t.assignee_ids ?? [])
                .map((id) => members.find((m) => m.user_id === id))
                .filter((m): m is MemberSummary => !!m);
              const projectName = t.project_id ? projects.find((p) => p.id === t.project_id)?.name : null;
              const dateRangeLabel = formatDateRangeLabel(t.start_date, t.due_date);

              return (
                <div key={t.id} className="group flex border-b border-border last:border-b-0">
                  <button
                    type="button"
                    onClick={() => openTask(t.id)}
                    style={{ width: LABEL_COL_WIDTH }}
                    className="sticky left-0 z-10 flex shrink-0 items-center gap-1.5 border-r border-border bg-surface px-3 py-2.5 text-left text-xs group-hover:bg-surface-muted"
                  >
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-foreground">{t.title}</span>
                      {projectName && <span className="block truncate text-[10px] text-muted-foreground">{projectName}</span>}
                      {/* 開始日時・期限は1行にまとめず改行して表示する（2026-08-27、
                          カンバンカードと同じくユーザー要望）。 */}
                      {t.start_date && (
                        <span className="block truncate text-[10px] text-muted-foreground">開始: {t.start_date}</span>
                      )}
                      {t.due_date && (
                        <span className="block truncate text-[10px] text-muted-foreground">期限: {t.due_date}</span>
                      )}
                    </span>
                    {t.has_incomplete_dependencies && (
                      <AlertTriangle
                        className="h-3 w-3 shrink-0 text-amber-500"
                        aria-label="先行タスクが未完了です"
                      />
                    )}
                    {assignees.length > 0 && (
                      <span className="flex shrink-0 -space-x-1">
                        {assignees.slice(0, 2).map((m) => (
                          <Avatar key={m.user_id} name={m.name} seed={m.user_id} size="sm" />
                        ))}
                      </span>
                    )}
                  </button>
                  <div className="relative flex group-hover:bg-surface-muted/40">
                    {Array.from({ length: totalDays }, (_, i) => i + 1).map((day) => (
                      <div
                        key={day}
                        style={{ width: DAY_COL_WIDTH }}
                        className="shrink-0 border-r border-border/40 py-2.5"
                      />
                    ))}
                    <button
                      type="button"
                      onClick={() => openTask(t.id)}
                      title={dateRangeLabel ? `${t.title}（${dateRangeLabel}）` : t.title}
                      style={{
                        left: (startCol - 1) * DAY_COL_WIDTH + 2,
                        width: span * DAY_COL_WIDTH - 4,
                      }}
                      className={clsx(
                        "absolute top-1/2 h-4 -translate-y-1/2 rounded-full shadow-sm transition-opacity hover:opacity-80",
                        STATUS_BAR_COLORS[t.status],
                      )}
                    />
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>

      {openTaskId && (
        <TaskDetailPanel
          key={openTaskId}
          taskId={openTaskId}
          workspaceId={workspaceId}
          onClose={closeTaskPanel}
          onChanged={load}
          initialCommentId={openCommentId}
        />
      )}
    </div>
  );
}
