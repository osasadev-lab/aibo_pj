# GitHub Actions用 Secrets チェックリスト

`https://github.com/osasadev-lab/aibo_pj` → Settings → Secrets and variables → Actions で登録する。

## Cloud Run（`deploy-server.yml`）

認証はWorkload Identity Federation（鍵レス）。`GCP_SA_KEY`のようなSecretは不要
（`workload_identity_provider` / `service_account`はワークフローYAML内に直書き。
機密情報ではないため）。セットアップ手順は`gcp-setup.sh`参照。

**M9（2026-08-29）でCloud Run本番環境にすべて反映済み**（ローカルCLI経由、方法A）。GitHub Actions（方法B）を使う場合は、以下をリポジトリSecretsとして登録すること（`server/internal/config/config.go`の`mustEnv`対象は全て必須、未設定だと起動時に落ちる）。値は`認証情報.txt`の「本番Cloud Run env vars」節参照（本番用に新規発行したものはローカル`.env`とは異なる値）。

| Secret名 | 値 | 備考 |
|---|---|---|
| `GCP_PROJECT_ID` | `aibo-505714` | |
| `GCP_REGION` | `asia-northeast1` | |
| `CLOUD_RUN_SERVICE` | `aibo-server` | |
| `DATABASE_URL` | Supabase接続文字列 | ローカルと同じ値（同一Supabaseプロジェクト） |
| `JWT_SECRET` | ランダム文字列 | 本番用に新規発行（ローカルと別の値） |
| `GOOGLE_OAUTH_CLIENT_ID` / `GOOGLE_OAUTH_CLIENT_SECRET` | GCPコンソールのOAuthクライアント | ローカルと同じ値（単一のOAuthクライアントを使い回し、リダイレクトURIを複数登録する運用） |
| `GOOGLE_OAUTH_REDIRECT_URL` | `https://aibo-server-494998752926.asia-northeast1.run.app/api/v1/auth/google/callback` | **GCPコンソールのOAuthクライアント「承認済みのリダイレクトURI」への追加登録が別途必要（未実施）** |
| `FRONTEND_URL` | `https://aibo-web.osasadev.workers.dev` | |
| `SUPABASE_JWT_SECRET` | Supabaseダッシュボードの値 | ローカルと同じ値（Supabase側の設定に紐づくため環境間で共通） |
| `GOOGLE_CALENDAR_REDIRECT_URL` | `https://aibo-server-494998752926.asia-northeast1.run.app/api/v1/auth/google/calendar/callback` | ログイン用とは別のcallback。**こちらもGCPコンソールへの追加登録が別途必要（未実施）** |
| `TOKEN_ENCRYPTION_KEY` | base64エンコードされた32バイト | 本番用に新規発行（ローカルと別の値） |
| `VAPID_PUBLIC_KEY` / `VAPID_PRIVATE_KEY` | `webpush.GenerateVAPIDKeys()`で生成した鍵ペア | 本番用に新規発行（ローカルと別の値） |
| `VAPID_SUBJECT` | `mailto:osasadev@gmail.com` | |
| `INTERNAL_CRON_SECRET` | ランダム文字列 | 本番用に新規発行（ローカルと別の値）。Cloud Schedulerジョブと共有する値 |

あわせてGoogle Cloud ConsoleでのCalendar API有効化・OAuth同意画面への`https://www.googleapis.com/auth/calendar.events`スコープ追加は対応済み（2026-08-27実機検証時）。ただし**本番Cloud RunのリダイレクトURI（上記2件）をOAuthクライアントの「承認済みのリダイレクトURI」へ追加登録する作業は未実施**（Google Cloud Consoleでの手動作業、CLIでは操作不可）。

Cloud Schedulerジョブ（15分ごとに`POST {Cloud Run URL}/api/v1/internal/cron/reminders`を`X-Internal-Cron-Secret`ヘッダー付きで呼ぶ）は未作成（`gcloud scheduler jobs create http`コマンド例はdocs/aibo/m7-implementation-plan.md参照）。

**任意項目（SMTP、`getEnv`のため未設定でも起動する）**：`SMTP_HOST`/`SMTP_PORT`/`SMTP_USERNAME`/`SMTP_PASSWORD`/`FEEDBACK_NOTIFY_EMAIL`。ローカルと同じGmail SMTPリレー設定を本番にも反映済み（2026-08-29）。未設定の場合、フィードバック機能のDB保存は動くがメール通知だけスキップされる。

**添付ファイル（R2）は本番で有効化済み（2026-08-29）**：`R2_ACCOUNT_ID`/`R2_ACCESS_KEY_ID`/`R2_SECRET_ACCESS_KEY`/`R2_BUCKET_NAME`（バケット名`aibo-attachments`）を設定済み（値は`認証情報.txt`参照）。あわせてR2バケットのCORS設定に本番オリジン（`https://aibo-web.osasadev.workers.dev`）を追加済み（従来`http://localhost:3000`のみ許可されていた）。ローカル`.env`側は開発の都合で引き続き無効化されたままで問題ない（`getEnv`のため未設定でもサーバーは起動し、アップロードUI自体が非表示になるだけ）。

## Cloudflare Workers（`deploy-web.yml`）

| Secret名 | 値 | 備考 |
|---|---|---|
| `CLOUDFLARE_API_TOKEN` | Workers Scripts:Edit権限のトークン | Cloudflareダッシュボードで発行 |
| `CLOUDFLARE_ACCOUNT_ID` | `c85792e66a8c3f143139af5026e02f0b` | |

worker名（`aibo-web`）は`web/wrangler.jsonc`の`name`で管理しているため、
Secretとしては登録不要。

**ビルド時のNEXT_PUBLIC_*変数**：`NEXT_PUBLIC_API_BASE_URL`・`NEXT_PUBLIC_SUPABASE_URL`・`NEXT_PUBLIC_SUPABASE_ANON_KEY`はSecretではなく`deploy-web.yml`に直書き（値自体が公開して問題ないもの：APIのURLとSupabaseのPublishable/anon key）。ローカルCLIでビルドする場合は`web/.env.production.local`（gitignore対象、2026-08-29新規作成）に同じ値を置いている。
