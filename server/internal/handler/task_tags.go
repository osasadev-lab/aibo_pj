package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/tag"
	"github.com/osasadev-lab/aibo_pj/server/ent/tasktag"
	"github.com/osasadev-lab/aibo_pj/server/internal/activity"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

type putTagsRequest struct {
	TagIDs []uuid.UUID `json:"tag_ids" binding:"required"`
}

// ListAssignableTags は GET /tasks/:task_id/assignable-tags。
// このタスクに付与可能なタグ一覧（プロジェクト所属タスクならそのプロジェクト専用タグ＋
// ワークスペース共通タグ、単体タスクなら共通タグのみ）を返す。タスク詳細のタグピッカー用。
func (h *TaskHandler) ListAssignableTags(c *gin.Context) {
	t := middleware.CurrentTask(c)

	scope := tag.ProjectIDIsNil()
	if t.ProjectID != nil {
		scope = tag.Or(tag.ProjectIDEQ(*t.ProjectID), tag.ProjectIDIsNil())
	}

	tags, err := h.client.Tag.Query().
		Where(tag.WorkspaceIDEQ(t.WorkspaceID), scope).
		Order(tag.ByName()).
		All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list assignable tags"})
		return
	}

	out := make([]gin.H, 0, len(tags))
	for _, tg := range tags {
		out = append(out, tagJSON(tg))
	}
	c.JSON(http.StatusOK, out)
}

// PutTags は PUT /tasks/:task_id/tags。タグを入れ替える。
func (h *TaskHandler) PutTags(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)

	var req putTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag_ids is required"})
		return
	}

	ctx := c.Request.Context()
	ids, err := h.validTaskTagIDs(ctx, t.WorkspaceID, t.ProjectID, req.TagIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag_ids must be assignable to this task"})
		return
	}

	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		if _, err := tx.TaskTag.Delete().Where(tasktag.TaskIDEQ(t.ID)).Exec(ctx); err != nil {
			return err
		}
		if len(ids) > 0 {
			builders := make([]*ent.TaskTagCreate, 0, len(ids))
			for _, tid := range ids {
				builders = append(builders, tx.TaskTag.Create().SetTaskID(t.ID).SetTagID(tid))
			}
			if _, err := tx.TaskTag.CreateBulk(builders...).Save(ctx); err != nil {
				return err
			}
		}
		return activity.Record(ctx, tx, t.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.tagged",
			map[string]any{"tag_ids": ids})
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tags"})
		return
	}
	c.Status(http.StatusNoContent)
}

// validTaskTagIDs はtag_idsが、このタスクに付与可能なタグ（projectIDが非nilなら
// そのプロジェクト専用タグ＋ワークスペース共通タグ、nilなら共通タグのみ）に
// すべて属することを検証する。
func (h *TaskHandler) validTaskTagIDs(ctx context.Context, workspaceID uuid.UUID, projectID *uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	deduped := dedupUUIDs(ids)
	scope := tag.ProjectIDIsNil()
	if projectID != nil {
		scope = tag.Or(tag.ProjectIDEQ(*projectID), tag.ProjectIDIsNil())
	}
	count, err := h.client.Tag.Query().
		Where(tag.WorkspaceIDEQ(workspaceID), tag.IDIn(deduped...), scope).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	if count != len(deduped) {
		return nil, errInvalidIDs
	}
	return deduped, nil
}
