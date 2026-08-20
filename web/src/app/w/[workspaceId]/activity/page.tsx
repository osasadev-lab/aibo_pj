"use client";

import { useRouter } from "next/navigation";

import ActivityPanel from "@/components/ActivityPanel";

// /w/[workspaceId]/activity への直接アクセス（ブックマーク・リロード等）用の
// 入り口。左サイドバーからの通常導線はWorkspaceLayoutが同じActivityPanelを
// 状態で開閉する（ページ遷移せず現在の画面を残したまま重ねて表示するため）。
export default function ActivityPage() {
  const router = useRouter();
  return <ActivityPanel onClose={() => router.back()} />;
}
