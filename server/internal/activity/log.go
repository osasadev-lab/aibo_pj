package activity

import (
	"context"

	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
)

// Record はActivityLog行をトランザクション内で作成する。呼び出し元のwithTx処理の
// 中で（本体の変更と同一コミットになるよう）呼ぶこと。taskID/projectIDはnil可。
func Record(ctx context.Context, tx *ent.Tx, workspaceID uuid.UUID, taskID, projectID *uuid.UUID, actorID uuid.UUID, actionType string, payload map[string]any) error {
	builder := tx.ActivityLog.Create().
		SetWorkspaceID(workspaceID).
		SetActorID(actorID).
		SetActionType(actionType).
		SetNillableTaskID(taskID).
		SetNillableProjectID(projectID)
	if payload != nil {
		builder = builder.SetPayload(payload)
	}
	_, err := builder.Save(ctx)
	return err
}
