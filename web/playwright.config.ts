import { defineConfig, devices } from "@playwright/test";

// M9で追加したE2Eテスト設定。ローカル/CIで起動したweb（Next.js）・server（Go）に対して実行する
// 想定で、本番環境そのものには向けない（本番確認は手動チェックリストで行う。
// docs/aibo/m9-implementation-plan.md参照）。
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? "dot" : "list",
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3000",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
