package handler

import (
	"context"
	"net/http"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/activitylog"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

// ActivityHandler は /workspaces/:workspace_id/activity（ハイライト機能、M5追加）を扱う。
type ActivityHandler struct {
	client *ent.Client
}

func NewActivityHandler(client *ent.Client) *ActivityHandler {
	return &ActivityHandler{client: client}
}

// ハイライトに表示する対象action_type。誰が何をしたか（プロジェクトの作成・削除・
// 変更、タスクの作成・削除・カンバンステータス変更）に限定する（ユーザー確認済みの
// スコープ。task.updated/task.assigned/task.tagged/comment.created等は対象外）。
var highlightActionTypes = []string{
	"project.created",
	"project.updated",
	"project.deleted",
	"task.created",
	"task.deleted",
	"task.status_changed",
}

// ハイライト（activity_logs）の保持期間。ユーザー確認済み（データ容量懸念への対応、
// 2026-08-20）：ハイライト表示は通常「今日」寄りだが直近30日分は遡って見られるように
// する。この期間を過ぎた行は物理削除する（対象はaction_typeを問わず全行。将来の
// フェーズ2 AI進捗サマリー機能もこの期間制約を受ける前提でユーザー確認済み）。
//
// Cloud Runはmin-instances=0のリクエスト駆動構成でバックグラウンドジョブ基盤が
// 無いため、cron等は使わずGET /workspaces/:id/activityの呼び出し時にその
// workspace分だけベストエフォートで削除する“read時清掃”方式にしている
// （(workspace_id, created_at)のインデックスがあるため低コスト）。
// 制約：ハイライトを一度も開かないworkspaceでは掃除が走らない。より厳密な保証が
// 必要になれば、Cloud Scheduler等から叩く専用の削除エンドポイント/ジョブへの
// 切り出しを検討する。
const highlightRetentionDays = 30

func (h *ActivityHandler) pruneOldLogs(ctx context.Context, workspaceID uuid.UUID) {
	cutoff := time.Now().AddDate(0, 0, -highlightRetentionDays)
	_, _ = h.client.ActivityLog.Delete().
		Where(activitylog.WorkspaceIDEQ(workspaceID), activitylog.CreatedAtLT(cutoff)).
		Exec(ctx)
}

const activityDefaultLimit = 30
const activityMaxLimit = 100

// activityFetchBatchSizeはDBから一度に取得する行数。可視性フィルタ（private
// projectの非参画メンバー除外）は行取得後にアプリ層で行うため、limit件ぴったり
// フェッチすると可視な行がlimit未満しか集まらないことがある。フィルタで
// 弾かれる分を見込んで多めに取得する。
const activityFetchBatchSize = 100

// activityMaxIterationsは上記バッチ取得を最大何回まで繰り返すか。privateな
// プロジェクトの操作が大半を占めるワークスペースでも高々この回数で打ち切り、
// 無限ループやDBへの過剰な往復を避ける安全弁。
const activityMaxIterations = 5

// List は GET /workspaces/:workspace_id/activity。クエリ`actor_id`任意で絞り込み。
// 行ごとに可視性を判定する（private projectの操作は非参画メンバーには見せない）。
// ページネーション（2026-08-27追加）：`limit`（既定30、最大100）・`before`
// （activity_log ID、これより古いものを返す）。レスポンスは`{items, has_more}`。
func (h *ActivityHandler) List(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	h.pruneOldLogs(ctx, m.WorkspaceID)

	limit := parseLimit(c.Query("limit"), activityDefaultLimit, activityMaxLimit)

	var actorFilter *uuid.UUID
	if v := c.Query("actor_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid actor_id"})
			return
		}
		actorFilter = &id
	}

	var cursorCreatedAt *time.Time
	var cursorID *uuid.UUID
	if before := c.Query("before"); before != "" {
		id, err := uuid.Parse(before)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before"})
			return
		}
		cursor, err := h.client.ActivityLog.Query().
			Where(activitylog.IDEQ(id), activitylog.WorkspaceIDEQ(m.WorkspaceID)).
			Only(ctx)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before"})
			return
		}
		cursorCreatedAt = &cursor.CreatedAt
		cursorID = &cursor.ID
	}

	visibilityCache := map[uuid.UUID]bool{}
	collected := make([]*ent.ActivityLog, 0, limit+1)

	for iter := 0; iter < activityMaxIterations && len(collected) <= limit; iter++ {
		q := h.client.ActivityLog.Query().
			Where(
				activitylog.WorkspaceIDEQ(m.WorkspaceID),
				activitylog.ActionTypeIn(highlightActionTypes...),
			).
			Order(activitylog.ByCreatedAt(sql.OrderDesc()), activitylog.ByID(sql.OrderDesc())).
			Limit(activityFetchBatchSize)
		if actorFilter != nil {
			q = q.Where(activitylog.ActorIDEQ(*actorFilter))
		}
		if cursorCreatedAt != nil {
			q = q.Where(activitylog.Or(
				activitylog.CreatedAtLT(*cursorCreatedAt),
				activitylog.And(activitylog.CreatedAtEQ(*cursorCreatedAt), activitylog.IDLT(*cursorID)),
			))
		}

		batch, err := q.WithActor().WithProject().WithTask().All(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list activity"})
			return
		}
		if len(batch) == 0 {
			break
		}

		for _, l := range batch {
			visible, err := h.isVisible(ctx, l, m.UserID, visibilityCache)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check visibility"})
				return
			}
			if !visible {
				continue
			}
			collected = append(collected, l)
			if len(collected) > limit {
				break
			}
		}

		last := batch[len(batch)-1]
		cursorCreatedAt = &last.CreatedAt
		cursorID = &last.ID

		if len(batch) < activityFetchBatchSize {
			// DB側にもうこのworkspace分の行が残っていない。
			break
		}
	}

	hasMore := len(collected) > limit
	if hasMore {
		collected = collected[:limit]
	}

	out := make([]gin.H, 0, len(collected))
	for _, l := range collected {
		out = append(out, activityLogJSON(l))
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "has_more": hasMore})
}

// isVisible は1行のactivity_logsを呼び出しユーザーが見てよいかを判定する。
// project.deletedはproject_idが既にnilのためpayloadに複製済みのvisibility/
// member_user_idsで判定し、それ以外はproject_id/task_idが指す生存エンティティの
// 現在の可視性で判定する（削除時にそのエンティティに紐づくactivity_logsは
// カスケード削除されるため、生きている行のFKは常に有効）。
func (h *ActivityHandler) isVisible(ctx context.Context, l *ent.ActivityLog, userID uuid.UUID, cache map[uuid.UUID]bool) (bool, error) {
	if l.ActionType == "project.deleted" {
		if l.ActorID == userID {
			return true, nil
		}
		visibility, _ := l.Payload["visibility"].(string)
		if visibility != "private" {
			return true, nil
		}
		raw, _ := l.Payload["member_user_ids"].([]any)
		for _, v := range raw {
			s, ok := v.(string)
			if ok && s == userID.String() {
				return true, nil
			}
		}
		return false, nil
	}

	if l.ProjectID == nil {
		// 単体タスクの操作、または純ワークスペース操作。workspace全体に可視。
		return true, nil
	}

	if visible, ok := cache[*l.ProjectID]; ok {
		return visible, nil
	}
	p, err := h.client.Project.Get(ctx, *l.ProjectID)
	if err != nil {
		return false, nil //nolint:nilerr // プロジェクトが見つからない場合は非表示にするだけでエラーにはしない
	}
	err = middleware.CheckProjectVisibility(ctx, h.client, p, userID)
	visible := err == nil
	cache[*l.ProjectID] = visible
	return visible, nil
}

func activityLogJSON(l *ent.ActivityLog) gin.H {
	row := gin.H{
		"id":          l.ID,
		"action_type": l.ActionType,
		"actor_id":    l.ActorID,
		"project_id":  l.ProjectID,
		"task_id":     l.TaskID,
		"payload":     l.Payload,
		"created_at":  l.CreatedAt,
	}
	if l.Edges.Actor != nil {
		row["actor_name"] = l.Edges.Actor.Name
	}
	if l.Edges.Project != nil {
		row["project_name"] = l.Edges.Project.Name
	}
	if l.Edges.Task != nil {
		row["task_title"] = l.Edges.Task.Title
	}
	return row
}
