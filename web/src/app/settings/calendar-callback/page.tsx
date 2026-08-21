"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense } from "react";

// Googleカレンダー連携の同意フロー（M6）の戻り先。ワークスペースIDを跨がない
// 経路のため（docs/aibo/m6-implementation-plan.md参照）、どのワークスペースの
// 設定画面から連携を開始したかをここでは特定できない。結果だけ表示し、
// ワークスペース一覧への導線を出す軽量な中継ページ。
function CalendarCallbackInner() {
  const searchParams = useSearchParams();
  const connected = searchParams.get("connected") === "1";
  const error = searchParams.get("error") === "1";

  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-background px-4 text-center">
      <p className="text-sm text-foreground">
        {connected && "Googleカレンダーとの連携が完了しました。"}
        {error && "Googleカレンダーとの連携に失敗しました。もう一度お試しください。"}
        {!connected && !error && "処理結果を確認できませんでした。"}
      </p>
      <p className="text-xs text-muted-foreground">設定画面に戻ってご確認ください。</p>
      <Link href="/workspaces" className="text-sm text-indigo-600 hover:underline dark:text-indigo-400">
        ワークスペース一覧へ戻る
      </Link>
    </div>
  );
}

export default function CalendarCallbackPage() {
  return (
    <Suspense fallback={null}>
      <CalendarCallbackInner />
    </Suspense>
  );
}
