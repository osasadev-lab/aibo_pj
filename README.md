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

## 現在の状態（M0：開発基盤構築）

- [x] monorepo構成（server/, web/）
- [x] Go + Ginの最小サーバー（`/health`, `/api/v1/ping`）
- [x] entスキーマ定義（db-schema.md準拠、全18テーブル）＋マイグレーション実行コマンド
- [x] Next.jsの最小ページ
- [x] Dockerfile（server、Cloud Run向け）
- [x] CI（lint/test/build）のGitHub Actionsワークフロー
- [ ] 実際のCloud Run / Cloudflare Workersへのデプロイ（デプロイワークフローは用意済み、実行は手動）

## セットアップ

### 前提

- Go 1.26+
- Node.js 24+
- Supabase側で作成済みのプロジェクト（DB接続情報が必要）

### server/

```sh
cd server
cp .env.example .env   # DATABASE_URL を実際の接続文字列に書き換える
go run ./cmd/migrate   # entスキーマをDBに適用（テーブル作成 + pgvector拡張の有効化）
go run ./cmd/server    # http://localhost:8080
```

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

M1（認証・ワークスペース）以降は `docs/aibo/execution-plan.md` を参照。
