// Package testutil はハンドラ・ミドルウェアのテストで使う共通ヘルパーを提供する
// （2026-08-27追加、自動テスト基盤の一部）。
//
// 本物のPostgres（Supabase）へは接続せず、pure-GoのSQLiteドライバ
// （modernc.org/sqlite、CGO不要）でテストごとに独立したin-memory DBを作り、
// entのスキーマをそのまま適用する。CI・ローカルどちらでも外部DB接続無しに
// 高速に実行できる（既存の.github/workflows/ci.ymlの`go test ./...`が
// そのまま使える）。
//
// 制約：Postgres固有の型ヒント（SchemaType）を持つフィールドがある場合、
// SQLite側では該当ヒントが無視されて汎用の型として扱われる。Postgres側だけの
// 型上の保証に依存するテストは、期待値を明示的に揃えること。
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	entdialect "entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"

	"github.com/osasadev-lab/aibo_pj/server/ent"
)

var dbCounter int64

// NewClient はテスト専用のin-memory SQLite DBに対し、スキーマ適用済みの
// ent.Clientを返す。t.Cleanup()でクローズするため呼び出し側での後始末は不要。
// DSNにテストごとの連番を含めることで、並行実行される複数テスト間でDBの
// 実体が混ざらないようにする。
func NewClient(t *testing.T) *ent.Client {
	t.Helper()

	n := atomic.AddInt64(&dbCounter, 1)
	dsn := fmt.Sprintf("file:testdb%d?mode=memory&cache=shared&_fk=1", n)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("testutil: sql.Open: %v", err)
	}
	// in-memoryかつ単一コネクションに固定する。複数コネクションを許すと
	// SQLiteのin-memory DBはコネクションごとに別実体になり、テスト中に
	// 「作ったはずのデータが見えない」という事故になるため。
	sqlDB.SetMaxOpenConns(1)

	drv := entsql.OpenDB(entdialect.SQLite, sqlDB)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })

	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("testutil: schema create: %v", err)
	}
	return client
}
