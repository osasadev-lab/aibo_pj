# aibo

Asana的なUIを持つ、チーム/組織向けタスク管理サービス。設計ドキュメントは `docs/aibo`（別リポジトリ管理）を参照：spec.md / db-schema.md / api-spec.md / execution-plan.md。

## 構成（monorepo）

```
server/   Go (Gin) + ent。REST API。Cloud Runにデプロイ
web/      Next.js（App Router）。Cloudflare Workersにデプロイ（@opennextjs/cloudflare使用）
```

spec.mdでは「Cloudflare Pages」としているが、Next.js 16はPages向けの旧アダプタ
（`@cloudflare/next-on-pages`）が未対応のため、Cloudflareが現在推奨している
Workers向けアダプタ（`@opennextjs/cloudflare`）に切り替えている（デプロイ先は
Pagesダッシュボードではなく Workersダッシュボードになる）。

## 現在の状態

### M0：開発基盤構築

- [x] monorepo構成（server/, web/）
- [x] Go + Ginの最小サーバー（`/health`, `/api/v1/ping`）
- [x] entスキーマ定義（db-schema.md準拠、全18テーブル）＋マイグレーション実行コマンド
- [x] Next.jsの最小ページ
- [x] Dockerfile（server、Cloud Run向け）
- [x] CI（lint/test/build）のGitHub Actionsワークフロー
- [x] 実際のCloud Run / Cloudflare Workersへのデプロイ検証（現在はコスト都合で一時停止中。詳細は`docs/aibo/deploy-guide.md`参照）

### M1：認証・ワークスペース

- [x] Google OAuth（Authorization Code Flow）＋JWTセッション発行
- [x] `RequireAuth` / `RequireWorkspaceMember` / `RequireOwner` ミドルウェア
- [x] ワークスペース作成、メンバー招待・一覧・ロール変更・削除（Owner保護ルール含む）
- [x] フロント：ログイン画面、ワークスペース作成/切り替えUI、メンバー一覧画面
- 実装詳細・設計判断は`docs/aibo/m1-implementation-notes.md`参照

### M2：プロジェクト／タスクのコアCRUD

- [x] プロジェクトCRUD（公開設定public/private、参画メンバー必須、既定4ステータス列の自動生成、列のカスタム編集・最大5列）
- [x] タスクCRUD（単体タスク・プロジェクト所属タスク両方、担当者複数アサイン、タグ、優先度、期限）
- [x] サブタスク作成（1階層制限）
- [x] `RequireProjectAccess` / `RequireTaskAccess` ミドルウェア
- [x] フロント：プロジェクト一覧/作成、プロジェクト詳細（列・タスク・子タスク管理）
- タスク依存関係はM4に先送り（execution-plan.md M4に明記されているため）
- 実装詳細・設計判断は`docs/aibo/m2-implementation-plan.md`参照

## セットアップ

### 前提

- Go 1.26+
- Node.js 24+
- Supabase側で作成済みのプロジェクト（DB接続情報が必要）

### server/

```sh
cd server
cp .env.example .env   # DATABASE_URL・JWT_SECRET・GOOGLE_OAUTH_*・FRONTEND_URL を設定する
go run ./cmd/migrate   # entスキーマをDBに適用（テーブル作成 + pgvector拡張の有効化）
go run ./cmd/server    # http://localhost:8080
```

Googleログインを試すには、GCPコンソールでOAuth 2.0 WebクライアントIDを作成し、
`GOOGLE_OAUTH_CLIENT_ID` / `GOOGLE_OAUTH_CLIENT_SECRET` に設定する必要がある
（承認済みリダイレクトURIに`GOOGLE_OAUTH_REDIRECT_URL`と同じ値を登録すること）。

ent スキーマを変更したら `go generate ./ent` でコード再生成してから `go run ./cmd/migrate` を実行する。

### web/

```sh
cd web
cp .env.example .env.local
npm install
npm run dev   # http://localhost:3000
```

## デプロイ

`.github/workflows/deploy-server.yml`（Cloud Run）、`.github/workflows/deploy-web.yml`（Cloudflare Workers）はどちらも `workflow_dispatch`（手動トリガー）のみで、事前にリポジトリのSecretsを設定してから実行する。詳細は各ワークフローファイル冒頭のコメントを参照。

web/ をローカルでCloudflare向けにビルド・プレビューする場合：

```sh
cd web
npm run preview   # ビルド + wranglerローカルプレビュー
npm run deploy    # ビルド + Cloudflare Workersへ実デプロイ
```

## 次のマイルストーン

M3（カンバン）以降は `docs/aibo/execution-plan.md` を参照。
