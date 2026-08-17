# GitHub Actions用 Secrets チェックリスト

`https://github.com/osasadev-lab/aibo_pj` → Settings → Secrets and variables → Actions で登録する。

## Cloud Run（`deploy-server.yml`）

| Secret名 | 値 | 備考 |
|---|---|---|
| `GCP_PROJECT_ID` | `aibo-505714` | |
| `GCP_REGION` | `asia-northeast1` | |
| `GCP_SA_KEY` | サービスアカウントのJSONキー全体 | `gcp-setup.sh`で作成 |
| `CLOUD_RUN_SERVICE` | `aibo-server` | |
| `DATABASE_URL` | Supabase接続文字列 | `server/.env.example`参照。パスワードは`認証情報.txt`のDBPASS |

## Cloudflare Workers（`deploy-web.yml`）

| Secret名 | 値 | 備考 |
|---|---|---|
| `CLOUDFLARE_API_TOKEN` | Workers Scripts:Edit権限のトークン | Cloudflareダッシュボードで発行 |
| `CLOUDFLARE_ACCOUNT_ID` | `c85792e66a8c3f143139af5026e02f0b` | |

worker名（`aibo-web`）は`web/wrangler.jsonc`の`name`で管理しているため、
Secretとしては登録不要。
