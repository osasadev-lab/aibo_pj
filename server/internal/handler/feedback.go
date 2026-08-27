package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/internal/feedbackmail"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

// FeedbackHandler は POST /feedback を扱う（M8）。
type FeedbackHandler struct {
	client  *ent.Client
	mailCfg feedbackmail.Config
}

func NewFeedbackHandler(client *ent.Client, mailCfg feedbackmail.Config) *FeedbackHandler {
	return &FeedbackHandler{client: client, mailCfg: mailCfg}
}

type createFeedbackRequest struct {
	Body        string     `json:"body" binding:"required"`
	WorkspaceID *uuid.UUID `json:"workspace_id"`
	PagePath    string     `json:"page_path"`
}

// Create は POST /feedback。RequireAuthのみ（ワークスペースメンバーシップは問わない）。
// DBに一次記録として保存した上で、コミット後にベストエフォートでosasadev@gmail.com宛に
// メール通知する（docs/aibo/m8-implementation-plan.md スコープ追加D）。
func (h *FeedbackHandler) Create(c *gin.Context) {
	u := middleware.CurrentUser(c)

	var req createFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body is required"})
		return
	}

	builder := h.client.Feedback.Create().
		SetUserID(u.ID).
		SetBody(req.Body)
	if req.WorkspaceID != nil {
		builder = builder.SetWorkspaceID(*req.WorkspaceID)
	}
	if req.PagePath != "" {
		builder = builder.SetPagePath(req.PagePath)
	}

	fb, err := builder.Save(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save feedback"})
		return
	}

	feedbackmail.Async(h.mailCfg, feedbackmail.Item{
		SenderName:  u.Name,
		SenderEmail: u.Email,
		WorkspaceID: req.WorkspaceID,
		PagePath:    req.PagePath,
		Body:        req.Body,
		CreatedAt:   fb.CreatedAt,
	})

	c.JSON(http.StatusCreated, gin.H{"id": fb.ID})
}
