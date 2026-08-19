package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/attachment"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/storage"
)

// maxAttachmentSizeBytes は1ファイルあたりの上限（25MB、spec.md 4.6）。
const maxAttachmentSizeBytes int64 = 25 * 1024 * 1024

// deleteR2Objects はタスク/プロジェクト/ワークスペース削除時のカスケード削除で、
// DBのトランザクションコミットが確定した後にR2オブジェクトをベストエフォートで
// 削除する（外部API呼び出しをDBトランザクション内で行わないため）。1件失敗しても
// 残りは続行し、リクエスト自体は失敗させない（DB側は既に整合が取れているため）。
// r2がnil（未設定）の場合は何もしない。
func deleteR2Objects(ctx context.Context, r2 *storage.R2Client, keys []string) {
	if r2 == nil {
		return
	}
	for _, key := range keys {
		if err := r2.DeleteObject(ctx, key); err != nil {
			log.Printf("failed to delete r2 object %q: %v", key, err)
		}
	}
}

// AttachmentHandler は /tasks/:task_id/attachments, /attachments/:attachment_id 配下を扱う。
type AttachmentHandler struct {
	client *ent.Client
	r2     *storage.R2Client
}

func NewAttachmentHandler(client *ent.Client, r2 *storage.R2Client) *AttachmentHandler {
	return &AttachmentHandler{client: client, r2: r2}
}

func attachmentJSON(a *ent.Attachment) gin.H {
	return gin.H{
		"id":           a.ID,
		"task_id":      a.TaskID,
		"uploaded_by":  a.UploadedBy,
		"file_name":    a.FileName,
		"size_bytes":   a.SizeBytes,
		"content_type": a.ContentType,
	}
}

// ListAttachments は GET /tasks/:task_id/attachments。
func (h *AttachmentHandler) ListAttachments(c *gin.Context) {
	t := middleware.CurrentTask(c)

	list, err := h.client.Attachment.Query().
		Where(attachment.TaskIDEQ(t.ID)).
		Order(attachment.ByCreatedAt()).
		All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list attachments"})
		return
	}
	out := make([]gin.H, 0, len(list))
	for _, a := range list {
		out = append(out, attachmentJSON(a))
	}
	c.JSON(http.StatusOK, out)
}

type createAttachmentRequest struct {
	FileName    string `json:"file_name" binding:"required"`
	ContentType string `json:"content_type" binding:"required"`
	SizeBytes   int64  `json:"size_bytes" binding:"required"`
}

// CreateAttachment は POST /tasks/:task_id/attachments。
// R2への署名付きPUT URLを発行し、メタデータ行を作成する2段階方式の1段階目。
// クライアントは返却されたupload_urlへ直接PUTする（api-spec.md）。
func (h *AttachmentHandler) CreateAttachment(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)

	if h.r2 == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage_not_configured"})
		return
	}

	var req createAttachmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_name, content_type, size_bytes are required"})
		return
	}
	if req.SizeBytes > maxAttachmentSizeBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_too_large"})
		return
	}

	ctx := c.Request.Context()
	storageKey := fmt.Sprintf("%s/%s/%s_%s", t.WorkspaceID, t.ID, uuid.NewString(), req.FileName)

	created, err := h.client.Attachment.Create().
		SetTaskID(t.ID).
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

	row := attachmentJSON(created)
	row["upload_url"] = uploadURL
	c.JSON(http.StatusCreated, row)
}

// DeleteAttachment は DELETE /attachments/:attachment_id。:task_idを含まないパスのため、
// TaskVisibleUserIDs（既存のタスク閲覧権限ヘルパー）で権限を個別確認する。
func (h *AttachmentHandler) DeleteAttachment(c *gin.Context) {
	u := middleware.CurrentUser(c)

	attachmentID, err := uuid.Parse(c.Param("attachment_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "attachment not found"})
		return
	}

	ctx := c.Request.Context()

	a, err := h.client.Attachment.Query().
		Where(attachment.IDEQ(attachmentID)).
		WithTask().
		Only(ctx)
	if err != nil || a.Edges.Task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "attachment not found"})
		return
	}

	visibleUserIDs, err := middleware.TaskVisibleUserIDs(ctx, h.client, a.Edges.Task)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check permission"})
		return
	}
	allowed := false
	for _, id := range visibleUserIDs {
		if id == u.ID {
			allowed = true
			break
		}
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	if h.r2 != nil {
		if err := h.r2.DeleteObject(ctx, a.StorageKey); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete file from storage"})
			return
		}
	}
	if err := h.client.Attachment.DeleteOneID(a.ID).Exec(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete attachment"})
		return
	}
	c.Status(http.StatusNoContent)
}
