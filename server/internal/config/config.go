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

	// R2*はM4（添付ファイル）用。未設定でもサーバー起動は妨げない（getEnvで空文字許容）。
	// 添付ファイルAPI呼び出し時に未設定なら500 storage_not_configuredを返す方針
	// （docs/aibo/m4-implementation-plan.md参照）。
	R2AccountID       string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2BucketName      string
}

// Load は環境変数からConfigを組み立てる。必須項目が欠けていればプロセスを終了する。
func Load() Config {
	cfg := Config{
		Port:                    getEnv("PORT", "8080"),
		DatabaseURL:             mustEnv("DATABASE_URL"),
		JWTSecret:               mustEnv("JWT_SECRET"),
		GoogleOAuthClientID:     mustEnv("GOOGLE_OAUTH_CLIENT_ID"),
		GoogleOAuthClientSecret: mustEnv("GOOGLE_OAUTH_CLIENT_SECRET"),
		GoogleOAuthRedirectURL:  mustEnv("GOOGLE_OAUTH_REDIRECT_URL"),
		FrontendURL:             mustEnv("FRONTEND_URL"),
		SupabaseJWTSecret:       mustEnv("SUPABASE_JWT_SECRET"),
		R2AccountID:             getEnv("R2_ACCOUNT_ID", ""),
		R2AccessKeyID:           getEnv("R2_ACCESS_KEY_ID", ""),
		R2SecretAccessKey:       getEnv("R2_SECRET_ACCESS_KEY", ""),
		R2BucketName:            getEnv("R2_BUCKET_NAME", ""),
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
