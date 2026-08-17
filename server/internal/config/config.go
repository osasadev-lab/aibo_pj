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
