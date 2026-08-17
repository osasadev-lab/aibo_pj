"use client";

import Link from "next/link";
import { usePathname, useParams, useRouter } from "next/navigation";
import { useEffect, useState, type ReactNode } from "react";
import { ArrowLeftRight, Bell, Folder, ListChecks, LogOut, Users } from "lucide-react";
import clsx from "clsx";

import { apiFetch } from "@/lib/apiClient";
import { useAuth } from "@/lib/auth/useAuth";
import Avatar from "@/components/ui/Avatar";

type Workspace = {
  id: string;
  name: string;
  role: "owner" | "member";
};

type Project = {
  id: string;
  name: string;
};

export default function WorkspaceLayout({ children }: { children: ReactNode }) {
  const { user, loading, logout } = useAuth();
  const router = useRouter();
  const pathname = usePathname();
  const params = useParams<{ workspaceId: string }>();

  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);

  useEffect(() => {
    if (!loading && !user) {
      router.replace("/login");
    }
  }, [loading, user, router]);

  useEffect(() => {
    if (!user || !params.workspaceId) return;
    apiFetch<Workspace>(`/workspaces/${params.workspaceId}`)
      .then(setWorkspace)
      .catch(() => setWorkspace(null));
    apiFetch<Project[]>(`/workspaces/${params.workspaceId}/projects`)
      .then(setProjects)
      .catch(() => setProjects([]));
  }, [user, params.workspaceId]);

  if (loading || !user) {
    return (
      <div className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">
        読み込み中...
      </div>
    );
  }

  const projectsHref = `/w/${params.workspaceId}/projects`;
  const navItems = [
    { href: `/w/${params.workspaceId}/my-tasks`, label: "マイタスク", icon: ListChecks },
    { href: `/w/${params.workspaceId}/members`, label: "メンバー", icon: Users },
    { href: `/w/${params.workspaceId}/notifications`, label: "通知", icon: Bell },
  ];

  return (
    <div className="flex min-h-screen bg-background">
      <aside className="flex w-60 shrink-0 flex-col border-r border-border bg-surface">
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
                  <li key={p.id}>
                    <Link
                      href={href}
                      title={p.name}
                      className={clsx(
                        "block truncate rounded-md px-2 py-1.5 text-xs transition-colors",
                        active
                          ? "font-medium text-indigo-600 dark:text-indigo-400"
                          : "text-muted-foreground hover:bg-surface-muted hover:text-foreground",
                      )}
                    >
                      {p.name}
                    </Link>
                  </li>
                );
              })}
            </ul>
          )}
          {navItems.map((item) => (
            <NavLink
              key={item.href}
              href={item.href}
              label={item.label}
              icon={item.icon}
              active={pathname === item.href || pathname.startsWith(`${item.href}/`)}
            />
          ))}
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
    </div>
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
