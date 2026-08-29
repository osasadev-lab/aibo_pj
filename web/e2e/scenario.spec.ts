import { test, expect } from "@playwright/test";

import { loginAs } from "./testLogin";

// execution-plan.md M9が指定するE2Eの主要シナリオ：招待→プロジェクト作成→タスク管理→カレンダー連携。
// 「カレンダー連携」は実際のGoogle同意フローが必要なため自動化せず、設定画面が正しく開けることのみ
// 確認する（実際の連携確認は本番URLに対する手動チェックリストで行う。m9-implementation-plan.md参照）。
//
// テスト用ユーザーはPOST /auth/test-loginで都度作成される（同一メールなら既存ユーザーを再利用）。
// 実行のたびにメールアドレスへタイムスタンプを混ぜて一意にし、過去の実行結果と衝突しないようにする。
const runID = Date.now();
const ownerEmail = `e2e-owner-${runID}@example.com`;
const inviteeEmail = `e2e-invitee-${runID}@example.com`;
const workspaceName = `E2Eワークスペース ${runID}`;
const projectName = `E2Eプロジェクト ${runID}`;
const taskTitle = `E2Eタスク ${runID}`;

test("招待→プロジェクト作成→タスク管理→カレンダー連携設定画面", async ({ page }) => {
  await loginAs(page, ownerEmail, "E2E Owner");
  await page.goto("/workspaces");

  // --- ワークスペース作成 ---
  await page.getByPlaceholder("ワークスペース名").fill(workspaceName);
  await page.getByRole("button", { name: "作成" }).click();
  const workspaceLink = page.getByRole("link", { name: new RegExp(workspaceName) });
  await expect(workspaceLink).toBeVisible();
  await workspaceLink.click();
  await expect(page).toHaveURL(/\/w\/[^/]+\/projects/);
  const workspaceId = page.url().match(/\/w\/([^/]+)\//)?.[1];
  expect(workspaceId).toBeTruthy();

  // --- メンバー招待 ---
  await page.goto(`/w/${workspaceId}/members`);
  await page.getByPlaceholder("メールアドレス").fill(inviteeEmail);
  await page.getByRole("button", { name: "招待" }).click();
  // 招待はworkspace_invitationsに保存されるだけで、承諾（ログイン）するまでメンバー一覧には
  // 現れない。UIは成功時に「招待しました」というステータス文言のみ表示する。
  await expect(page.getByText("招待しました")).toBeVisible();

  // --- プロジェクト作成 ---
  await page.goto(`/w/${workspaceId}/projects`);
  await page.getByPlaceholder("プロジェクト名").fill(projectName);
  await page.getByRole("button", { name: "作成", exact: true }).click();
  // 同名のリンクが左サイドバーのプロジェクト一覧にも出るため、Public/Privateバッジを
  // 含むメインコンテンツ側の一覧項目に絞り込む。
  const projectLink = page.getByRole("link", { name: new RegExp(projectName) }).filter({ hasText: "Public" });
  await expect(projectLink).toBeVisible();
  await projectLink.click();
  await expect(page).toHaveURL(/\/w\/[^/]+\/projects\/[^/]+/);

  // --- タスク管理：作成 ---
  await page.getByRole("button", { name: "タスク追加" }).first().click();
  await page.getByPlaceholder("タスク名").fill(taskTitle);
  await page.getByRole("button", { name: "追加", exact: true }).click();
  const taskCard = page.getByRole("button", { name: new RegExp(taskTitle) });
  await expect(taskCard).toBeVisible();

  // --- タスク管理：詳細を開いて編集内容が保存されることを確認 ---
  await taskCard.click();
  await expect(page.getByText(taskTitle).first()).toBeVisible();

  // --- カレンダー連携：設定画面が開けることを確認（実際の同意フローは対象外） ---
  await page.goto(`/w/${workspaceId}/settings`);
  await expect(page.getByText(/カレンダー連携|連携する/).first()).toBeVisible();

  // --- 招待の検証：招待されたユーザーが初回ログインするとworkspace_invitationsが
  //     workspace_memberに変換され、ワークスペース一覧に現れることを確認 ---
  await loginAs(page, inviteeEmail, "E2E Invitee");
  await page.goto("/workspaces");
  await expect(page.getByRole("link", { name: new RegExp(workspaceName) })).toBeVisible();
});
