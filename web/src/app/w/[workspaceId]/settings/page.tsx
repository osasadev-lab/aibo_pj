"use client";

import { useParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";

import { apiFetch } from "@/lib/apiClient";
import HoverSettingsSection from "@/components/settings/HoverSettingsSection";
import CalendarSyncSection from "@/components/settings/CalendarSyncSection";
import TagSection from "@/components/settings/TagSection";
import { Select } from "@/components/ui/fields";
import { useProjects } from "@/lib/workspace/ProjectsContext";
import type { Workspace } from "@/lib/types";

const COMMON_TARGET = "common";

// 左サイドバー「設定」から開く実ページ。タグ管理（Owner=共通タグ＋全プロジェクトの
// タグ、プロジェクト責任者=自分が責任者のプロジェクトのタグのみ）を扱う。
// スタッフ（どのプロジェクトの責任者でもない一般メンバー）には管理対象が無いため
// 空表示になる。
//
// プロジェクト数が増えるとタグ区画を全部並べるとスペースを取りすぎるため、
// プルダウンで対象（共通タグ or 各プロジェクト）を選び、選択中の1区画だけを
// 表示する方式にしている（ユーザー要望）。
export default function SettingsPage() {
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;
  const { projects } = useProjects();

  const [workspace, setWorkspace] = useState<Workspace | null>(null);

  useEffect(() => {
    if (!workspaceId) return;
    apiFetch<Workspace>(`/workspaces/${workspaceId}`)
      .then(setWorkspace)
      .catch(() => {});
  }, [workspaceId]);

  const isOwner = workspace?.role === "owner";
  const managedProjects = useMemo(() => projects.filter((p) => p.is_manager), [projects]);

  const targets = useMemo(
    () => [
      ...(isOwner ? [{ id: COMMON_TARGET, label: "共通タグ（ワークスペース全体）" }] : []),
      ...managedProjects.map((p) => ({ id: p.id, label: `${p.name} のタグ` })),
    ],
    [isOwner, managedProjects],
  );

  const [selectedTarget, setSelectedTarget] = useState<string>("");
  const activeTarget = targets.some((t) => t.id === selectedTarget) ? selectedTarget : (targets[0]?.id ?? "");

  return (
    <div className="flex max-w-3xl flex-col gap-6 px-6 py-8 lg:px-10">
      <h1 className="text-2xl font-semibold tracking-tight text-foreground">設定</h1>

      <div className="flex flex-col gap-4">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">個人設定</h2>
        <HoverSettingsSection />
        <CalendarSyncSection />
      </div>

      {targets.length > 0 && (
        <div className="flex flex-col gap-4">
          <h2 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">タグ管理</h2>

          <Select
            value={activeTarget}
            onChange={(e) => setSelectedTarget(e.target.value)}
            className="max-w-sm"
          >
            {targets.map((t) => (
              <option key={t.id} value={t.id}>
                {t.label}
              </option>
            ))}
          </Select>

          {activeTarget === COMMON_TARGET ? (
            <TagSection
              key={COMMON_TARGET}
              title="共通タグ（ワークスペース全体）"
              listUrl={`/workspaces/${workspaceId}/common-tags`}
              createUrl={`/workspaces/${workspaceId}/common-tags`}
              updateUrl={(tagId) => `/workspaces/${workspaceId}/common-tags/${tagId}`}
              deleteUrl={(tagId) => `/workspaces/${workspaceId}/common-tags/${tagId}`}
            />
          ) : (
            activeTarget && (
              <TagSection
                key={activeTarget}
                title={targets.find((t) => t.id === activeTarget)?.label ?? ""}
                listUrl={`/projects/${activeTarget}/tags`}
                createUrl={`/projects/${activeTarget}/tags`}
                updateUrl={(tagId) => `/projects/${activeTarget}/tags/${tagId}`}
                deleteUrl={(tagId) => `/projects/${activeTarget}/tags/${tagId}`}
              />
            )
          )}
        </div>
      )}
    </div>
  );
}
