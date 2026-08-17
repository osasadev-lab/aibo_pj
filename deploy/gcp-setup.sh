#!/usr/bin/env bash
# GitHub Actions経由でCloud Runへデプロイする場合に必要な、GCP側の初期セットアップ。
# ローカルCLIから直接デプロイする分には不要（README.md参照）。
#
# 認証はWorkload Identity Federation（鍵ファイル不使用）。当初はサービスアカウントの
# JSONキーを発行する方式だったが、aibo-505714側の組織ポリシー
# (iam.disableServiceAccountKeyCreation)がプロジェクト単位の上書きも含めて
# 鍵発行そのものをブロックしていた（Organization Policy Administrator権限が必要で、
# このプロジェクトのOwnerからは変更不可）ため、鍵レスのOIDC認証に変更した。
#
# 前提：gcloudにログイン済み、対象プロジェクトがカレントに設定されていること。
# 実行方法：PROJECT_ID / REGION / GITHUB_REPOを環境に合わせて変更した上で、
# 必要な箇所だけ手動実行する。

set -euo pipefail

PROJECT_ID="aibo-505714"
REGION="asia-northeast1"
REPO_NAME="aibo"
SA_NAME="aibo-deployer"
SA_DISPLAY_NAME="aibo CI/CD deployer"
GITHUB_REPO="osasadev-lab/aibo_pj"
WIF_POOL="github-pool"
WIF_PROVIDER="github-provider"

PROJECT_NUMBER=$(gcloud projects describe "$PROJECT_ID" --format="value(projectNumber)")

# 1. 必要なAPIを有効化（既に有効化済みなら何もしない）
gcloud services enable run.googleapis.com artifactregistry.googleapis.com iam.googleapis.com \
  iamcredentials.googleapis.com sts.googleapis.com \
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
COMPUTE_SA="${PROJECT_NUMBER}-compute@developer.gserviceaccount.com"

# 4. 必要なロールを付与（最小権限：デプロイのみ、IAMポリシー変更は含まない）
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/run.developer"

gcloud projects add-iam-policy-binding "$PROJECT_ID" \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/artifactregistry.writer"

# Cloud Runの実行時サービスアカウント（デフォルトのCompute Engine SA）に対してのみ、
# なりすまし(actAs)権限を付与する（プロジェクト全体には付与しない）
gcloud iam service-accounts add-iam-policy-binding "$COMPUTE_SA" \
  --project "$PROJECT_ID" \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/iam.serviceAccountUser"

# 5. Workload Identity Pool / Provider を作成（GitHub ActionsのOIDCトークンを信頼する）
gcloud iam workload-identity-pools create "$WIF_POOL" \
  --project="$PROJECT_ID" \
  --location="global" \
  --display-name="GitHub Actions Pool"

gcloud iam workload-identity-pools providers create-oidc "$WIF_PROVIDER" \
  --project="$PROJECT_ID" \
  --location="global" \
  --workload-identity-pool="$WIF_POOL" \
  --display-name="GitHub Actions Provider" \
  --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository,attribute.repository_owner=assertion.repository_owner" \
  --issuer-uri="https://token.actions.githubusercontent.com" \
  --attribute-condition="assertion.repository_owner == '$(echo "$GITHUB_REPO" | cut -d/ -f1)'"

# 6. 指定したGitHubリポジトリからのみ、aibo-deployerをimpersonate可能にする
gcloud iam service-accounts add-iam-policy-binding "$SA_EMAIL" \
  --project="$PROJECT_ID" \
  --role="roles/iam.workloadIdentityUser" \
  --member="principalSet://iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${WIF_POOL}/attribute.repository/${GITHUB_REPO}"

echo "作成完了。.github/workflows/deploy-server.yml の workload_identity_provider には以下を設定する："
echo "  projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${WIF_POOL}/providers/${WIF_PROVIDER}"
echo "service_account には ${SA_EMAIL} を設定する（GitHub Secretsへの登録は不要）。"
