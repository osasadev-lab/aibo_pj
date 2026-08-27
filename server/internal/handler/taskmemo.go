package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"entgo.io/ent/dialect/sql"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/task"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskmemo"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskmemoattachment"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/storage"
)

// TaskMemoHandler は /tasks/:task_id/memo(/attachments), /memo-attachments/:attachment_id,
// /workspaces/:workspace_id/task-memos を扱う。メモは本人にのみ表示される個人データ
// （docs/aibo/m8.5-implementation-plan.md）。
type TaskMemoHandler struct {
	client *ent.Client
	r2     *storage.R2Client
}

func NewTaskMemoHandler(client *ent.Client, r2 *storage.R2Client) *TaskMemoHandler {
	return &TaskMemoHandler{client: client, r2: r2}
}

func taskMemoJSON(m *ent.TaskMemo) gin.H {
	row := gin.H{"task_id": m.TaskID, "body": m.Body, "updated_at": m.UpdatedAt}
	atts := make([]gin.H, 0, len(m.Edges.Attachments))
	for _, a := range m.Edges.Attachments {
		atts = append(atts, gin.H{
			"id": a.ID, "file_name": a.FileName, "size_bytes": a.SizeBytes, "content_type": a.ContentType,
		})
	}
	row["attachments"] = atts
	return row
}

// Get は GET /tasks/:task_id/memo。自分のメモが無ければbody/attachmentsとも空で返す。
func (h *TaskMemoHandler) Get(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	m, err := h.client.TaskMemo.Query().
		Where(taskmemo.TaskIDEQ(t.ID), taskmemo.UserIDEQ(u.ID)).
		WithAttachments().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusOK, gin.H{"task_id": t.ID, "body": nil, "attachments": []gin.H{}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load memo"})
		return
	}
	c.JSON(http.StatusOK, taskMemoJSON(m))
}

type putTaskMemoRequest struct {
	Body string `json:"body"`
}

// Put は PUT /tasks/:task_id/memo。本文を丸ごと置換（無ければ作成）。
func (h *TaskMemoHandler) Put(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	var req putTaskMemoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	m, err := h.getOrCreateMemo(ctx, t.ID, u.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save memo"})
		return
	}
	updated, err := h.client.TaskMemo.UpdateOneID(m.ID).SetBody(req.Body).Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save memo"})
		return
	}
	updated, err = h.client.TaskMemo.Query().Where(taskmemo.IDEQ(updated.ID)).WithAttachments().Only(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save memo"})
		return
	}
	c.JSON(http.StatusOK, taskMemoJSON(updated))
}

// Delete は DELETE /tasks/:task_id/memo。
func (h *TaskMemoHandler) Delete(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	m, err := h.client.TaskMemo.Query().Where(taskmemo.TaskIDEQ(t.ID), taskmemo.UserIDEQ(u.ID)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.Status(http.StatusNoContent)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete memo"})
		return
	}

	attachments, err := h.client.TaskMemoAttachment.Query().Where(taskmemoattachment.TaskMemoIDEQ(m.ID)).All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete memo"})
		return
	}
	if err := withTx(ctx, h.client, func(tx *ent.Tx) error {
		if _, err := tx.TaskMemoAttachment.Delete().Where(taskmemoattachment.TaskMemoIDEQ(m.ID)).Exec(ctx); err != nil {
			return err
		}
		return tx.TaskMemo.DeleteOneID(m.ID).Exec(ctx)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete memo"})
		return
	}

	keys := make([]string, 0, len(attachments))
	for _, a := range attachments {
		keys = append(keys, a.StorageKey)
	}
	deleteR2Objects(ctx, h.r2, keys)
	c.Status(http.StatusNoContent)
}

// deleteTaskMemosForTasks はtask.go/project.go/workspace.goのカスケード削除から呼ぶ
// 共通ヘルパー。指定タスク群に紐づくメモ・メモ添付を削除し、R2削除用のstorage_key一覧を返す
// （呼び出し元はwithTxのトランザクション内で使い、コミット後にdeleteR2Objectsへ渡すこと）。
func deleteTaskMemosForTasks(ctx context.Context, tx *ent.Tx, taskIDs []uuid.UUID) ([]string, error) {
	if len(taskIDs) == 0 {
		return nil, nil
	}
	memoIDs, err := tx.TaskMemo.Query().Where(taskmemo.TaskIDIn(taskIDs...)).IDs(ctx)
	if err != nil {
		return nil, err
	}
	if len(memoIDs) == 0 {
		return nil, nil
	}
	attachments, err := tx.TaskMemoAttachment.Query().Where(taskmemoattachment.TaskMemoIDIn(memoIDs...)).All(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(attachments))
	for _, a := range attachments {
		keys = append(keys, a.StorageKey)
	}
	if _, err := tx.TaskMemoAttachment.Delete().Where(taskmemoattachment.TaskMemoIDIn(memoIDs...)).Exec(ctx); err != nil {
		return nil, err
	}
	if _, err := tx.TaskMemo.Delete().Where(taskmemo.IDIn(memoIDs...)).Exec(ctx); err != nil {
		return nil, err
	}
	return keys, nil
}

func (h *TaskMemoHandler) getOrCreateMemo(ctx context.Context, taskID, userID uuid.UUID) (*ent.TaskMemo, error) {
	m, err := h.client.TaskMemo.Query().Where(taskmemo.TaskIDEQ(taskID), taskmemo.UserIDEQ(userID)).Only(ctx)
	if err == nil {
		return m, nil
	}
	if !ent.IsNotFound(err) {
		return nil, err
	}
	return h.client.TaskMemo.Create().SetTaskID(taskID).SetUserID(userID).Save(ctx)
}

type createTaskMemoAttachmentRequest struct {
	FileName    string `json:"file_name" binding:"required"`
	ContentType string `json:"content_type" binding:"required"`
	SizeBytes   int64  `json:"size_bytes" binding:"required"`
}

// CreateAttachment は POST /tasks/:task_id/memo/attachments。
// メモ行が無ければ自動作成してから添付する。task/attachments と同じ2段階方式・
// 25MB上限（maxAttachmentSizeBytes、attachment.go）。
func (h *TaskMemoHandler) CreateAttachment(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	if h.r2 == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage_not_configured"})
		return
	}

	var req createTaskMemoAttachmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_name, content_type, size_bytes are required"})
		return
	}
	if req.SizeBytes > maxAttachmentSizeBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_too_large"})
		return
	}

	m, err := h.getOrCreateMemo(ctx, t.ID, u.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create attachment"})
		return
	}

	storageKey := fmt.Sprintf("memo/%s/%s/%s_%s", t.WorkspaceID, m.ID, uuid.NewString(), req.FileName)
	created, err := h.client.TaskMemoAttachment.Create().
		SetTaskMemoID(m.ID).
		SetUploadedBy(u.ID).
		SetFileName(req.FileName).
		SetStorageKey(storageKey).
		SetSizeBytes(req.SizeBytes).
		SetContentType(req.ContentType).
		Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create attachment"})
		return
	}

	uploadURL, err := h.r2.PresignPutObject(ctx, storageKey, req.ContentType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to presign upload url"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id": created.ID, "file_name": created.FileName, "size_bytes": created.SizeBytes,
		"content_type": created.ContentType, "upload_url": uploadURL,
	})
}

// DeleteAttachment は DELETE /memo-attachments/:attachment_id。本人がアップロードした
// メモ添付のみ削除できる（メモ自体が本人限定のデータのため）。
func (h *TaskMemoHandler) DeleteAttachment(c *gin.Context) {
	u := middleware.CurrentUser(c)

	attachmentID, err := uuid.Parse(c.Param("attachment_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "attachment not found"})
		return
	}

	ctx := c.Request.Context()
	a, err := h.client.TaskMemoAttachment.Query().
		Where(taskmemoattachment.IDEQ(attachmentID), taskmemoattachment.UploadedByEQ(u.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "attachment not found"})
		return
	}

	if h.r2 != nil {
		if err := h.r2.DeleteObject(ctx, a.StorageKey); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete file from storage"})
			return
		}
	}
	if err := h.client.TaskMemoAttachment.DeleteOneID(a.ID).Exec(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete attachment"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ListMine は GET /workspaces/:workspace_id/task-memos。自分がメモを持つタスク一覧
// （updated_at降順、PinnedTasksPanelと同型のレスポンス）。
func (h *TaskMemoHandler) ListMine(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	memos, err := h.client.TaskMemo.Query().
		Where(taskmemo.UserIDEQ(u.ID), taskmemo.HasTaskWith(task.WorkspaceIDEQ(m.WorkspaceID))).
		Order(taskmemo.ByUpdatedAt(sql.OrderDesc())).
		WithTask(func(q *ent.TaskQuery) { q.WithProject() }).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list memos"})
		return
	}

	out := make([]gin.H, 0, len(memos))
	for _, mm := range memos {
		if mm.Edges.Task == nil {
			continue
		}
		t := mm.Edges.Task
		var projectName *string
		if t.Edges.Project != nil {
			projectName = &t.Edges.Project.Name
		}
		excerptText := ""
		if mm.Body != nil {
			excerptText = excerpt(*mm.Body, 80)
		}
		out = append(out, gin.H{
			"task_id":      t.ID,
			"task_title":   t.Title,
			"project_id":   t.ProjectID,
			"project_name": projectName,
			"excerpt":      excerptText,
			"updated_at":   mm.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, out)
}
