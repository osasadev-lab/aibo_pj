package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskassignee"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	"github.com/osasadev-lab/aibo_pj/server/internal/activity"
	"github.com/osasadev-lab/aibo_pj/server/internal/calendarsync"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/pushdelivery"
)

type putAssigneesRequest struct {
	UserIDs []uuid.UUID `json:"user_ids" binding:"required"`
}

// PutAssignees は PUT /tasks/:task_id/assignees。担当者を入れ替える。
func (h *TaskHandler) PutAssignees(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)

	var req putAssigneesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids is required"})
		return
	}

	ctx := c.Request.Context()
	ids, err := h.validWorkspaceUserIDs(ctx, t.WorkspaceID, req.UserIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids must all be workspace members"})
		return
	}

	var addedIDs, removedIDs []uuid.UUID
	var pendingPush []pushdelivery.Item
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		existing, err := tx.TaskAssignee.Query().Where(taskassignee.TaskIDEQ(t.ID)).All(ctx)
		if err != nil {
			return err
		}
		oldSet := make(map[uuid.UUID]struct{}, len(existing))
		for _, a := range existing {
			oldSet[a.UserID] = struct{}{}
		}
		newSet := make(map[uuid.UUID]struct{}, len(ids))
		for _, id := range ids {
			newSet[id] = struct{}{}
		}
		for _, id := range ids {
			if _, was := oldSet[id]; !was {
				addedIDs = append(addedIDs, id)
			}
		}
		for id := range oldSet {
			if _, still := newSet[id]; !still {
				removedIDs = append(removedIDs, id)
			}
		}

		if _, err := tx.TaskAssignee.Delete().Where(taskassignee.TaskIDEQ(t.ID)).Exec(ctx); err != nil {
			return err
		}
		if len(ids) > 0 {
			builders := make([]*ent.TaskAssigneeCreate, 0, len(ids))
			for _, uid := range ids {
				builders = append(builders, tx.TaskAssignee.Create().SetTaskID(t.ID).SetUserID(uid))
			}
			if _, err := tx.TaskAssignee.CreateBulk(builders...).Save(ctx); err != nil {
				return err
			}
		}

		for _, id := range ids {
			if _, was := oldSet[id]; was {
				continue
			}
			payload := map[string]any{
				"task_id":         t.ID,
				"project_id":      t.ProjectID,
				"changed_by":      u.ID,
				"changed_by_name": u.Name,
				"task_title":      t.Title,
			}
			if _, err := tx.Notification.Create().
				SetUserID(id).
				SetType("assigned").
				SetPayload(payload).
				Save(ctx); err != nil {
				return err
			}
			pendingPush = append(pendingPush, pushdelivery.BuildItem(id, h.frontendURL, t.WorkspaceID.String(), "assigned", payload))
		}
		for id := range oldSet {
			if _, still := newSet[id]; still {
				continue
			}
			payload := map[string]any{
				"task_id":         t.ID,
				"project_id":      t.ProjectID,
				"changed_by":      u.ID,
				"changed_by_name": u.Name,
				"task_title":      t.Title,
			}
			if _, err := tx.Notification.Create().
				SetUserID(id).
				SetType("unassigned").
				SetPayload(payload).
				Save(ctx); err != nil {
				return err
			}
			pendingPush = append(pendingPush, pushdelivery.BuildItem(id, h.frontendURL, t.WorkspaceID.String(), "unassigned", payload))
		}

		return activity.Record(ctx, tx, t.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.assigned",
			map[string]any{"assignee_ids": ids})
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update assignees"})
		return
	}

	// Googleカレンダー自動同期（M6）。新規追加された担当者にはイベントを作成、
	// 外れた担当者のイベントは削除する（他の担当者には影響しない）。
	// バックグラウンド化の理由はCreate/Updateと同じ（体感速度対策、2026-08-21）。
	for _, id := range addedIDs {
		calendarsync.Async(func(ctx context.Context) {
			calendarsync.SyncTaskForUser(ctx, h.client, h.calCfg, h.encKey, t, id, h.frontendURL)
		})
	}
	for _, id := range removedIDs {
		calendarsync.Async(func(ctx context.Context) {
			calendarsync.SyncTaskForUserOnUnassign(ctx, h.client, h.calCfg, h.encKey, t.ID, id)
		})
	}

	pushdelivery.Async(h.client, h.pushCfg, pendingPush)

	c.Status(http.StatusNoContent)
}

// validWorkspaceUserIDs はuser_idsが重複除去の上、全てそのworkspaceのメンバーで
// あることを検証する（担当者・メンション先の検証で共通利用）。
func (h *TaskHandler) validWorkspaceUserIDs(ctx context.Context, workspaceID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	deduped := dedupUUIDs(ids)
	count, err := h.client.WorkspaceMember.Query().
		Where(workspacemember.WorkspaceIDEQ(workspaceID), workspacemember.UserIDIn(deduped...)).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	if count != len(deduped) {
		return nil, errInvalidIDs
	}
	return deduped, nil
}
