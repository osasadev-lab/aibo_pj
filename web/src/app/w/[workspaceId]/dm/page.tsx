"use client";

import { useCallback, useEffect, useState } from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";

import { apiFetch } from "@/lib/apiClient";
import { getSupabaseClient } from "@/lib/supabaseClient";
import DMChannelList, { type DMChannel } from "@/components/dm/DMChannelList";
import DMThread from "@/components/dm/DMThread";
import NewDMDialog from "@/components/dm/NewDMDialog";

// DM画面（M8.5）。カレンダー/ガントチャートと同じ専用ページ方式
// （グループのメンバー表示・添付プレビュー等で画面幅が必要なため、
// docs/aibo/m8.5-implementation-plan.md）。左：チャンネル一覧、右：スレッド。
// lg未満では選択中のチャンネルが無ければ一覧のみ、選べば一覧を隠しスレッドのみ表示する。
export default function DMPage() {
  const params = useParams<{ workspaceId: string }>();
  const workspaceId = params.workspaceId;
  const router = useRouter();
  const searchParams = useSearchParams();
  const selectedId = searchParams.get("channel");

  const [channels, setChannels] = useState<DMChannel[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [showNewDM, setShowNewDM] = useState(false);

  const loadChannels = useCallback(() => {
    if (!workspaceId) return;
    apiFetch<DMChannel[]>(`/workspaces/${workspaceId}/dm/channels`)
      .then(setChannels)
      .catch(() => setError("チャンネル一覧の取得に失敗しました"));
  }, [workspaceId]);

  useEffect(() => {
    loadChannels();
  }, [loadChannels]);

  // 他チャンネルの新着で一覧の並び順・未読表示を更新する
  // （選択中チャンネルの新着はDMThread側のonChannelUpdatedでも同じ関数を呼ぶ）。
  useEffect(() => {
    const supabase = getSupabaseClient();
    const channel = supabase
      .channel(`dm_channel_list:${workspaceId}`)
      .on("postgres_changes", { event: "INSERT", schema: "public", table: "dm_messages" }, () => loadChannels())
      .subscribe();
    return () => {
      void supabase.removeChannel(channel);
    };
  }, [workspaceId, loadChannels]);

  function selectChannel(id: string) {
    router.push(`/w/${workspaceId}/dm?channel=${id}`);
  }

  function handleCreated(id: string) {
    setShowNewDM(false);
    loadChannels();
    selectChannel(id);
  }

  return (
    <div className="flex h-[calc(100dvh-3.5rem)] lg:h-dvh">
      {error && (
        <p className="absolute left-1/2 top-2 z-10 -translate-x-1/2 rounded-lg bg-red-50 px-3 py-1.5 text-sm text-red-600 dark:bg-red-950/40 dark:text-red-400">
          {error}
        </p>
      )}
      <div className={selectedId ? "hidden lg:flex lg:w-80 lg:shrink-0" : "flex w-full lg:w-80 lg:shrink-0"}>
        <DMChannelList
          channels={channels}
          selectedId={selectedId}
          onSelect={selectChannel}
          onNewDM={() => setShowNewDM(true)}
        />
      </div>
      <div className={selectedId ? "flex min-w-0 flex-1" : "hidden lg:flex lg:min-w-0 lg:flex-1"}>
        {selectedId ? (
          <DMThread
            key={selectedId}
            workspaceId={workspaceId}
            channelId={selectedId}
            onLeft={() => {
              loadChannels();
              router.push(`/w/${workspaceId}/dm`);
            }}
            onChannelUpdated={loadChannels}
            onBack={() => router.push(`/w/${workspaceId}/dm`)}
          />
        ) : (
          <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
            チャンネルを選択してください
          </div>
        )}
      </div>

      {showNewDM && (
        <NewDMDialog workspaceId={workspaceId} onClose={() => setShowNewDM(false)} onCreated={handleCreated} />
      )}
    </div>
  );
}
