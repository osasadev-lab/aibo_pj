"use client";

import { BookOpen, Download } from "lucide-react";

import SidePanel from "@/components/ui/SidePanel";

// 取扱説明書（PDF）のダウンロード導線（左サイドバー「ガイド」から開く右サイドパネル、M8.5）。
// PDF本体は静的ファイル（web/public/guide/aisu-guide.pdf）として配置済み。
// アプリの依存関係にPDF生成ライブラリは追加していない
// （docs/aibo/m8.5-implementation-plan.md）。
export default function GuidePanel({ onClose }: { onClose: () => void }) {
  return (
    <SidePanel title="ガイド" onClose={onClose}>
      <div className="flex flex-col items-center gap-4 py-10 text-center">
        <BookOpen className="h-8 w-8 text-indigo-500" />
        <div>
          <p className="text-sm font-medium text-foreground">aisu 取扱説明書</p>
          <p className="mt-1 text-sm text-muted-foreground">
            プロジェクト・タスク管理からDM、カレンダー連携、PWAの使い方まで、
            <br />
            はじめて使う方向けにまとめています。
          </p>
        </div>
        <a
          href="/guide/aisu-guide.pdf"
          download="aisu-guide.pdf"
          className="inline-flex shrink-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-lg bg-indigo-600 px-3.5 py-2 text-sm font-medium text-white shadow-sm shadow-indigo-600/20 transition-colors hover:bg-indigo-500 dark:bg-indigo-500 dark:hover:bg-indigo-400"
        >
          <Download className="h-3.5 w-3.5" />
          ガイドをダウンロード（PDF）
        </a>
      </div>
    </SidePanel>
  );
}
