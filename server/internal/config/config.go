package config

import (
	"log"
	"os"
)

// Config はサーバー起動に必要な環境変数をまとめたもの。
type Config struct {
	Port                    string
	DatabaseURL             string
	JWTSecret               string
	GoogleOAuthClientID     string
	GoogleOAuthClientSecret string
	GoogleOAuthRedirectURL  string
	FrontendURL             string
	SupabaseJWTSecret       string

	// GoogleCalendarRedirectURLはM6（Googleカレンダー連携）用の同意フロー専用
	// redirect URL。ログイン用（GoogleOAuthRedirectURL）とは別のcallbackパスにする
	// （docs/aibo/m6-implementation-plan.md参照。stateにaibo JWTを載せる方式のため
	// ログインフローとは別経路にする必要がある）。
	GoogleCalendarRedirectURL string
	// TokenEncryptionKeyはusers.google_refresh_tokenの暗号化(AES-256-GCM)に使う鍵。
	// base64エンコードされた32バイトを想定（`openssl rand -base64 32`等で生成）。
	TokenEncryptionKey string

	// VAPID*はM7（Web Push配信）用。webpush.GenerateVAPIDKeys()で1回生成して設定する
	// （docs/aibo/m7-implementation-plan.md参照）。
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string
	// InternalCronSecretはCloud Schedulerからのリマインダーバッチ呼び出し
	// （POST /internal/cron/reminders）を認証する共有シークレット。
	InternalCronSecret string

	// R2*はM4（添付ファイル）用。未設定でもサーバー起動は妨げない（getEnvで空文字許容）。
	// 添付ファイルAPI呼び出し時に未設定なら500 storage_not_configuredを返す方針
	// （docs/aibo/m4-implementation-plan.md参照）。
	R2AccountID       string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2BucketName      string

	// SMTP*はM8（フィードバック機能）用。GmailのSMTPリレー経由でosasadev@gmail.com宛に
	// フィードバック内容を通知する。R2同様、未設定でもサーバー起動は妨げない
	// （getEnvで空文字許容）。SMTPHostが空ならfeedbackmail.Asyncは何もせずスキップする
	// （フィードバック自体のDB保存は引き続き成功する）。
	SMTPHost            string
	SMTPPort            string
	SMTPUsername        string
	SMTPPassword        string
	FeedbackNotifyEmail string

	// AllowTestLoginはM9（E2Eテスト）用。空でなければPOST /auth/test-loginが
	// 有効になり、この値と一致する`X-Test-Login-Secret`ヘッダー付きリクエストで
	// Google OAuthを介さずJWTを発行できる（Playwright等のE2Eテスト専用）。
	// 本番環境のSecretsには絶対に設定しないこと（未設定ならエンドポイントは404）。
	AllowTestLogin string
}

// Load は環境変数からConfigを組み立てる。必須項目が欠けていればプロセスを終了する。
func Load() Config {
	cfg := Config{
		Port:                      getEnv("PORT", "8080"),
		DatabaseURL:               mustEnv("DATABASE_URL"),
		JWTSecret:                 mustEnv("JWT_SECRET"),
		GoogleOAuthClientID:       mustEnv("GOOGLE_OAUTH_CLIENT_ID"),
		GoogleOAuthClientSecret:   mustEnv("GOOGLE_OAUTH_CLIENT_SECRET"),
		GoogleOAuthRedirectURL:    mustEnv("GOOGLE_OAUTH_REDIRECT_URL"),
		FrontendURL:               mustEnv("FRONTEND_URL"),
		SupabaseJWTSecret:         mustEnv("SUPABASE_JWT_SECRET"),
		GoogleCalendarRedirectURL: mustEnv("GOOGLE_CALENDAR_REDIRECT_URL"),
		TokenEncryptionKey:        mustEnv("TOKEN_ENCRYPTION_KEY"),
		VAPIDPublicKey:            mustEnv("VAPID_PUBLIC_KEY"),
		VAPIDPrivateKey:           mustEnv("VAPID_PRIVATE_KEY"),
		VAPIDSubject:              mustEnv("VAPID_SUBJECT"),
		InternalCronSecret:        mustEnv("INTERNAL_CRON_SECRET"),
		R2AccountID:               getEnv("R2_ACCOUNT_ID", ""),
		R2AccessKeyID:             getEnv("R2_ACCESS_KEY_ID", ""),
		R2SecretAccessKey:         getEnv("R2_SECRET_ACCESS_KEY", ""),
		R2BucketName:              getEnv("R2_BUCKET_NAME", ""),
		SMTPHost:                  getEnv("SMTP_HOST", ""),
		SMTPPort:                  getEnv("SMTP_PORT", "587"),
		SMTPUsername:              getEnv("SMTP_USERNAME", ""),
		SMTPPassword:              getEnv("SMTP_PASSWORD", ""),
		FeedbackNotifyEmail:       getEnv("FEEDBACK_NOTIFY_EMAIL", ""),
		AllowTestLogin:            getEnv("ALLOW_TEST_LOGIN", ""),
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %s is not set", key)
	}
	return v
}
