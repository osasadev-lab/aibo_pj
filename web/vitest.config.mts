import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "node:path";

const dirname = import.meta.dirname;

// フロントエンドの自動テスト基盤（2026-08-27追加）。Next.js本体のビルド設定とは
// 独立させ、pure-logic/コンポーネント単体テストの実行にのみ使う（E2Eはスコープ外）。
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.mts"],
    globals: true,
    css: false,
  },
  resolve: {
    alias: {
      "@": path.resolve(dirname, "./src"),
    },
  },
});
