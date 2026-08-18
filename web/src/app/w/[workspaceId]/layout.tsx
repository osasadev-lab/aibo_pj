"use client";

import Link from "next/link";
import { usePathname, useParams, useRouter } from "next/navigation";
import { useEffect, useState, type ReactNode } from "react";
import { ArrowLeftRight, Bell, Folder, Globe, ListChecks, Lock, LogOut, Trash2, Users } from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";
import Avatar from "@/components/ui/Avatar";
import IconButton from "@/components/ui/IconButton";
import MembersPanel from "@/components/MembersPanel";
import NotificationsPanel from "@/components/NotificationsPanel";
import { ProjectsProvider, useProjects } from "@/lib/workspace/ProjectsContext";
import { CurrentProjectProvider, useCurrentProject } from "@/lib/workspace/CurrentProjectContext";
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
    <ProjectsProvider workspaceId={params.workspaceId}>
      <CurrentProjectProvider workspaceId={params.workspaceId}>
        <WorkspaceLayoutInner>{children}</WorkspaceLayoutInner>
      </CurrentProjectProvider>
    </ProjectsProvider>
  );
}

function WorkspaceLayoutInner({ children }: { children: ReactNode }) {
  const { user, logout } = useAuth();
  const pathname = usePathname();
  const params = useParams<{ workspaceId: string }>();
  const { projects } = useProjects();

  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  // メンバー・通知は別ルートへ遷移せず、現在のページ（プロジェクトのカンバンや
  // マイタスク等）を残したまま右サイドバーに重ねて開く。ページ遷移だと直前の
  // 画面が丸ごとアンマウントされてタスクが見えなくなってしまうため。
  const [showMembers, setShowMembers] = useState(false);
  const [showNotifications, setShowNotifications] = useState(false);

  useEffect(() => {
    if (!params.workspaceId) return;
    apiFetch<Workspace>(`/workspaces/${params.workspaceId}`)
      .then(setWorkspace)
      .catch(() => setWorkspace(null));
  }, [params.workspaceId]);

  if (!user) return null;

  const projectsHref = `/w/${params.workspaceId}/projects`;
  const myTasksHref = `/w/${params.workspaceId}/my-tasks`;

  return (
    <div className="flex min-h-screen bg-background">
      <aside className="flex w-64 shrink-0 flex-col border-r border-border bg-surface">
        <Link
          href="/workspaces"
          className="group flex items-center gap-2 border-b border-border px-4 py-4 transition-colors hover:bg-surface-muted"
        >
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-indigo-600 text-sm font-semibold text-white">
            {workspace?.name?.slice(0, 1) ?? "…"}
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-semibold text-foreground">{workspace?.name ?? "..."}</span>
            <span className="flex items-center gap-1 text-xs text-muted-foreground">
              <ArrowLeftRight className="h-3 w-3" />
              切替
            </span>
          </span>
        </Link>

        <nav className="flex flex-1 flex-col gap-0.5 overflow-y-auto px-3 py-3">
          <NavLink href={projectsHref} label="プロジェクト" icon={Folder} active={pathname === projectsHref} />
          {projects.length > 0 && (
            <ul className="mb-1 ml-3.5 flex flex-col gap-0.5 border-l border-border pl-3">
              {projects.map((p) => {
                const href = `${projectsHref}/${p.id}`;
                const active = pathname === href;
                return (
                  <ProjectNavItem
                    key={p.id}
                    name={p.name}
                    visibility={p.visibility}
                    href={href}
                    active={active}
                  />
                );
              })}
            </ul>
          )}
          <NavLink
            href={myTasksHref}
            label="マイタスク"
            icon={ListChecks}
            active={pathname === myTasksHref || pathname.startsWith(`${myTasksHref}/`)}
          />
          <NavButton label="メンバー" icon={Users} active={showMembers} onClick={() => setShowMembers(true)} />
          <NavButton
            label="通知"
            icon={Bell}
            active={showNotifications}
            onClick={() => setShowNotifications(true)}
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
      <main className="min-w-0 flex-1">{children}</main>

      {showMembers && <MembersPanel onClose={() => setShowMembers(false)} />}
      {showNotifications && <NotificationsPanel onClose={() => setShowNotifications(false)} />}
    </div>
  );
}

// プロジェクト一覧の1行。公開設定（Public/Private）は元のページヘッダーと同じ
// Badge表示にする。現在開いているプロジェクトかつ責任者/Ownerの場合のみ、
// 参画メンバー（publicは責任者の付与）・削除ボタンも名前の横に表示する
// （パネル自体はCurrentProjectProviderがレンダーする。ここではトリガーのみ）。
function ProjectNavItem({
  name,
  visibility,
  href,
  active,
}: {
  name: string;
  visibility: "public" | "private";
  href: string;
  active: boolean;
}) {
  const { isManager, openMembersPanel, openDeleteConfirm } = useCurrentProject();
  return (
    <li className="flex items-center gap-1">
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
}: {
  href: string;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  active: boolean;
}) {
  return (
    <Link
      href={href}
      className={clsx(
        "flex items-center gap-2.5 rounded-md px-2.5 py-2 text-sm transition-colors",
        active
          ? "bg-indigo-50 font-medium text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-400"
          : "text-muted-foreground hover:bg-surface-muted hover:text-foreground",
      )}
    >
      <Icon className="h-4 w-4 shrink-0" />
      {label}
    </Link>
  );
}

// メンバー・通知用。NavLinkと見た目は同じだがページ遷移せず状態でパネルを開く。
function NavButton({
  label,
  icon: Icon,
  active,
  onClick,
}: {
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  active: boolean;
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
          : "text-muted-foreground hover:bg-surface-muted hover:text-foreground",
      )}
    >
      <Icon className="h-4 w-4 shrink-0" />
      {label}
    </button>
  );
}
