import type { Page } from "@playwright/test";

// server側 POST /auth/test-login（M9追加、ALLOW_TEST_LOGIN設定時のみ有効）を叩いてJWTを取得し、
// AuthContext（web/src/lib/auth/AuthContext.tsx）が読むlocalStorageキー"aibo_token"に
// 直接書き込む。Google OAuthの同意画面はbot対策等で安定して自動化できないため、
// このバイパスでログイン状態を作る（本番環境には絶対に向けない。使い方はplaywright.config.ts参照）。
const API_BASE_URL = process.env.PLAYWRIGHT_API_BASE_URL ?? "http://localhost:8080/api/v1";
const TEST_LOGIN_SECRET = process.env.E2E_TEST_LOGIN_SECRET;

export async function loginAs(page: Page, email: string, name: string): Promise<void> {
  if (!TEST_LOGIN_SECRET) {
    throw new Error("E2E_TEST_LOGIN_SECRET is not set (must match server's ALLOW_TEST_LOGIN)");
  }

  const res = await page.request.post(`${API_BASE_URL}/auth/test-login`, {
    headers: { "X-Test-Login-Secret": TEST_LOGIN_SECRET },
    data: { email, name },
  });
  if (!res.ok()) {
    throw new Error(`test-login failed: ${res.status()} ${await res.text()}`);
  }
  const { token } = (await res.json()) as { token: string };

  // localStorageはページのオリジンに紐づくため、書き込み前に一度そのオリジンへ遷移しておく。
  await page.goto("/login");
  await page.evaluate((t) => window.localStorage.setItem("aibo_token", t), token);
}
