"use client";

import { useState, type KeyboardEvent } from "react";
import { useParams, useRouter } from "next/navigation";
import { Search } from "lucide-react";

import { Input } from "@/components/ui/fields";

// 左サイドバー常時表示の検索入力欄（M7）。検索ボタンは設けず、Enterキー押下時のみ
// 検索結果ページ（/w/:workspaceId/search?q=...）へ遷移する（ユーザー確認済みの
// 明示的要件）。検索結果ページ自体には入力欄を置かない（ユーザーフィードバックにより
// 撤回、検索の実行はここ1箇所に統一する）。
export default function SearchInput() {
  const router = useRouter();
  const params = useParams<{ workspaceId: string }>();
  const [value, setValue] = useState("");

  function handleKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key !== "Enter") return;
    const trimmed = value.trim();
    if (!trimmed) return;
    router.push(`/w/${params.workspaceId}/search?q=${encodeURIComponent(trimmed)}`);
  }

  return (
    <div className="relative">
      <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
      <Input
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="検索"
        className="pl-8"
      />
    </div>
  );
}
