"use client";

import { useRouter } from "next/navigation";

import NotificationsPanel from "@/components/NotificationsPanel";

// /w/[workspaceId]/notifications への直接アクセス（ブックマーク・リロード等）用の
// 入り口。左サイドバーからの通常導線はWorkspaceLayoutが同じNotificationsPanelを
// 状態で開閉する（ページ遷移せず現在の画面を残したまま重ねて表示するため）。
export default function NotificationsPage() {
  const router = useRouter();
  return <NotificationsPanel onClose={() => router.back()} />;
}
