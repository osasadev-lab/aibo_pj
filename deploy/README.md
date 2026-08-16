# デプロイ関連ドキュメント

実際のビルド・デプロイ設定ファイル自体は各アプリのディレクトリに置いている
（`server/Dockerfile`、`web/wrangler.jsonc` 等はビルドツールの仕様上そこに
置く必要があるため）。このフォルダには手順・チェックリスト・セットアップ
スクリプトなどのドキュメント系だけをまとめる。

- [secrets-checklist.md](secrets-checklist.md)：GitHub Actionsのデプロイワークフローに必要なリポジトリSecrets一覧
- [gcp-setup.sh](gcp-setup.sh)：GitHub Actions経由でCloud Runへデプロイする場合に必要な、GCP側の初期セットアップ（Artifact Registry・CI用サービスアカウント作成）
- [verification-log.md](verification-log.md)：実際にデプロイして疎通確認した記録

## デプロイ方法は2通り

1. **GitHub Actions経由**（`.github/workflows/deploy-server.yml` / `deploy-web.yml`）：`secrets-checklist.md`のSecretsを登録した上で、GitHubの Actions タブから手動実行（`workflow_dispatch`）
2. **ローカルCLIから直接**：手元で`gcloud`（Cloud Run）・`wrangler`（Cloudflare Workers）にログイン済みであれば、下記コマンドで直接デプロイ可能

```sh
# Cloud Run（Cloud Buildでビルドしてそのままデプロイ）
gcloud run deploy aibo-server \
  --source ./server \
  --region asia-northeast1 \
  --allow-unauthenticated \
  --set-env-vars DATABASE_URL=<接続文字列>

# Cloudflare Workers
cd web
npm run deploy
```
