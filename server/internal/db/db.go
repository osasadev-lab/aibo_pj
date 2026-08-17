package db

import (
	"database/sql"

	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/osasadev-lab/aibo_pj/server/ent"
)

// NewEntClient はDSNからent.Clientを組み立てる。
// server/cmd/migrate/main.go と同じ接続パターンを共通化したもの。
func NewEntClient(dsn string) (*ent.Client, error) {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	drv := entsql.OpenDB("postgres", sqlDB)
	return ent.NewClient(ent.Driver(drv)), nil
}
