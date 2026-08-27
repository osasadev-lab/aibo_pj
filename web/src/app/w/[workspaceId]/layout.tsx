"use client";

import Link from "next/link";
import { usePathname, useParams, useRouter } from "next/navigation";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  Activity,
  ArrowLeftRight,
  BarChart3,
  Bell,
  BookOpen,
  Calendar,
  Check,
  Folder,
  GanttChartSquare,
  Globe,
  GripVertical,
  ListChecks,
  Lock,
  LogOut,
  Menu,
  MessageCircle,
  MessageSquarePlus,
  NotebookPen,
  Pencil,
  Pin,
  Settings,
  Trash2,
  Users,
  X,
} from "lucide-react";
import clsx from "clsx";
import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

import { apiFetch } from "@/lib/apiClient";
import { getSupabaseClient } from "@/lib/supabaseClient";
import { useAuth } from "@/lib/auth/useAuth";
import Avatar from "@/components/ui/Avatar";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import IconButton from "@/components/ui/IconButton";
import { Input } from "@/components/ui/fields";
import ActivityPanel from "@/components/ActivityPanel";
import MembersPanel from "@/components/MembersPanel";
import NotificationsPanel from "@/components/NotificationsPanel";
import PinnedTasksPanel from "@/components/PinnedTasksPanel";
import MemoListPanel from "@/components/MemoListPanel";
import GuidePanel from "@/components/GuidePanel";
import FeedbackPanel from "@/components/FeedbackPanel";
import SearchInput from "@/components/SearchInput";
import { ProjectsProvider, useProjects } from "@/lib/workspace/ProjectsContext";
import { CurrentProjectProvider, useCurrentProject } from "@/lib/workspace/CurrentProjectContext";
import { RightPanelProvider, useRightPanel } from "@/lib/workspace/RightPanelContext";
import type { Workspace } from "@/lib/types";

export default function WorkspaceLayout({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth();
  const router = useRouter();
  const params = useParams<{ workspaceId: string }>();

  useEffect(() => {
    if (!loading && !user) {
      router.replace("/login");
    }
  }, [loading, user, router]);

  if (loading || !user) {
    return (
      <div className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">
        読み込み中...
      </div>
    );
  }

  return (
    <RightPanelProvider>
      <ProjectsProvider workspaceId={params.workspaceId}>
        <CurrentProjectProvider workspaceId={params.workspaceId}>
          <WorkspaceLayoutInner>{children}</WorkspaceLayoutInner>
        </CurrentProjectProvider>
      </ProjectsProvider>
    </RightPanelProvider>
  );
}

function WorkspaceLayoutInner({ children }: { children: ReactNode }) {
  const { user, logout } = useAuth();
  const router = useRouter();
  const pathname = usePathname();
  const params = useParams<{ workspaceId: string }>();
  const { projects } = useProjects();

  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [editingWorkspaceName, setEditingWorkspaceName] = useState(false);
  const [workspaceNameDraft, setWorkspaceNameDraft] = useState("");
  const [showDeleteWorkspaceConfirm, setShowDeleteWorkspaceConfirm] = useState(false);
  const [deleteWorkspaceError, setDeleteWorkspaceError] = useState<string | null>(null);
  // メンバー・通知は別ルートへ遷移せず、現在のページ（プロジェクトのカンバンや
  // マイタスク等）を残したまま右サイドバーに重ねて開く。ページ遷移だと直前の
  // 画面が丸ごとアンマウントされてタスクが見えなくなってしまうため。
  // 開閉状態はRightPanelContextに一本化している（同時に複数開いて重なって隠れる
  // 不具合の修正、2026-08-27）。
  const { isOpen: isRightPanelOpen, open: openRightPanelRaw, close: closeRightPanel } = useRightPanel();
  const [hasUnread, setHasUnread] = useState(false);
  const [hasUnreadDM, setHasUnreadDM] = useState(false);
  // モバイル用ハンバーガーメニューの開閉（M8）。lg未満でのみ使う。ページ遷移のたびに
  // 自動で閉じる（ナビゲーション項目タップ後の体験のため）。エフェクトではなく
  // レンダー中の「前回pathnameとの比較」で行う（React公式が推奨する
  // 「propの変化に応じたstateの調整」パターン、setState-in-effectの
  // カスケード再レンダリングを避けるため）。
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  // NavButton（メンバー/通知/ハイライト/ピン留め/フィードバック）はNavLinkと違い
  // ページ遷移しないため、モバイルのハンバーガーメニューを開いたまま押すと
  // mobileNavOpenが閉じられず、左サイドバー（z-50）が右サイドバー（z-40）の
  // 上に被さって隠れてしまう不具合があった。パネルを開くタイミングで
  // ハンバーガーメニューも閉じるようにする。
  const openRightPanel: typeof openRightPanelRaw = (id) => {
    setMobileNavOpen(false);
    openRightPanelRaw(id);
  };
  const [prevPathname, setPrevPathname] = useState(pathname);
  if (pathname !== prevPathname) {
    setPrevPathname(pathname);
    setMobileNavOpen(false);
  }

  useEffect(() => {
    if (!params.workspaceId) return;
    apiFetch<Workspace>(`/workspaces/${params.workspaceId}`)
      .then(setWorkspace)
      .catch(() => setWorkspace(null));
  }, [params.workspaceId]);

  // 未読の有無だけをここで把握し、通知パネルを開かずとも左サイドバーの
  // 「通知」ラベルを明るく表示する。パネルを閉じた直後（既読化された可能性が
  // ある）に再取得する。有無だけ分かればよいためlimit=1で十分（2026-08-27、
  // ページネーション対応でレスポンスが{items, has_more}形に変わったのに合わせて更新）。
  const refreshUnread = () => {
    apiFetch<{ items: unknown[]; has_more: boolean }>("/notifications?unread=true&limit=1")
      .then((res) => setHasUnread(res.items.length > 0))
      .catch(() => {});
  };

  useEffect(() => {
    refreshUnread();
  }, []);

  // 新着通知（アサイン・メンション・リマインダー等）が来てもリロードしないと
  // 強調表示が更新されない問題への対応（ユーザーフィードバック）。comments/tasksと
  // 同じくSupabase Realtimeでnotificationsテーブルの変更を直接購読し、
  // 自分宛のINSERT時に未読バッジを即時再取得する。
  useEffect(() => {
    if (!user) return;
    const supabase = getSupabaseClient();
    const channel = supabase
      .channel(`notifications:${user.id}`)
      .on(
        "postgres_changes",
        { event: "INSERT", schema: "public", table: "notifications", filter: `user_id=eq.${user.id}` },
        () => refreshUnread(),
      )
      .subscribe();
    return () => {
      void supabase.removeChannel(channel);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [user?.id]);

  // DM（M8.5）の未読有無。チャンネル一覧のunreadフラグのいずれかがtrueかで判定する
  // （通知ベルと同じ「有無だけ分かればよい」考え方）。
  const refreshUnreadDM = () => {
    if (!params.workspaceId) return;
    apiFetch<{ unread: boolean }[]>(`/workspaces/${params.workspaceId}/dm/channels`)
      .then((channels) => setHasUnreadDM(channels.some((c) => c.unread)))
      .catch(() => {});
  };

  useEffect(() => {
    refreshUnreadDM();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params.workspaceId]);

  // dm_messagesはRLSで自分が参加するチャンネルのみ購読対象に絞られるため、
  // filterは付けずINSERT全体を購読すればよい（commentsと同じ考え方）。
  useEffect(() => {
    if (!user) return;
    const supabase = getSupabaseClient();
    const channel = supabase
      .channel(`dm_messages:${user.id}`)
      .on("postgres_changes", { event: "INSERT", schema: "public", table: "dm_messages" }, () => refreshUnreadDM())
      .subscribe();
    return () => {
      void supabase.removeChannel(channel);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [user?.id, params.workspaceId]);

  const isWorkspaceOwner = workspace?.role === "owner";

  async function handleRenameWorkspace(e: React.FormEvent) {
    e.preventDefault();
    if (!workspaceNameDraft.trim()) return;
    try {
      const updated = await apiFetch<Workspace>(`/workspaces/${params.workspaceId}`, {
        method: "PATCH",
        body: JSON.stringify({ name: workspaceNameDraft.trim() }),
      });
      setWorkspace((prev) => (prev ? { ...prev, name: updated.name } : prev));
      setEditingWorkspaceName(false);
    } catch {
      // 失敗時は編集フォームを残す（ユーザーが再試行できるように）。
    }
  }

  async function handleDeleteWorkspace() {
    setDeleteWorkspaceError(null);
    try {
      await apiFetch(`/workspaces/${params.workspaceId}`, { method: "DELETE" });
      router.replace("/workspaces");
    } catch {
      setDeleteWorkspaceError("ワークスペースの削除に失敗しました");
    }
  }

  // M8追加：タスク詳細のポップアウト専用ウィンドウ（/w/:workspaceId/tasks/:taskId）は
  // 左サイドバー等のチェインUIを一切描画せず、childrenだけを最小限のラッパーで返す
  // （認証ガード・ProjectsProvider/CurrentProjectProviderはWorkspaceLayout側で維持される。
  // docs/aibo/m8-implementation-plan.md スコープ追加C）。
  const isPopoutRoute = /^\/w\/[^/]+\/tasks\/[^/]+$/.test(pathname);

  // 左サイドバーのプロジェクト一覧の個人ごとの並び順（M8）。workspace.project_orderに
  // 含まれるIDはその順に、含まれないプロジェクト（新規作成分等）はAPI返却順のまま
  // 末尾に補う。
  const sortedProjects = useMemo(() => {
    const order = workspace?.project_order ?? [];
    const indexed = projects.map((p, i) => ({ p, i }));
    return indexed
      .sort((a, b) => {
        const ai = order.indexOf(a.p.id);
        const bi = order.indexOf(b.p.id);
        if (ai === -1 && bi === -1) return a.i - b.i;
        if (ai === -1) return 1;
        if (bi === -1) return -1;
        return ai - bi;
      })
      .map(({ p }) => p);
  }, [projects, workspace?.project_order]);

  const sortSensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));

  async function handleReorderProjects(event: DragEndEvent) {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = sortedProjects.findIndex((p) => p.id === active.id);
    const newIndex = sortedProjects.findIndex((p) => p.id === over.id);
    if (oldIndex === -1 || newIndex === -1) return;
    const newOrder = arrayMove(sortedProjects, oldIndex, newIndex).map((p) => p.id);
    setWorkspace((prev) => (prev ? { ...prev, project_order: newOrder } : prev));
    try {
      await apiFetch(`/workspaces/${params.workspaceId}/project-order`, {
        method: "PATCH",
        body: JSON.stringify({ project_order: newOrder }),
      });
    } catch {
      // 失敗時は次回のGET /workspaces/:idで実際の状態に巻き戻る。
    }
  }

  if (!user) return null;

  if (isPopoutRoute) {
    return <div className="min-h-screen bg-background">{children}</div>;
  }

  const projectsHref = `/w/${params.workspaceId}/projects`;
  const calendarHref = `/w/${params.workspaceId}/calendar`;
  const myTasksHref = `/w/${params.workspaceId}/my-tasks`;
  const progressHref = `/w/${params.workspaceId}/progress`;
  const ganttHref = `/w/${params.workspaceId}/gantt`;
  const dmHref = `/w/${params.workspaceId}/dm`;
  const settingsHref = `/w/${params.workspaceId}/settings`;

  return (
    <div className="flex min-h-screen bg-background">
      {/* モバイル用ハンバーガーヘッダー（lg未満のみ表示、M8）。左サイドバーは
          既定でオフキャンバス（画面外）になるため、開くための入口として常時表示する。 */}
      <header className="fixed inset-x-0 top-0 z-30 flex items-center gap-2 border-b border-border bg-surface px-3 py-3 lg:hidden">
        <IconButton onClick={() => setMobileNavOpen(true)} title="メニューを開く">
          <Menu className="h-5 w-5" />
        </IconButton>
        <span className="truncate text-sm font-semibold text-foreground">{workspace?.name ?? "..."}</span>
      </header>
      {mobileNavOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/40 lg:hidden"
          onClick={() => setMobileNavOpen(false)}
        />
      )}

      <aside
        className={clsx(
          "fixed inset-y-0 left-0 z-50 flex w-72 shrink-0 flex-col border-r border-border bg-surface transition-transform duration-200",
          "lg:static lg:translate-x-0",
          mobileNavOpen ? "translate-x-0" : "-translate-x-full",
        )}
      >
        {editingWorkspaceName ? (
          <form
            onSubmit={handleRenameWorkspace}
            className="flex items-center gap-1.5 border-b border-border px-4 py-4"
          >
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-indigo-600 text-sm font-semibold text-white">
              {workspace?.name?.slice(0, 1) ?? "…"}
            </span>
            <Input
              autoFocus
              value={workspaceNameDraft}
              onChange={(e) => setWorkspaceNameDraft(e.target.value)}
              className="h-8 min-w-0 flex-1 py-1 text-sm"
            />
            <IconButton size="sm" type="submit" title="保存">
              <Check className="h-3.5 w-3.5" />
            </IconButton>
            <IconButton size="sm" type="button" title="取消" onClick={() => setEditingWorkspaceName(false)}>
              <X className="h-3.5 w-3.5" />
            </IconButton>
          </form>
        ) : (
          <div className="group flex items-center gap-1 border-b border-border px-4 py-4">
            <Link
              href="/workspaces"
              className="flex min-w-0 flex-1 items-center gap-2 rounded-lg transition-colors hover:bg-surface-muted"
            >
              <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-indigo-600 text-sm font-semibold text-white">
                {workspace?.name?.slice(0, 1) ?? "…"}
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-semibold text-foreground">
                  {workspace?.name ?? "..."}
                </span>
                <span className="flex items-center gap-1 text-xs text-muted-foreground">
                  <ArrowLeftRight className="h-3 w-3" />
                  切替
                </span>
              </span>
            </Link>
            {isWorkspaceOwner && (
              <div className="flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
                <IconButton
                  size="sm"
                  title="ワークスペース名を編集"
                  onClick={() => {
                    setWorkspaceNameDraft(workspace?.name ?? "");
                    setEditingWorkspaceName(true);
                  }}
                >
                  <Pencil className="h-3.5 w-3.5" />
                </IconButton>
                <IconButton
                  size="sm"
                  title="ワークスペースを削除"
                  onClick={() => {
                    setDeleteWorkspaceError(null);
                    setShowDeleteWorkspaceConfirm(true);
                  }}
                >
                  <Trash2 className="h-3.5 w-3.5 hover:text-red-600" />
                </IconButton>
              </div>
            )}
          </div>
        )}

        <div className="border-b border-border px-3 py-3">
          <SearchInput />
        </div>

        <nav className="flex flex-1 flex-col gap-0.5 overflow-y-auto px-3 py-3">
          <NavLink href={projectsHref} label="プロジェクト" icon={Folder} active={pathname === projectsHref} />
          {sortedProjects.length > 0 && (
            <DndContext sensors={sortSensors} collisionDetection={closestCenter} onDragEnd={handleReorderProjects}>
              <SortableContext items={sortedProjects.map((p) => p.id)} strategy={verticalListSortingStrategy}>
                <ul className="mb-1 ml-3.5 flex flex-col gap-0.5 border-l border-border pl-3">
                  {sortedProjects.map((p) => {
                    const href = `${projectsHref}/${p.id}`;
                    const active = pathname === href;
                    return (
                      <ProjectNavItem
                        key={p.id}
                        id={p.id}
                        name={p.name}
                        visibility={p.visibility}
                        href={href}
                        active={active}
                      />
                    );
                  })}
                </ul>
              </SortableContext>
            </DndContext>
          )}
          <NavLink
            href={calendarHref}
            label="カレンダー"
            icon={Calendar}
            active={pathname === calendarHref || pathname.startsWith(`${calendarHref}/`)}
          />
          <NavLink
            href={myTasksHref}
            label="マイタスク"
            icon={ListChecks}
            active={pathname === myTasksHref || pathname.startsWith(`${myTasksHref}/`)}
          />
          <NavLink
            href={progressHref}
            label="進捗"
            icon={BarChart3}
            active={pathname === progressHref || pathname.startsWith(`${progressHref}/`)}
          />
          <NavLink
            href={ganttHref}
            label="ガントチャート"
            icon={GanttChartSquare}
            active={pathname === ganttHref || pathname.startsWith(`${ganttHref}/`)}
          />
          <NavLink
            href={dmHref}
            label="DM"
            icon={MessageCircle}
            active={pathname === dmHref || pathname.startsWith(`${dmHref}/`)}
            emphasize={hasUnreadDM}
          />
          <NavButton
            label="メンバー"
            icon={Users}
            active={isRightPanelOpen("members")}
            onClick={() => openRightPanel("members")}
          />
          <NavButton
            label="通知"
            icon={Bell}
            active={isRightPanelOpen("notifications")}
            emphasize={hasUnread}
            onClick={() => openRightPanel("notifications")}
          />
          <NavButton
            label="ハイライト"
            icon={Activity}
            active={isRightPanelOpen("activity")}
            onClick={() => openRightPanel("activity")}
          />
          <NavButton
            label="ピン留め"
            icon={Pin}
            active={isRightPanelOpen("pinned")}
            onClick={() => openRightPanel("pinned")}
          />
          <NavButton
            label="メモ"
            icon={NotebookPen}
            active={isRightPanelOpen("memos")}
            onClick={() => openRightPanel("memos")}
          />
          <NavLink href={settingsHref} label="設定" icon={Settings} active={pathname === settingsHref} />
          <NavButton
            label="ガイド"
            icon={BookOpen}
            active={isRightPanelOpen("guide")}
            onClick={() => openRightPanel("guide")}
          />
          <NavButton
            label="フィードバック"
            icon={MessageSquarePlus}
            active={isRightPanelOpen("feedback")}
            onClick={() => openRightPanel("feedback")}
          />
        </nav>

        <div className="flex items-center gap-2 border-t border-border px-3 py-3">
          <Avatar name={user.name} seed={user.id} />
          <span className="min-w-0 flex-1 truncate text-sm text-foreground">{user.name}</span>
          <button
            onClick={logout}
            title="ログアウト"
            className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-surface-muted hover:text-foreground"
          >
            <LogOut className="h-4 w-4" />
          </button>
        </div>
      </aside>
      <main className="min-w-0 flex-1 pt-14 lg:pt-0">{children}</main>

      {isRightPanelOpen("members") && <MembersPanel onClose={closeRightPanel} />}
      {isRightPanelOpen("notifications") && (
        <NotificationsPanel
          onClose={() => {
            closeRightPanel();
            refreshUnread();
          }}
        />
      )}
      {isRightPanelOpen("activity") && <ActivityPanel onClose={closeRightPanel} />}
      {isRightPanelOpen("pinned") && <PinnedTasksPanel onClose={closeRightPanel} />}
      {isRightPanelOpen("memos") && <MemoListPanel onClose={closeRightPanel} />}
      {isRightPanelOpen("guide") && <GuidePanel onClose={closeRightPanel} />}
      {isRightPanelOpen("feedback") && <FeedbackPanel onClose={closeRightPanel} />}

      <ConfirmDialog
        open={showDeleteWorkspaceConfirm}
        title="ワークスペースを削除しますか？"
        message={`「${workspace?.name ?? ""}」を削除すると、配下の全プロジェクト・タスク・メンバー・添付ファイル等がすべて削除されます。この操作は取り消せません。`}
        error={deleteWorkspaceError}
        onConfirm={handleDeleteWorkspace}
        onCancel={() => setShowDeleteWorkspaceConfirm(false)}
      />
    </div>
  );
}

// プロジェクト一覧の1行。公開設定（Public/Private）は元のページヘッダーと同じ
// Badge表示にする。現在開いているプロジェクトかつ責任者/Ownerの場合のみ、
// 参画メンバー（publicは責任者の付与）・削除ボタンも名前の横に表示する
// （パネル自体はCurrentProjectProviderがレンダーする。ここではトリガーのみ）。
// M8追加：個人ごとの並び順（自分の表示にのみ影響）でD&D並び替え可能にする。
function ProjectNavItem({
  id,
  name,
  visibility,
  href,
  active,
}: {
  id: string;
  name: string;
  visibility: "public" | "private";
  href: string;
  active: boolean;
}) {
  const { isManager, openMembersPanel, openDeleteConfirm } = useCurrentProject();
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id });
  const style = { transform: CSS.Transform.toString(transform), transition };
  return (
    <li ref={setNodeRef} style={style} className={clsx("flex items-center gap-1", isDragging && "opacity-50")}>
      <span
        {...attributes}
        {...listeners}
        className="cursor-grab select-none text-muted-foreground/40 hover:text-muted-foreground"
        aria-label="ドラッグして並び替え"
      >
        <GripVertical className="h-3.5 w-3.5" />
      </span>
      <span
        title={visibility === "private" ? "Private" : "Public"}
        className={clsx(
          "flex h-5 w-5 shrink-0 items-center justify-center rounded-full",
          visibility === "private"
            ? "bg-amber-50 text-amber-700 dark:bg-amber-950/50 dark:text-amber-300"
            : "bg-emerald-50 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300",
        )}
      >
        {visibility === "private" ? <Lock className="h-3 w-3" /> : <Globe className="h-3 w-3" />}
      </span>
      <Link
        href={href}
        title={name}
        className={clsx(
          "block min-w-0 flex-1 truncate rounded-md px-2 py-1.5 text-xs transition-colors",
          active
            ? "font-medium text-indigo-600 dark:text-indigo-400"
            : "text-muted-foreground hover:bg-surface-muted hover:text-foreground",
        )}
      >
        {name}
      </Link>
      {active && isManager && (
        <span className="flex shrink-0 items-center gap-0.5">
          <IconButton size="sm" title="参画メンバー" onClick={openMembersPanel}>
            <Users className="h-3.5 w-3.5" />
          </IconButton>
          <IconButton size="sm" title="プロジェクトを削除" onClick={openDeleteConfirm}>
            <Trash2 className="h-3.5 w-3.5 hover:text-red-600" />
          </IconButton>
        </span>
      )}
    </li>
  );
}

function NavLink({
  href,
  label,
  icon: Icon,
  active,
  emphasize,
}: {
  href: string;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  active: boolean;
  // DMの未読バッジ等、ページ遷移するNavLinkでもNavButtonのemphasizeと同じ
  // 強調表示をしたい場合に使う（M8.5）。
  emphasize?: boolean;
}) {
  return (
    <Link
      href={href}
      className={clsx(
        "flex items-center gap-2.5 rounded-md px-2.5 py-2 text-sm transition-colors",
        active
          ? "bg-indigo-50 font-medium text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-400"
          : emphasize
            ? "font-medium text-foreground hover:bg-surface-muted"
            : "text-muted-foreground hover:bg-surface-muted hover:text-foreground",
      )}
    >
      <Icon className="h-4 w-4 shrink-0" />
      {label}
    </Link>
  );
}

// メンバー・通知用。NavLinkと見た目は同じだがページ遷移せず状態でパネルを開く。
// emphasize（未読通知あり等）の場合は非アクティブ時でも文字色を明るくする。
function NavButton({
  label,
  icon: Icon,
  active,
  emphasize,
  onClick,
}: {
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  active: boolean;
  emphasize?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={clsx(
        "flex items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-sm transition-colors",
        active
          ? "bg-indigo-50 font-medium text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-400"
          : emphasize
            ? "font-medium text-foreground hover:bg-surface-muted"
            : "text-muted-foreground hover:bg-surface-muted hover:text-foreground",
      )}
    >
      <Icon className="h-4 w-4 shrink-0" />
      {label}
    </button>
  );
}
