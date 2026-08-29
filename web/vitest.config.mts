import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "node:path";

const dirname = import.meta.dirname;

// フロントエンドの自動テスト基盤（2026-08-27追加）。Next.js本体のビルド設定とは
// 独立させ、pure-logic/コンポーネント単体テストの実行にのみ使う。
// E2E（web/e2e/、M9追加）はPlaywright専用のtest()を使うため明示的に除外する
// （除外しないとvitestが自身のtest()として誤って収集し衝突する）。
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.mts"],
    globals: true,
    css: false,
    exclude: ["node_modules/**", "e2e/**"],
  },
  resolve: {
    alias: {
      "@": path.resolve(dirname, "./src"),
    },
  },
});
