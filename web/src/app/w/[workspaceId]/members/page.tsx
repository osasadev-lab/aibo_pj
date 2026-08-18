"use client";

import { useRouter } from "next/navigation";

import MembersPanel from "@/components/MembersPanel";

// /w/[workspaceId]/members への直接アクセス（ブックマーク・リロード等）用の入り口。
// 左サイドバーからの通常導線はWorkspaceLayoutが同じMembersPanelを状態で開閉する
// （ページ遷移せず現在の画面を残したまま重ねて表示するため）。
export default function MembersPage() {
  const router = useRouter();
  return <MembersPanel onClose={() => router.back()} />;
}
