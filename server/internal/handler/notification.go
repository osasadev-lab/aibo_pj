package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"entgo.io/ent/dialect/sql"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/notification"
	"github.com/osasadev-lab/aibo_pj/server/ent/pushsubscription"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/pushdelivery"
)

// NotificationHandler は /notifications 配下を扱う。
type NotificationHandler struct {
	client  *ent.Client
	pushCfg pushdelivery.Config
}

func NewNotificationHandler(client *ent.Client, pushCfg pushdelivery.Config) *NotificationHandler {
	return &NotificationHandler{client: client, pushCfg: pushCfg}
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

const notificationDefaultLimit = 20
const notificationMaxLimit = 100

// List は GET /notifications。?unread=true で未読のみ。
// ページネーション（2026-08-27追加）：`limit`（既定20、最大100）・`before`
// （通知ID、これより古いものを返す）。レスポンスは`{items, has_more}`。
// 常に作成日時の降順（最新が先頭）で返すため、フロント側は「もっと見る」で
// 末尾の通知IDを`before`に渡せば続きを取得できる。
func (h *NotificationHandler) List(c *gin.Context) {
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	limit := parseLimit(c.Query("limit"), notificationDefaultLimit, notificationMaxLimit)

	query := h.client.Notification.Query().
		Where(notification.UserIDEQ(u.ID)).
		Order(notification.ByCreatedAt(sql.OrderDesc()), notification.ByID(sql.OrderDesc()))
	if c.Query("unread") == "true" {
		query = query.Where(notification.ReadAtIsNil())
	}
	if before := c.Query("before"); before != "" {
		beforeID, err := uuid.Parse(before)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before"})
			return
		}
		cursor, err := h.client.Notification.Query().
			Where(notification.IDEQ(beforeID), notification.UserIDEQ(u.ID)).
			Only(ctx)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before"})
			return
		}
		query = query.Where(notification.Or(
			notification.CreatedAtLT(cursor.CreatedAt),
			notification.And(notification.CreatedAtEQ(cursor.CreatedAt), notification.IDLT(cursor.ID)),
		))
	}

	// limit+1件取得し、実際にlimitを超えていれば「まだ続きがある」と判定する
	// （COUNTクエリを別途発行せずに済む定番の手法）。
	notifications, err := query.Limit(limit + 1).All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list notifications"})
		return
	}
	hasMore := len(notifications) > limit
	if hasMore {
		notifications = notifications[:limit]
	}

	out := make([]gin.H, 0, len(notifications))
	for _, n := range notifications {
		out = append(out, notificationJSON(n))
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "has_more": hasMore})
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

// PushPublicKey は GET /notifications/push-public-key（M7追加）。VAPID公開鍵のみ返す
// （Next.js側に鍵をハードコードせず済むように、設計判断9）。
func (h *NotificationHandler) PushPublicKey(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"public_key": h.pushCfg.VAPIDPublicKey})
}

type subscribeRequest struct {
	Endpoint string `json:"endpoint" binding:"required"`
	Keys     struct {
		P256dh string `json:"p256dh" binding:"required"`
		Auth   string `json:"auth" binding:"required"`
	} `json:"keys" binding:"required"`
}

// Subscribe は POST /notifications/subscribe（M7追加）。endpointをキーに既存行を
// 検索し、あれば鍵を更新（ローテーション対応）、無ければ新規作成する。
func (h *NotificationHandler) Subscribe(c *gin.Context) {
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	var req subscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	existing, err := h.client.PushSubscription.Query().
		Where(pushsubscription.EndpointEQ(req.Endpoint)).
		Only(ctx)
	switch {
	case err == nil:
		if _, err := h.client.PushSubscription.UpdateOneID(existing.ID).
			SetUserID(u.ID).
			SetP256dh(req.Keys.P256dh).
			SetAuth(req.Keys.Auth).
			Save(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update subscription"})
			return
		}
	case ent.IsNotFound(err):
		if _, err := h.client.PushSubscription.Create().
			SetUserID(u.ID).
			SetEndpoint(req.Endpoint).
			SetP256dh(req.Keys.P256dh).
			SetAuth(req.Keys.Auth).
			Save(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create subscription"})
			return
		}
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save subscription"})
		return
	}
	c.Status(http.StatusNoContent)
}

type unsubscribeRequest struct {
	Endpoint string `json:"endpoint" binding:"required"`
}

// Unsubscribe は DELETE /notifications/subscribe（M7追加）。本人の該当行のみ削除する。
func (h *NotificationHandler) Unsubscribe(c *gin.Context) {
	u := middleware.CurrentUser(c)

	var req unsubscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if _, err := h.client.PushSubscription.Delete().
		Where(pushsubscription.EndpointEQ(req.Endpoint), pushsubscription.UserIDEQ(u.ID)).
		Exec(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete subscription"})
		return
	}
	c.Status(http.StatusNoContent)
}
