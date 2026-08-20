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

### M3：カンバン

- [x] プロジェクトKanban・マイタスクのドラッグ&ドロップUI（`@dnd-kit`。status⇄status_column_idの同期ロジック自体はM2実装済み）
- [x] コメント（リアルタイムチャット）：Supabase Realtime + 自作JWTブリッジ（`GET /me/supabase-token`）で実装。RLSポリシーで閲覧権限をSupabase側でも強制
- [x] ActivityLog記録（Task/ProjectのCRUD、担当者・タグ変更、コメント作成。M1のworkspace/member操作は対象外）
- [x] `@`メンション・通知（メンション時に`notifications`作成、`/notifications`画面で確認・既読化）
- 実装詳細・設計判断・スモークテスト結果は`docs/aibo/m3-implementation-plan.md`参照

### M4：依存関係・タグ・添付ファイル

- [x] タスク依存関係（先行タスク設定、循環依存チェック、同一ワークスペース内チェック、未完了バッジ・確認ダイアログ）
- [x] タグ管理（プロジェクト専用タグ／ワークスペース共通タグ、責任者・Owner限定のCRUD、タスクへの付与は全メンバー可）・左サイドバー「設定」画面
- [x] 添付ファイル（Cloudflare R2、署名付きURLアップロード、25MB上限チェック。R2未設定時はアップロードUI自体を非表示にしグレースフルに動作）
- [x] ワークスペース削除機能（元々未実装だったため新規実装。Owner限定）、タスク/プロジェクト/ワークスペース削除時のR2オブジェクト連動削除
- [x] 追加UI：子タスクのカンバン表示、Slack風マークダウンツールバー（説明・コメント）、タスク詳細のリンクコピー、ホバー強調（タグ一致/依存関係/親子関係、個人設定）
- 実装詳細・設計判断は`docs/aibo/m4-implementation-plan.md`参照

### M5：カレンダー画面・進捗画面

- [x] 月タイル表示カレンダー（既定は自分のタスクのみ、他メンバーを追加すると重ねて表示。`calendar_watched_members`テーブル）
- [x] 進捗画面：プロジェクト別・担当者別の棒グラフ（自前実装、外部チャートライブラリ不使用）。Supabase Realtimeで`tasks`テーブルを購読しステータス変更にリアルタイム追随
- [x] メンバー画面のタスクドリルダウン（`GET /workspaces/:id/members/:member_id/tasks`、進捗画面に`?member_id=`で遷移）
- [x] ハイライト機能（プロジェクト/タスクの作成・削除・変更・ステータス変更をactorで絞り込める右サイドバーパネル。追加要望）
- 実装詳細・設計判断は`docs/aibo/m5-implementation-plan.md`参照（Supabase側の`tasks`テーブルRLS/Realtime publication設定も適用済み）

## セットアップ

### 前提

- Go 1.26+
- Node.js 24+
- Supabase側で作成済みのプロジェクト（DB接続情報が必要）

### server/

```sh
cd server
cp .env.example .env   # DATABASE_URL・JWT_SECRET・GOOGLE_OAUTH_*・FRONTEND_URL・SUPABASE_JWT_SECRET を設定する
go run ./cmd/migrate   # entスキーマをDBに適用（テーブル作成 + pgvector拡張の有効化）
go run ./cmd/server    # http://localhost:8080
```

Googleログインを試すには、GCPコンソールでOAuth 2.0 WebクライアントIDを作成し、
`GOOGLE_OAUTH_CLIENT_ID` / `GOOGLE_OAUTH_CLIENT_SECRET` に設定する必要がある
（承認済みリダイレクトURIに`GOOGLE_OAUTH_REDIRECT_URL`と同じ値を登録すること）。

コメントのリアルタイム反映（M3）を試すには、Supabaseダッシュボード → Settings → JWT Keys →
「Legacy JWT Secret」を`SUPABASE_JWT_SECRET`に設定する必要がある（このアプリ自身の
`JWT_SECRET`とは別物。詳細は`docs/aibo/m3-implementation-plan.md`参照）。

ent スキーマを変更したら `go generate ./ent` でコード再生成してから `go run ./cmd/migrate` を実行する。

添付ファイル機能（M4）を試すには、Cloudflare R2でバケットを作成しAccount API Tokenを発行して
`R2_ACCOUNT_ID` / `R2_ACCESS_KEY_ID` / `R2_SECRET_ACCESS_KEY` / `R2_BUCKET_NAME` を設定する
必要がある（ブラウザから直接R2へPUTするため、バケットのCORS設定でフロントのオリジンからの
PUT/GETを許可すること）。**未設定でもサーバーは起動し、他機能は問題なく動作する**（アップロード
UI自体が表示されなくなるだけ。詳細は`docs/aibo/m4-implementation-plan.md`参照）。

### web/

```sh
cd web
cp .env.example .env.local   # NEXT_PUBLIC_SUPABASE_ANON_KEY はSupabaseダッシュボード Settings > API Keys の Publishable key を設定する
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

M6（Googleカレンダー連携）以降は `docs/aibo/execution-plan.md` を参照。
