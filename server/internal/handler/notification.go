package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"entgo.io/ent/dialect/sql"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/notification"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

// NotificationHandler は /notifications 配下を扱う。
type NotificationHandler struct {
	client *ent.Client
}

func NewNotificationHandler(client *ent.Client) *NotificationHandler {
	return &NotificationHandler{client: client}
}

func notificationJSON(n *ent.Notification) gin.H {
	return gin.H{
		"id":         n.ID,
		"type":       n.Type,
		"payload":    n.Payload,
		"read_at":    n.ReadAt,
		"created_at": n.CreatedAt,
	}
}

// List は GET /notifications。?unread=true で未読のみ。
func (h *NotificationHandler) List(c *gin.Context) {
	u := middleware.CurrentUser(c)

	query := h.client.Notification.Query().
		Where(notification.UserIDEQ(u.ID)).
		Order(notification.ByCreatedAt(sql.OrderDesc()))
	if c.Query("unread") == "true" {
		query = query.Where(notification.ReadAtIsNil())
	}

	notifications, err := query.All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list notifications"})
		return
	}

	out := make([]gin.H, 0, len(notifications))
	for _, n := range notifications {
		out = append(out, notificationJSON(n))
	}
	c.JSON(http.StatusOK, out)
}

// MarkRead は PATCH /notifications/:notification_id/read。本人の通知のみ既読化できる。
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	u := middleware.CurrentUser(c)

	id, err := uuid.Parse(c.Param("notification_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
		return
	}

	ctx := c.Request.Context()
	n, err := h.client.Notification.Query().
		Where(notification.IDEQ(id), notification.UserIDEQ(u.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
		return
	}

	if _, err := h.client.Notification.UpdateOneID(n.ID).SetReadAt(time.Now()).Save(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark notification as read"})
		return
	}
	c.Status(http.StatusNoContent)
}
