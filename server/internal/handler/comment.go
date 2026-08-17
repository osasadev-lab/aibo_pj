package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/comment"
	"github.com/osasadev-lab/aibo_pj/server/ent/user"
	"github.com/osasadev-lab/aibo_pj/server/internal/activity"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

// CommentHandler は /tasks/:task_id/comments, /tasks/:task_id/mentionable-members を扱う。
type CommentHandler struct {
	client *ent.Client
}

func NewCommentHandler(client *ent.Client) *CommentHandler {
	return &CommentHandler{client: client}
}

// MentionableMembers は GET /tasks/:task_id/mentionable-members。
// そのタスクを閲覧できる範囲のメンバーのみを`@`メンション候補として返す。
func (h *CommentHandler) MentionableMembers(c *gin.Context) {
	t := middleware.CurrentTask(c)
	ctx := c.Request.Context()

	ids, err := middleware.TaskVisibleUserIDs(ctx, h.client, t)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list mentionable members"})
		return
	}

	users, err := h.client.User.Query().Where(user.IDIn(ids...)).All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list mentionable members"})
		return
	}

	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		out = append(out, gin.H{
			"id":         u.ID,
			"user_id":    u.ID,
			"name":       u.Name,
			"email":      u.Email,
			"avatar_url": u.AvatarURL,
		})
	}
	c.JSON(http.StatusOK, out)
}

func commentJSON(cm *ent.Comment) gin.H {
	row := gin.H{
		"id":         cm.ID,
		"task_id":    cm.TaskID,
		"user_id":    cm.UserID,
		"body":       cm.Body,
		"created_at": cm.CreatedAt,
	}
	if cm.Edges.User != nil {
		row["user_name"] = cm.Edges.User.Name
		row["user_avatar_url"] = cm.Edges.User.AvatarURL
	}
	return row
}

// ListComments は GET /tasks/:task_id/comments。初回表示用（以降はSupabase Realtime購読）。
func (h *CommentHandler) ListComments(c *gin.Context) {
	t := middleware.CurrentTask(c)

	comments, err := h.client.Comment.Query().
		Where(comment.TaskIDEQ(t.ID)).
		WithUser().
		Order(comment.ByCreatedAt()).
		All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list comments"})
		return
	}

	out := make([]gin.H, 0, len(comments))
	for _, cm := range comments {
		out = append(out, commentJSON(cm))
	}
	c.JSON(http.StatusOK, out)
}

type createCommentRequest struct {
	Body             string      `json:"body" binding:"required"`
	MentionedUserIDs []uuid.UUID `json:"mentioned_user_ids"`
}

// CreateComment は POST /tasks/:task_id/comments。
// 書き込みは必ずこのAPI経由（RLSはINSERTを許可しないため直接書き込み不可）。
// メンション先の検証・CommentMention作成・Notification作成・ActivityLog記録を
// 1トランザクションで行う。
func (h *CommentHandler) CreateComment(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)

	var req createCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body is required"})
		return
	}

	ctx := c.Request.Context()

	mentionIDs := dedupUUIDs(req.MentionedUserIDs)
	if len(mentionIDs) > 0 {
		visibleIDs, err := middleware.TaskVisibleUserIDs(ctx, h.client, t)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate mentions"})
			return
		}
		visibleSet := map[uuid.UUID]struct{}{}
		for _, id := range visibleIDs {
			visibleSet[id] = struct{}{}
		}
		for _, id := range mentionIDs {
			if _, ok := visibleSet[id]; !ok {
				c.JSON(http.StatusBadRequest, gin.H{"error": "mentioned_user_ids must be visible to this task"})
				return
			}
		}
	}

	var created *ent.Comment
	err := withTx(ctx, h.client, func(tx *ent.Tx) error {
		cm, err := tx.Comment.Create().
			SetTaskID(t.ID).
			SetUserID(u.ID).
			SetBody(req.Body).
			Save(ctx)
		if err != nil {
			return err
		}

		for _, uid := range mentionIDs {
			if _, err := tx.CommentMention.Create().
				SetCommentID(cm.ID).
				SetMentionedUserID(uid).
				Save(ctx); err != nil {
				return err
			}
			if _, err := tx.Notification.Create().
				SetUserID(uid).
				SetType("mentioned").
				SetPayload(map[string]any{
					"task_id":           t.ID,
					"comment_id":        cm.ID,
					"project_id":        t.ProjectID,
					"mentioned_by":      u.ID,
					"mentioned_by_name": u.Name,
					"excerpt":           excerpt(req.Body, 100),
				}).
				Save(ctx); err != nil {
				return err
			}
		}

		if err := activity.Record(ctx, tx, t.WorkspaceID, &t.ID, t.ProjectID, u.ID, "comment.created",
			map[string]any{"comment_id": cm.ID, "mentioned_user_ids": mentionIDs}); err != nil {
			return err
		}

		cm.Edges.User = u
		created = cm
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create comment"})
		return
	}

	c.JSON(http.StatusCreated, commentJSON(created))
}

func excerpt(s string, maxLen int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxLen {
		return string(r)
	}
	return string(r[:maxLen]) + "..."
}
