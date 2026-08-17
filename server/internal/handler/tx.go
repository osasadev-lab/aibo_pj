package handler

import (
	"context"

	"github.com/osasadev-lab/aibo_pj/server/ent"
)

// withTx はトランザクションを開始し、fnの結果に応じてCommit/Rollbackする共通ヘルパー。
func withTx(ctx context.Context, client *ent.Client, fn func(tx *ent.Tx) error) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
