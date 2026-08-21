# GitHub Actions用 Secrets チェックリスト

`https://github.com/osasadev-lab/aibo_pj` → Settings → Secrets and variables → Actions で登録する。

## Cloud Run（`deploy-server.yml`）

認証はWorkload Identity Federation（鍵レス）。`GCP_SA_KEY`のようなSecretは不要
（`workload_identity_provider` / `service_account`はワークフローYAML内に直書き。
機密情報ではないため）。セットアップ手順は`gcp-setup.sh`参照。

| Secret名 | 値 | 備考 |
|---|---|---|
| `GCP_PROJECT_ID` | `aibo-505714` | |
| `GCP_REGION` | `asia-northeast1` | |
| `CLOUD_RUN_SERVICE` | `aibo-server` | |
| `DATABASE_URL` | Supabase接続文字列 | `server/.env.example`参照。パスワードは`認証情報.txt`のDBPASS |

**M6追加（Googleカレンダー連携）で必要な環境変数（未登録・要対応）**：`deploy-server.yml`は現状`DATABASE_URL`しか`env_vars`に渡していない。`JWT_SECRET`等の既存必須環境変数と同様、以下もCloud Runサービス側に設定する必要がある（`server/internal/config/config.go`の`mustEnv`対象、未設定だと起動時に落ちる）。

| 環境変数名 | 値 | 備考 |
|---|---|---|
| `TOKEN_ENCRYPTION_KEY` | base64エンコードされた32バイト | `users.google_refresh_token`の暗号化用（AES-256-GCM）。`openssl rand -base64 32`等で新規発行し、ローカル`.env`とは別の値にすること |
| `GOOGLE_CALENDAR_REDIRECT_URL` | 例: `https://<Cloud Run URL>/api/v1/auth/google/calendar/callback` | ログイン用`GOOGLE_OAUTH_REDIRECT_URL`とは別のcallback。GCPコンソールのOAuthクライアント「承認済みのリダイレクトURI」にも追加登録が必要 |

あわせてGoogle Cloud ConsoleでCalendar APIの有効化、OAuth同意画面への`https://www.googleapis.com/auth/calendar.events`スコープ追加、（本番公開前は）テストユーザー登録または確認審査の実施が必要（未実施、docs/aibo/m6-implementation-plan.md参照）。

## Cloudflare Workers（`deploy-web.yml`）

| Secret名 | 値 | 備考 |
|---|---|---|
| `CLOUDFLARE_API_TOKEN` | Workers Scripts:Edit権限のトークン | Cloudflareダッシュボードで発行 |
| `CLOUDFLARE_ACCOUNT_ID` | `c85792e66a8c3f143139af5026e02f0b` | |

worker名（`aibo-web`）は`web/wrangler.jsonc`の`name`で管理しているため、
Secretとしては登録不要。
