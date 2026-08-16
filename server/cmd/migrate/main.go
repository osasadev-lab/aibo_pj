package main

import (
	"context"
	"database/sql"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	entsql "entgo.io/ent/dialect/sql"
)

// entのスキーマを対象DBに適用する（テーブル作成／不足しているカラム・
// インデックスの追加）。実行方法: go run ./cmd/migrate
func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("failed opening connection to postgres: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// pgvectorはフェーズ2（タスクの埋め込み保存）で使用する。
	// spec.md 9章の方針に従い、今のうちに拡張を有効化しておく。
	if _, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		log.Fatalf("failed enabling pgvector extension: %v", err)
	}

	drv := entsql.OpenDB("postgres", db)
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	if err := client.Schema.Create(ctx); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}

	log.Println("migration completed successfully")
}
