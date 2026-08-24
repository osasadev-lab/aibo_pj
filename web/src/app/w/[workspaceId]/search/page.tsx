"use client";

import { useCallback, useEffect, useState } from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { Search as SearchIcon } from "lucide-react";

import { apiFetch } from "@/lib/apiClient";
import Badge from "@/components/ui/Badge";
import { useProjects, type WorkspaceProject } from "@/lib/workspace/ProjectsContext";

// タイトル・説明は1タスクに1つしか無いため一致すれば文字列1件、コメント・添付ファイルは
// 複数一致しうるため配列（バックエンドはtask_id INの一括クエリでN+1を避けて集計する）。
type TaskMatches = {
  title?: string;
  description?: string;
  comments: string[];
  attachments: string[];
};

type Task = {
  id: string;
  project_id: string | null;
  title: string;
  status: "not_started" | "in_progress" | "done" | "on_hold";
  due_date: string | null;
  // APIを叩くサーバーが古いビルドの場合に備えて防御的にoptional扱いする。
  matches?: TaskMatches;
};

const STATUS_LABELS: Record<Task["status"], string> = {
  not_started: "未対応",
  in_progress: "対応中",
  done: "対応済",
  on_hold: "保留",
};

function projectName(t: Task, projects: WorkspaceProject[]): string {
  if (!t.project_id) return "単体タスク";
  return projects.find((p) => p.id === t.project_id)?.name ?? "単体タスク";
}

function taskHref(workspaceId: string, t: Task): string {
  return t.project_id
    ? `/w/${workspaceId}/projects/${t.project_id}?task=${t.id}`
    : `/w/${workspaceId}/my-tasks?task=${t.id}`;
}

// 1タスク分の行。extraLinesにはその区分で一致した内容（説明の抜粋、コメントの抜粋、
// 添付ファイル名等）を渡す。
function TaskRow({
  t,
  extraLines,
  workspaceId,
  projects,
}: {
  t: Task;
  extraLines: string[];
  workspaceId: string;
  projects: WorkspaceProject[];
}) {
  const router = useRouter();
  return (
    <li>
      <button
        type="button"
        onClick={() => router.push(taskHref(workspaceId, t))}
        className="flex w-full items-center justify-between gap-3 rounded-xl border border-border bg-surface px-4 py-3 text-left text-sm transition-colors hover:border-indigo-300 dark:hover:border-indigo-500/50"
      >
        <span className="min-w-0 flex-1">
          <span className="block truncate text-foreground">{t.title}</span>
          <span className="text-xs text-muted-foreground">{projectName(t, projects)}</span>
          {extraLines.length > 0 && (
            <ul className="mt-1 flex flex-col gap-0.5">
              {extraLines.map((line, i) => (
                <li key={i} className="truncate text-xs text-muted-foreground/80">
                  {line}
                </li>
              ))}
            </ul>
          )}
        </span>
        <Badge>{STATUS_LABELS[t.status]}</Badge>
      </button>
    </li>
  );
}

// 「タスク/説明/コメント/添付ファイル」の1区分。見出しの下にヒットしたタスクを並べ、
// 1件も無ければ「ヒットなし」を表示する（ユーザー指定のレイアウト要望）。
function MatchSection({
  label,
  hits,
  workspaceId,
  projects,
}: {
  label: string;
  hits: { t: Task; extraLines: string[] }[];
  workspaceId: string;
  projects: WorkspaceProject[];
}) {
  return (
    <div className="flex flex-col gap-2">
      <h2 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">－{label}－</h2>
      {hits.length === 0 ? (
        <p className="text-sm text-muted-foreground/70">ヒットなし</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {hits.map(({ t, extraLines }) => (
            <TaskRow key={t.id} t={t} extraLines={extraLines} workspaceId={workspaceId} projects={projects} />
          ))}
        </ul>
      )}
    </div>
  );
}

// プロジェクト横断の全文検索結果画面（タスク名・説明・コメント・添付ファイル名が
// 対象、docs/aibo/m7-implementation-plan.md 設計判断6）。検索の実行は左サイドバーの
// 入力欄（Enterキー）のみで行い、このページ自体には入力欄を置かない。
// レイアウトは「タスク/説明/コメント/添付ファイル」の4区分を見出しとして表示し、
// 各区分の下にその区分でヒットしたタスクを並べる（ユーザー指定のレイアウト要望。
// 1タスクの中に4区分をまとめる案は不採用になった）。
export default function SearchPage() {
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;
  const searchParams = useSearchParams();
  const { projects } = useProjects();

  const q = searchParams.get("q") ?? "";
  const [tasks, setTasks] = useState<Task[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!workspaceId || !q) return;
    apiFetch<Task[]>(`/workspaces/${workspaceId}/search?q=${encodeURIComponent(q)}`)
      .then((result) => {
        setTasks(result);
        setError(null);
      })
      .catch(() => setError("検索に失敗しました"));
  }, [workspaceId, q]);

  useEffect(() => {
    load();
  }, [load]);

  const titleHits = (tasks ?? [])
    .filter((t) => t.matches?.title)
    .map((t) => ({ t, extraLines: [] as string[] }));
  const descriptionHits = (tasks ?? [])
    .filter((t) => t.matches?.description)
    .map((t) => ({ t, extraLines: [t.matches!.description!] }));
  const commentHits = (tasks ?? [])
    .filter((t) => (t.matches?.comments.length ?? 0) > 0)
    .map((t) => ({ t, extraLines: t.matches!.comments }));
  const attachmentHits = (tasks ?? [])
    .filter((t) => (t.matches?.attachments.length ?? 0) > 0)
    .map((t) => ({ t, extraLines: t.matches!.attachments }));

  return (
    <div className="flex max-w-3xl flex-col gap-6 px-6 py-8 lg:px-10">
      <h1 className="text-2xl font-semibold tracking-tight text-foreground">
        {q ? `「${q}」の検索結果` : "検索"}
      </h1>

      {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

      {!q ? (
        <p className="text-sm text-muted-foreground">左サイドバーの検索欄にキーワードを入力してEnterを押してください。</p>
      ) : tasks === null ? (
        <p className="text-sm text-muted-foreground">読み込み中...</p>
      ) : tasks.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-border py-16 text-center">
          <SearchIcon className="h-6 w-6 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground">「{q}」に一致するタスクは見つかりませんでした。</p>
        </div>
      ) : (
        <div className="flex flex-col gap-6">
          <MatchSection label="タスク" hits={titleHits} workspaceId={workspaceId} projects={projects} />
          <MatchSection label="説明" hits={descriptionHits} workspaceId={workspaceId} projects={projects} />
          <MatchSection label="コメント" hits={commentHits} workspaceId={workspaceId} projects={projects} />
          <MatchSection label="添付ファイル" hits={attachmentHits} workspaceId={workspaceId} projects={projects} />
        </div>
      )}
    </div>
  );
}
