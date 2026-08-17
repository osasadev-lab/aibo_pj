#!/usr/bin/env bash
# GitHub Actions経由でCloud Runへデプロイする場合に必要な、GCP側の初期セットアップ。
# ローカルCLIから直接デプロイする分には不要（README.md参照）。
#
# 前提：gcloudにログイン済み、対象プロジェクトがカレントに設定されていること。
# 実行方法：PROJECT_ID / REGIONを環境に合わせて変更した上で、必要な箇所だけ手動実行する
# （最後のサービスアカウントキー発行は長期間有効な機密情報を作るため、
#  スクリプト一括実行ではなく内容を確認しながら手動で行うことを推奨）。

set -euo pipefail

PROJECT_ID="aibo-505714"
REGION="asia-northeast1"
REPO_NAME="aibo"
SA_NAME="aibo-deployer"
SA_DISPLAY_NAME="aibo CI/CD deployer"

# 1. 必要なAPIを有効化（既に有効化済みなら何もしない）
gcloud services enable run.googleapis.com artifactregistry.googleapis.com iam.googleapis.com \
  --project "$PROJECT_ID"

# 2. Artifact RegistryにDockerリポジトリを作成
gcloud artifacts repositories create "$REPO_NAME" \
  --repository-format=docker \
  --location="$REGION" \
  --project "$PROJECT_ID"

# 3. CI/CD用サービスアカウントを作成
gcloud iam service-accounts create "$SA_NAME" \
  --display-name="$SA_DISPLAY_NAME" \
  --project "$PROJECT_ID"

SA_EMAIL="${SA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"

# 4. 必要なロールを付与
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/run.admin"

gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/artifactregistry.writer"

gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/iam.serviceAccountUser"

# 5. JSONキーを発行（GitHub Secretsの GCP_SA_KEY に登録する）
#    発行したファイルはGitHub Secrets登録後に手元から削除すること。
gcloud iam service-accounts keys create "./${SA_NAME}-key.json" \
  --iam-account="$SA_EMAIL" \
  --project "$PROJECT_ID"

echo "作成完了。./${SA_NAME}-key.json の中身をGitHub SecretsのGCP_SA_KEYに登録し、その後このファイルは削除してください。"
