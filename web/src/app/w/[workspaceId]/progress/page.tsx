"use client";

import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowLeft } from "lucide-react";

import { apiFetch } from "@/lib/apiClient";
import { getSupabaseClient } from "@/lib/supabaseClient";
import AssigneeTasksModal from "@/components/AssigneeTasksModal";
import ProgressBar, { SingleBar, type ProgressSegment } from "@/components/ui/ProgressBar";
import { Select } from "@/components/ui/fields";
import Button from "@/components/ui/Button";
import { useProjects } from "@/lib/workspace/ProjectsContext";

type StatusCounts = {
  not_started: number;
  in_progress: number;
  done: number;
  on_hold: number;
};

type ProjectProgress = {
  project_id: string;
  project_name: string;
  counts: StatusCounts;
};

type AssigneeProgress = {
  user_id: string;
  name: string;
  total: number;
  done: number;
};

type ProgressData = {
  by_project: ProjectProgress[];
  by_assignee: AssigneeProgress[];
};

type MemberTask = {
  id: string;
  title: string;
  status: "not_started" | "in_progress" | "done" | "on_hold";
  project_id: string | null;
};

type MemberDrilldown = {
  member: { id: string; user_id: string; name: string; email: string };
  tasks: MemberTask[];
  summary: { total: number; by_status: StatusCounts };
};

const STATUS_LABELS: Record<keyof StatusCounts, string> = {
  not_started: "未対応",
  in_progress: "対応中",
  done: "対応済",
  on_hold: "保留",
};

const STATUS_COLORS: Record<keyof StatusCounts, string> = {
  not_started: "bg-zinc-400 dark:bg-zinc-500",
  in_progress: "bg-indigo-500 dark:bg-indigo-400",
  done: "bg-emerald-500 dark:bg-emerald-400",
  on_hold: "bg-amber-500 dark:bg-amber-400",
};

function totalOf(counts: StatusCounts): number {
  return counts.not_started + counts.in_progress + counts.done + counts.on_hold;
}

function segmentsOf(counts: StatusCounts): ProgressSegment[] {
  return (Object.keys(STATUS_LABELS) as (keyof StatusCounts)[]).map((key) => ({
    key,
    value: counts[key],
    className: STATUS_COLORS[key],
    label: STATUS_LABELS[key],
  }));
}

// 進捗画面（spec.md 4.5）。プロジェクト別・担当者別の棒グラフを表示する。
// ?member_idがある場合はメンバー個人のドリルダウン表示に切り替える
// （MembersPanelのメンバー名クリックから遷移、spec.md 4.7「進捗画面の個人版」）。
// Supabase Realtimeでtasksテーブルの変更を購読し、リロード無しでグラフに反映する
// （ユーザー指定要件）。
export default function ProgressPage() {
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const memberId = searchParams.get("member_id");
  const { projects } = useProjects();

  const [projectFilter, setProjectFilter] = useState("");
  const [data, setData] = useState<ProgressData | null>(null);
  const [drilldown, setDrilldown] = useState<MemberDrilldown | null>(null);
  const [error, setError] = useState<string | null>(null);
  // 「担当者別」の行クリックで開くタスク一覧モーダル（プロジェクト名-タスク名-期限-
  // カンバンステータス）の対象。
  const [assigneeModal, setAssigneeModal] = useState<{ userId: string; name: string } | null>(null);

  const loadProgress = useCallback(() => {
    if (!workspaceId) return;
    const query = projectFilter ? `?project_id=${projectFilter}` : "";
    apiFetch<ProgressData>(`/workspaces/${workspaceId}/progress${query}`)
      .then(setData)
      .catch(() => setError("進捗の取得に失敗しました"));
  }, [workspaceId, projectFilter]);

  const loadDrilldown = useCallback(() => {
    if (!workspaceId || !memberId) return;
    apiFetch<MemberDrilldown>(`/workspaces/${workspaceId}/members/${memberId}/tasks`)
      .then(setDrilldown)
      .catch(() => setError("メンバーのタスク取得に失敗しました"));
  }, [workspaceId, memberId]);

  useEffect(() => {
    if (memberId) {
      loadDrilldown();
    } else {
      loadProgress();
    }
  }, [memberId, loadDrilldown, loadProgress]);

  // タスクの作成・削除・ステータス変更を購読し、都度素朴に再取得する
  // （差分適用ではなく全量再取得。CommentThread.tsxと同じchannel/cleanupパターン）。
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    if (!workspaceId) return;
    const supabase = getSupabaseClient();
    const channel = supabase
      .channel(`tasks:${workspaceId}`)
      .on(
        "postgres_changes",
        { event: "*", schema: "public", table: "tasks", filter: `workspace_id=eq.${workspaceId}` },
        () => {
          if (debounceRef.current) clearTimeout(debounceRef.current);
          debounceRef.current = setTimeout(() => {
            if (memberId) loadDrilldown();
            else loadProgress();
          }, 400);
        },
      )
      .subscribe();
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
      void supabase.removeChannel(channel);
    };
  }, [workspaceId, memberId, loadProgress, loadDrilldown]);

  function backToOverview() {
    const p = new URLSearchParams(searchParams);
    p.delete("member_id");
    const qs = p.toString();
    router.push(qs ? `${pathname}?${qs}` : pathname);
  }

  if (memberId) {
    return (
      <div className="flex max-w-3xl flex-col gap-6 px-6 py-8 lg:px-10">
        <Button variant="ghost" size="sm" onClick={backToOverview} className="self-start">
          <ArrowLeft className="h-3.5 w-3.5" />
          進捗一覧に戻る
        </Button>
        <h1 className="text-2xl font-semibold tracking-tight text-foreground">
          {drilldown?.member.name ?? "..."}さんの進捗
        </h1>
        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
        {drilldown && (
          <>
            <section className="rounded-xl border border-border bg-surface p-4">
              <div className="mb-2 flex items-center justify-between text-sm text-foreground">
                <span>保有タスク {drilldown.summary.total}件</span>
                <span>完了 {drilldown.summary.by_status.done}件</span>
              </div>
              <ProgressBar segments={segmentsOf(drilldown.summary.by_status)} total={drilldown.summary.total} />
              <div className="mt-2 flex flex-wrap gap-3 text-xs text-muted-foreground">
                {(Object.keys(STATUS_LABELS) as (keyof StatusCounts)[]).map((key) => (
                  <span key={key} className="flex items-center gap-1">
                    <span className={`h-2 w-2 rounded-full ${STATUS_COLORS[key]}`} />
                    {STATUS_LABELS[key]} {drilldown.summary.by_status[key]}
                  </span>
                ))}
              </div>
            </section>

            <ul className="flex flex-col gap-2">
              {drilldown.tasks.map((t) => (
                <li
                  key={t.id}
                  className="flex items-center justify-between rounded-lg border border-border bg-surface px-4 py-3 text-sm"
                >
                  <span className="text-foreground">{t.title}</span>
                  <span className="text-xs text-muted-foreground">{STATUS_LABELS[t.status]}</span>
                </li>
              ))}
              {drilldown.tasks.length === 0 && (
                <p className="text-sm text-muted-foreground">担当しているタスクはありません。</p>
              )}
            </ul>
          </>
        )}
      </div>
    );
  }

  return (
    <div className="flex max-w-3xl flex-col gap-8 px-6 py-8 lg:px-10">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight text-foreground">進捗</h1>
        <Select value={projectFilter} onChange={(e) => setProjectFilter(e.target.value)} className="w-auto max-w-xs">
          <option value="">全プロジェクト</option>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </Select>
      </div>

      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

      <section className="flex flex-col gap-4">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">プロジェクト別</h2>
        {data && data.by_project.length === 0 && (
          <p className="text-sm text-muted-foreground">プロジェクトに紐づくタスクがありません。</p>
        )}
        {data?.by_project.map((p) => (
          <div key={p.project_id} className="rounded-xl border border-border bg-surface p-4">
            <div className="mb-2 flex items-center justify-between text-sm font-medium text-foreground">
              <span>{p.project_name}</span>
              <span className="text-xs font-normal text-muted-foreground">{totalOf(p.counts)}件</span>
            </div>
            <ProgressBar segments={segmentsOf(p.counts)} total={totalOf(p.counts)} />
          </div>
        ))}
      </section>

      <section className="flex flex-col gap-4">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">担当者別</h2>
        {data && data.by_assignee.length === 0 && (
          <p className="text-sm text-muted-foreground">担当者が設定されたタスクがありません。</p>
        )}
        {data?.by_assignee.map((a) => (
          <button
            key={a.user_id}
            type="button"
            onClick={() => setAssigneeModal({ userId: a.user_id, name: a.name })}
            className="rounded-xl border border-border bg-surface p-4 text-left transition-colors hover:border-indigo-300 dark:hover:border-indigo-500/50"
          >
            <div className="mb-2 flex items-center justify-between text-sm font-medium text-foreground">
              <span>{a.name}</span>
              <span className="text-xs font-normal text-muted-foreground">
                {a.done}/{a.total}件 完了
              </span>
            </div>
            <SingleBar value={a.done} total={a.total} />
          </button>
        ))}
      </section>

      {assigneeModal && (
        <AssigneeTasksModal
          workspaceId={workspaceId}
          userId={assigneeModal.userId}
          userName={assigneeModal.name}
          onClose={() => setAssigneeModal(null)}
        />
      )}
    </div>
  );
}
