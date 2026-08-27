package activity

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/internal/logging"
)

// Record はActivityLog行をトランザクション内で作成する。呼び出し元のwithTx処理の
// 中で（本体の変更と同一コミットになるよう）呼ぶこと。taskID/projectIDはnil可。
//
// あわせてlogging.Loggerへ「誰が・何を・対象は」を1行のJSONログとして出力する
// （2026-08-27追加）。ActivityLog（DB、ハイライト機能・将来のAI進捗サマリー機能の
// データソース）とは別に、Cloud Logging等でのプロセスログ検索・障害調査からも
// 主要な状態変更操作を追えるようにする目的。この関数がプロジェクト作成/更新/削除・
// タスク作成/更新/削除/担当者変更/タグ付け/ステータス変更/依存関係変更・コメント投稿
// など、既存の全呼び出し箇所（実装時点で13箇所）を一括でカバーする。
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
	if _, err := builder.Save(ctx); err != nil {
		return err
	}

	attrs := make([]slog.Attr, 0, len(payload)+4)
	attrs = append(attrs,
		slog.String("action", actionType),
		slog.String("actor_id", actorID.String()),
		slog.String("workspace_id", workspaceID.String()),
	)
	if taskID != nil {
		attrs = append(attrs, slog.String("task_id", taskID.String()))
	}
	if projectID != nil {
		attrs = append(attrs, slog.String("project_id", projectID.String()))
	}
	for k, v := range payload {
		attrs = append(attrs, slog.Any(k, v))
	}
	logging.Logger.LogAttrs(ctx, slog.LevelInfo, "action", attrs...)

	return nil
}
