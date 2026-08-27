"use client";

import { WifiOff } from "lucide-react";

import Button from "@/components/ui/Button";

// Service Workerがネットワーク不通時のナビゲーションフォールバック先として
// インストール時にプリキャッシュする最小ページ（M8、docs/aibo/m8-implementation-plan.md）。
export default function OfflinePage() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-background px-6 text-center">
      <WifiOff className="h-10 w-10 text-muted-foreground" />
      <div>
        <h1 className="text-lg font-semibold text-foreground">オフラインです</h1>
        <p className="mt-1 text-sm text-muted-foreground">ネットワーク接続を確認してください</p>
      </div>
      <Button variant="primary" onClick={() => window.location.reload()}>
        再読み込み
      </Button>
    </div>
  );
}
