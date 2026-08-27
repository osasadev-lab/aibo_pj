package handler

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/dmattachment"
	"github.com/osasadev-lab/aibo_pj/server/ent/dmchannel"
	"github.com/osasadev-lab/aibo_pj/server/ent/dmchannelmember"
	"github.com/osasadev-lab/aibo_pj/server/ent/dmmessage"
	"github.com/osasadev-lab/aibo_pj/server/ent/reaction"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/pushdelivery"
	"github.com/osasadev-lab/aibo_pj/server/internal/storage"
)

// DMHandler は /workspaces/:workspace_id/dm/*, /dm/channels/*, /dm/messages/:message_id/attachments,
// /dm-attachments/:attachment_id を扱う（docs/aibo/m8.5-implementation-plan.md）。
type DMHandler struct {
	client      *ent.Client
	r2          *storage.R2Client
	pushCfg     pushdelivery.Config
	frontendURL string
}

func NewDMHandler(client *ent.Client, r2 *storage.R2Client, pushCfg pushdelivery.Config, frontendURL string) *DMHandler {
	return &DMHandler{client: client, r2: r2, pushCfg: pushCfg, frontendURL: frontendURL}
}

// dmKey は1:1チャンネルの正規化キー（ソートした2人分のuser_idを連結）。
func dmKey(a, b uuid.UUID) string {
	as, bs := a.String(), b.String()
	if as > bs {
		as, bs = bs, as
	}
	return as + ":" + bs
}

func memberJSON(u *ent.User) gin.H {
	return gin.H{"user_id": u.ID, "name": u.Name, "avatar_url": u.AvatarURL}
}

// displayName はチャンネルの表示名。グループはname（未設定ならメンバー名を結合）、
// 1:1は相手の名前を使う。
func displayName(ch *ent.DMChannel, members []*ent.User, selfID uuid.UUID) string {
	if ch.IsGroup {
		if ch.Name != nil && *ch.Name != "" {
			return *ch.Name
		}
		names := make([]string, 0, len(members))
		for _, m := range members {
			if m.ID == selfID {
				continue
			}
			names = append(names, m.Name)
		}
		if len(names) > 3 {
			return strings.Join(names[:3], "、") + fmt.Sprintf(" 他%d名", len(names)-3)
		}
		return strings.Join(names, "、")
	}
	for _, m := range members {
		if m.ID != selfID {
			return m.Name
		}
	}
	// 相手が自分自身のみ＝セルフDM（自分専用のメモ代わりチャンネル）。
	for _, m := range members {
		if m.ID == selfID {
			return m.Name + "（自分）"
		}
	}
	return "" // 自分しかいない不整合データの保険（通常発生しない）
}

// ListChannels は GET /workspaces/:workspace_id/dm/channels。
// 自分が参加しているチャンネルを最新メッセージ順に返す。
func (h *DMHandler) ListChannels(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	myMemberships, err := h.client.DMChannelMember.Query().
		Where(dmchannelmember.UserIDEQ(u.ID), dmchannelmember.HasChannelWith(dmchannel.WorkspaceIDEQ(m.WorkspaceID))).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list channels"})
		return
	}
	if len(myMemberships) == 0 {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}

	channelIDs := make([]uuid.UUID, 0, len(myMemberships))
	lastReadByChannel := make(map[uuid.UUID]*ent.DMChannelMember, len(myMemberships))
	for _, mem := range myMemberships {
		channelIDs = append(channelIDs, mem.ChannelID)
		lastReadByChannel[mem.ChannelID] = mem
	}

	channels, err := h.client.DMChannel.Query().
		Where(dmchannel.IDIn(channelIDs...)).
		WithMembers(func(q *ent.DMChannelMemberQuery) { q.WithUser() }).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list channels"})
		return
	}

	latestMessages, err := h.client.DMMessage.Query().
		Where(dmmessage.ChannelIDIn(channelIDs...)).
		WithUser().
		Order(dmmessage.ByCreatedAt(sql.OrderDesc())).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list channels"})
		return
	}
	latestByChannel := make(map[uuid.UUID]*ent.DMMessage, len(channels))
	for _, msg := range latestMessages {
		if _, ok := latestByChannel[msg.ChannelID]; !ok {
			latestByChannel[msg.ChannelID] = msg
		}
	}

	// 表示順（最新メッセージ降順）を確定するため、まずtime.Timeを保持したまま
	// ソートし、その後にgin.Hへ整形する（JSON化後のfmt.Sprint比較は小数秒の
	// 桁数がまちまちで辞書順とずれるため避ける）。
	sort.SliceStable(channels, func(i, j int) bool {
		return channelUpdatedAt(channels[i], latestByChannel).After(channelUpdatedAt(channels[j], latestByChannel))
	})

	out := make([]gin.H, 0, len(channels))
	for _, ch := range channels {
		users := make([]*ent.User, 0, len(ch.Edges.Members))
		memberRows := make([]gin.H, 0, len(ch.Edges.Members))
		for _, cm := range ch.Edges.Members {
			if cm.Edges.User == nil {
				continue
			}
			users = append(users, cm.Edges.User)
			memberRows = append(memberRows, memberJSON(cm.Edges.User))
		}

		row := gin.H{
			"id":           ch.ID,
			"is_group":     ch.IsGroup,
			"name":         ch.Name,
			"display_name": displayName(ch, users, u.ID),
			"members":      memberRows,
			"updated_at":   channelUpdatedAt(ch, latestByChannel),
		}
		if last, ok := latestByChannel[ch.ID]; ok {
			senderName := ""
			if last.Edges.User != nil {
				senderName = last.Edges.User.Name
			}
			body := ""
			if last.Body != nil {
				body = *last.Body
			}
			row["last_message"] = gin.H{"body": body, "sender_name": senderName, "created_at": last.CreatedAt}
			myLastRead := lastReadByChannel[ch.ID].LastReadAt
			row["unread"] = myLastRead == nil || last.CreatedAt.After(*myLastRead)
		} else {
			row["last_message"] = nil
			row["unread"] = false
		}
		out = append(out, row)
	}

	c.JSON(http.StatusOK, out)
}

// channelUpdatedAt はチャンネルの並び替え・表示用の基準時刻（最新メッセージが
// あればその時刻、無ければチャンネル作成時刻）。
func channelUpdatedAt(ch *ent.DMChannel, latestByChannel map[uuid.UUID]*ent.DMMessage) time.Time {
	if last, ok := latestByChannel[ch.ID]; ok {
		return last.CreatedAt
	}
	return ch.CreatedAt
}

type createDMChannelRequest struct {
	UserIDs []uuid.UUID `json:"user_ids" binding:"required"`
	Name    *string     `json:"name"`
}

// CreateChannel は POST /workspaces/:workspace_id/dm/channels。
// user_idsが1人なら1:1（dm_keyでfind-or-create）、2人以上なら常に新規グループを作る。
func (h *DMHandler) CreateChannel(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	var req createDMChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids is required"})
		return
	}

	seen := map[uuid.UUID]struct{}{}
	otherIDs := make([]uuid.UUID, 0, len(req.UserIDs))
	selfSelected := false
	for _, id := range req.UserIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if id == u.ID {
			selfSelected = true
			continue
		}
		otherIDs = append(otherIDs, id)
	}
	if len(otherIDs) == 0 && !selfSelected {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids must include at least one member"})
		return
	}

	count, err := h.client.WorkspaceMember.Query().
		Where(workspacemember.WorkspaceIDEQ(m.WorkspaceID), workspacemember.UserIDIn(otherIDs...)).
		Count(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate members"})
		return
	}
	if count != len(otherIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids must all be workspace members"})
		return
	}

	// 自分だけを選んだ場合：自分専用チャンネル（メモ代わりのセルフDM）を
	// dm_keyでfind-or-createする（ユーザー要望、2026-08-27追加）。
	if len(otherIDs) == 0 && selfSelected {
		key := dmKey(u.ID, u.ID)
		existing, err := h.client.DMChannel.Query().
			Where(dmchannel.WorkspaceIDEQ(m.WorkspaceID), dmchannel.DmKeyEQ(key)).
			Only(ctx)
		if err == nil {
			c.JSON(http.StatusOK, gin.H{"id": existing.ID})
			return
		}
		if !ent.IsNotFound(err) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create channel"})
			return
		}

		var created *ent.DMChannel
		err = withTx(ctx, h.client, func(tx *ent.Tx) error {
			ch, err := tx.DMChannel.Create().
				SetWorkspaceID(m.WorkspaceID).
				SetIsGroup(false).
				SetDmKey(key).
				SetCreatedBy(u.ID).
				Save(ctx)
			if err != nil {
				return err
			}
			if err := tx.DMChannelMember.Create().SetChannelID(ch.ID).SetUserID(u.ID).Exec(ctx); err != nil {
				return err
			}
			created = ch
			return nil
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create channel"})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"id": created.ID})
		return
	}

	// 1:1（相手が1人）はdm_keyで既存チャンネルを再利用する。
	if len(otherIDs) == 1 {
		key := dmKey(u.ID, otherIDs[0])
		existing, err := h.client.DMChannel.Query().
			Where(dmchannel.WorkspaceIDEQ(m.WorkspaceID), dmchannel.DmKeyEQ(key)).
			Only(ctx)
		if err == nil {
			c.JSON(http.StatusOK, gin.H{"id": existing.ID})
			return
		}
		if !ent.IsNotFound(err) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create channel"})
			return
		}

		var created *ent.DMChannel
		err = withTx(ctx, h.client, func(tx *ent.Tx) error {
			ch, err := tx.DMChannel.Create().
				SetWorkspaceID(m.WorkspaceID).
				SetIsGroup(false).
				SetDmKey(key).
				SetCreatedBy(u.ID).
				Save(ctx)
			if err != nil {
				return err
			}
			if err := tx.DMChannelMember.Create().SetChannelID(ch.ID).SetUserID(u.ID).Exec(ctx); err != nil {
				return err
			}
			if err := tx.DMChannelMember.Create().SetChannelID(ch.ID).SetUserID(otherIDs[0]).Exec(ctx); err != nil {
				return err
			}
			created = ch
			return nil
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create channel"})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"id": created.ID})
		return
	}

	// グループ（相手が2人以上）は常に新規作成する。
	var created *ent.DMChannel
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		builder := tx.DMChannel.Create().
			SetWorkspaceID(m.WorkspaceID).
			SetIsGroup(true).
			SetCreatedBy(u.ID)
		if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
			builder = builder.SetName(strings.TrimSpace(*req.Name))
		}
		ch, err := builder.Save(ctx)
		if err != nil {
			return err
		}
		allMembers := append([]uuid.UUID{u.ID}, otherIDs...)
		for _, id := range allMembers {
			if err := tx.DMChannelMember.Create().SetChannelID(ch.ID).SetUserID(id).Exec(ctx); err != nil {
				return err
			}
		}
		created = ch
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create channel"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": created.ID})
}

// GetChannel は GET /dm/channels/:channel_id。
func (h *DMHandler) GetChannel(c *gin.Context) {
	ch := middleware.CurrentDMChannel(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	full, err := h.client.DMChannel.Query().
		Where(dmchannel.IDEQ(ch.ID)).
		WithMembers(func(q *ent.DMChannelMemberQuery) { q.WithUser() }).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load channel"})
		return
	}

	users := make([]*ent.User, 0, len(full.Edges.Members))
	memberRows := make([]gin.H, 0, len(full.Edges.Members))
	for _, cm := range full.Edges.Members {
		if cm.Edges.User == nil {
			continue
		}
		users = append(users, cm.Edges.User)
		memberRows = append(memberRows, memberJSON(cm.Edges.User))
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           full.ID,
		"is_group":     full.IsGroup,
		"name":         full.Name,
		"display_name": displayName(full, users, u.ID),
		"members":      memberRows,
	})
}

type renameDMChannelRequest struct {
	Name string `json:"name" binding:"required"`
}

// RenameChannel は PATCH /dm/channels/:channel_id。グループのみ、メンバーなら誰でも変更可。
func (h *DMHandler) RenameChannel(c *gin.Context) {
	ch := middleware.CurrentDMChannel(c)
	if !ch.IsGroup {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not_applicable_for_direct_channel"})
		return
	}

	var req renameDMChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if _, err := h.client.DMChannel.UpdateOneID(ch.ID).SetName(strings.TrimSpace(req.Name)).Save(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rename channel"})
		return
	}
	c.Status(http.StatusNoContent)
}

type addDMMembersRequest struct {
	UserIDs []uuid.UUID `json:"user_ids" binding:"required"`
}

// AddMembers は POST /dm/channels/:channel_id/members。グループのみ、メンバーなら誰でも追加可。
func (h *DMHandler) AddMembers(c *gin.Context) {
	ch := middleware.CurrentDMChannel(c)
	if !ch.IsGroup {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not_applicable_for_direct_channel"})
		return
	}

	var req addDMMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids is required"})
		return
	}

	ctx := c.Request.Context()
	count, err := h.client.WorkspaceMember.Query().
		Where(workspacemember.WorkspaceIDEQ(ch.WorkspaceID), workspacemember.UserIDIn(req.UserIDs...)).
		Count(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate members"})
		return
	}
	if count != len(req.UserIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids must all be workspace members"})
		return
	}

	for _, id := range req.UserIDs {
		exists, err := h.client.DMChannelMember.Query().
			Where(dmchannelmember.ChannelIDEQ(ch.ID), dmchannelmember.UserIDEQ(id)).
			Exist(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add members"})
			return
		}
		if exists {
			continue
		}
		if err := h.client.DMChannelMember.Create().SetChannelID(ch.ID).SetUserID(id).Exec(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add members"})
			return
		}
	}
	c.Status(http.StatusNoContent)
}

// LeaveChannel は DELETE /dm/channels/:channel_id/members/me。グループのみ、自分のみ退出可。
func (h *DMHandler) LeaveChannel(c *gin.Context) {
	ch := middleware.CurrentDMChannel(c)
	u := middleware.CurrentUser(c)
	if !ch.IsGroup {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot_leave_direct_channel"})
		return
	}

	if _, err := h.client.DMChannelMember.Delete().
		Where(dmchannelmember.ChannelIDEQ(ch.ID), dmchannelmember.UserIDEQ(u.ID)).
		Exec(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to leave channel"})
		return
	}
	c.Status(http.StatusNoContent)
}

const dmMessageDefaultLimit = 30
const dmMessageMaxLimit = 100

func dmMessageJSON(msg *ent.DMMessage) gin.H {
	row := gin.H{
		"id":         msg.ID,
		"channel_id": msg.ChannelID,
		"user_id":    msg.UserID,
		"body":       msg.Body,
		"created_at": msg.CreatedAt,
		// 編集済みかどうかはフロントでcreated_at!==updated_atで判定する
		// （commentsと同じ考え方、2026-08-27追加）。
		"updated_at": msg.UpdatedAt,
	}
	if msg.Edges.User != nil {
		row["user_name"] = msg.Edges.User.Name
		row["user_avatar_url"] = msg.Edges.User.AvatarURL
	}
	atts := make([]gin.H, 0, len(msg.Edges.Attachments))
	for _, a := range msg.Edges.Attachments {
		atts = append(atts, gin.H{
			"id": a.ID, "file_name": a.FileName, "size_bytes": a.SizeBytes, "content_type": a.ContentType,
		})
	}
	row["attachments"] = atts
	return row
}

// ListMessages は GET /dm/channels/:channel_id/messages。
// 既定は直近limit件、`before=<message_id>`でそれより古いものを遡る
// （commentsと同じ考え方だが、DM通知は特定メッセージへのジャンプを要求しないため
// aroundモードは持たない、docs/aibo/m8.5-implementation-plan.md）。
func (h *DMHandler) ListMessages(c *gin.Context) {
	ch := middleware.CurrentDMChannel(c)
	ctx := c.Request.Context()

	limit := parseLimit(c.Query("limit"), dmMessageDefaultLimit, dmMessageMaxLimit)

	query := h.client.DMMessage.Query().
		Where(dmmessage.ChannelIDEQ(ch.ID)).
		WithUser().
		WithAttachments().
		Order(dmmessage.ByCreatedAt(sql.OrderDesc()), dmmessage.ByID(sql.OrderDesc()))

	if before := c.Query("before"); before != "" {
		beforeID, err := uuid.Parse(before)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before"})
			return
		}
		cursor, err := h.client.DMMessage.Query().Where(dmmessage.IDEQ(beforeID), dmmessage.ChannelIDEQ(ch.ID)).Only(ctx)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before"})
			return
		}
		query = query.Where(dmmessage.Or(
			dmmessage.CreatedAtLT(cursor.CreatedAt),
			dmmessage.And(dmmessage.CreatedAtEQ(cursor.CreatedAt), dmmessage.IDLT(cursor.ID)),
		))
	}

	messages, err := query.Limit(limit + 1).All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list messages"})
		return
	}
	hasMoreOlder := len(messages) > limit
	if hasMoreOlder {
		messages = messages[:limit]
	}
	sort.SliceStable(messages, func(i, j int) bool { return messages[i].CreatedAt.Before(messages[j].CreatedAt) })

	reactionsByMessage, err := loadDMReactionsByChannel(ctx, h.client, ch.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list messages"})
		return
	}
	out := make([]gin.H, 0, len(messages))
	for _, msg := range messages {
		row := dmMessageJSON(msg)
		row["reactions"] = orEmptyReactions(reactionsByMessage[msg.ID])
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "has_more_older": hasMoreOlder})
}

type createDMMessageRequest struct {
	Body string `json:"body"`
}

// CreateMessage は POST /dm/channels/:channel_id/messages。
// 自分以外の全メンバーへ通知（type: dm_message）＋Web Pushを作成する。
func (h *DMHandler) CreateMessage(c *gin.Context) {
	ch := middleware.CurrentDMChannel(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	var req createDMMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	body := strings.TrimSpace(req.Body)

	var created *ent.DMMessage
	err := withTx(ctx, h.client, func(tx *ent.Tx) error {
		builder := tx.DMMessage.Create().SetChannelID(ch.ID).SetUserID(u.ID)
		if body != "" {
			builder = builder.SetBody(body)
		}
		msg, err := builder.Save(ctx)
		if err != nil {
			return err
		}
		// 送信は既読相当として自分のlast_read_atも更新する（自分の投稿を未読扱いしないため）。
		if _, err := tx.DMChannelMember.Update().
			Where(dmchannelmember.ChannelIDEQ(ch.ID), dmchannelmember.UserIDEQ(u.ID)).
			SetLastReadAt(msg.CreatedAt).
			Save(ctx); err != nil {
			return err
		}
		msg.Edges.User = u
		created = msg
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send message"})
		return
	}

	// 通知・Web Push（自分以外の全メンバー）。DBコミット後に行う。
	memberIDs, err := h.client.DMChannelMember.Query().
		Where(dmchannelmember.ChannelIDEQ(ch.ID), dmchannelmember.UserIDNEQ(u.ID)).
		All(ctx)
	if err == nil && len(memberIDs) > 0 {
		payload := map[string]any{
			"channel_id":   ch.ID,
			"message_id":   created.ID,
			"sender_id":    u.ID,
			"sender_name":  u.Name,
			"excerpt":      excerpt(body, 100),
			"is_group":     ch.IsGroup,
			"channel_name": ch.Name,
		}
		var pendingPush []pushdelivery.Item
		_ = withTx(ctx, h.client, func(tx *ent.Tx) error {
			for _, mem := range memberIDs {
				if _, err := tx.Notification.Create().
					SetUserID(mem.UserID).
					SetType("dm_message").
					SetPayload(payload).
					Save(ctx); err != nil {
					return err
				}
				pendingPush = append(pendingPush, pushdelivery.BuildItem(mem.UserID, h.frontendURL, ch.WorkspaceID.String(), "dm_message", payload))
			}
			return nil
		})
		pushdelivery.Async(h.client, h.pushCfg, pendingPush)
	}

	c.JSON(http.StatusCreated, dmMessageJSON(created))
}

type updateDMMessageRequest struct {
	Body string `json:"body" binding:"required"`
}

// UpdateMessage は PATCH /dm/channels/:channel_id/messages/:message_id。
// 送信者本人のみ編集可（ユーザー要望、2026-08-27追加）。commentsと同じく
// updated_atの自動更新で「編集済み」をフロント側から判定できるようにする。
func (h *DMHandler) UpdateMessage(c *gin.Context) {
	ch := middleware.CurrentDMChannel(c)
	u := middleware.CurrentUser(c)

	messageID, err := uuid.Parse(c.Param("message_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}

	var req updateDMMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body is required"})
		return
	}

	ctx := c.Request.Context()
	existing, err := h.client.DMMessage.Query().
		Where(dmmessage.IDEQ(messageID), dmmessage.ChannelIDEQ(ch.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}
	if existing.UserID != u.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	updated, err := h.client.DMMessage.UpdateOneID(messageID).
		SetBody(strings.TrimSpace(req.Body)).
		Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update message"})
		return
	}
	updated, err = h.client.DMMessage.Query().
		Where(dmmessage.IDEQ(messageID)).
		WithAttachments().
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load message"})
		return
	}
	updated.Edges.User = u
	c.JSON(http.StatusOK, dmMessageJSON(updated))
}

// DeleteMessage は DELETE /dm/channels/:channel_id/messages/:message_id。
// 送信者本人のみ削除可。添付ファイル（DB行・R2オブジェクト）も連動削除する。
func (h *DMHandler) DeleteMessage(c *gin.Context) {
	ch := middleware.CurrentDMChannel(c)
	u := middleware.CurrentUser(c)

	messageID, err := uuid.Parse(c.Param("message_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}

	ctx := c.Request.Context()
	existing, err := h.client.DMMessage.Query().
		Where(dmmessage.IDEQ(messageID), dmmessage.ChannelIDEQ(ch.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}
	if existing.UserID != u.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	var attachmentKeys []string
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		atts, err := tx.DMAttachment.Query().Where(dmattachment.DmMessageIDEQ(messageID)).All(ctx)
		if err != nil {
			return err
		}
		for _, a := range atts {
			attachmentKeys = append(attachmentKeys, a.StorageKey)
		}
		if _, err := tx.DMAttachment.Delete().Where(dmattachment.DmMessageIDEQ(messageID)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.Reaction.Delete().
			Where(reaction.TargetTypeEQ(reaction.TargetTypeDmMessage), reaction.TargetIDEQ(messageID)).
			Exec(ctx); err != nil {
			return err
		}
		return tx.DMMessage.DeleteOneID(messageID).Exec(ctx)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete message"})
		return
	}
	deleteR2Objects(ctx, h.r2, attachmentKeys)
	c.Status(http.StatusNoContent)
}

// MarkRead は PATCH /dm/channels/:channel_id/read。
func (h *DMHandler) MarkRead(c *gin.Context) {
	ch := middleware.CurrentDMChannel(c)
	u := middleware.CurrentUser(c)

	if _, err := h.client.DMChannelMember.Update().
		Where(dmchannelmember.ChannelIDEQ(ch.ID), dmchannelmember.UserIDEQ(u.ID)).
		SetLastReadAt(time.Now()).
		Save(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark channel as read"})
		return
	}
	c.Status(http.StatusNoContent)
}

var errNotChannelMember = errors.New("not a channel member")

// loadMessageForMember はmessage_idからDMMessageを解決し、呼び出しユーザーが
// そのチャンネルのメンバーであることを検証する（添付ファイルAPIはchannel_idを
// パスに含まないため、RequireDMChannelAccessではなくここで個別に確認する）。
func (h *DMHandler) loadMessageForMember(c *gin.Context) (*ent.DMMessage, error) {
	u := middleware.CurrentUser(c)
	messageID, err := uuid.Parse(c.Param("message_id"))
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	msg, err := h.client.DMMessage.Query().Where(dmmessage.IDEQ(messageID)).Only(ctx)
	if err != nil {
		return nil, err
	}
	isMember, err := h.client.DMChannelMember.Query().
		Where(dmchannelmember.ChannelIDEQ(msg.ChannelID), dmchannelmember.UserIDEQ(u.ID)).
		Exist(ctx)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, errNotChannelMember
	}
	return msg, nil
}

func dmAttachmentJSON(a *ent.DMAttachment) gin.H {
	return gin.H{
		"id": a.ID, "dm_message_id": a.DmMessageID, "uploaded_by": a.UploadedBy,
		"file_name": a.FileName, "size_bytes": a.SizeBytes, "content_type": a.ContentType,
	}
}

type createDMAttachmentRequest struct {
	FileName    string `json:"file_name" binding:"required"`
	ContentType string `json:"content_type" binding:"required"`
	SizeBytes   int64  `json:"size_bytes" binding:"required"`
}

// CreateAttachment は POST /dm/messages/:message_id/attachments。
// task/attachments と同じ2段階方式・25MB上限（maxAttachmentSizeBytes、attachment.go）。
func (h *DMHandler) CreateAttachment(c *gin.Context) {
	msg, err := h.loadMessageForMember(c)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}
	u := middleware.CurrentUser(c)

	if h.r2 == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "storage_not_configured"})
		return
	}

	var req createDMAttachmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_name, content_type, size_bytes are required"})
		return
	}
	if req.SizeBytes > maxAttachmentSizeBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_too_large"})
		return
	}

	ctx := c.Request.Context()
	storageKey := fmt.Sprintf("dm/%s/%s/%s_%s", msg.ChannelID, msg.ID, uuid.NewString(), req.FileName)

	created, err := h.client.DMAttachment.Create().
		SetDmMessageID(msg.ID).
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

	row := dmAttachmentJSON(created)
	row["upload_url"] = uploadURL
	c.JSON(http.StatusCreated, row)
}

// DeleteAttachment は DELETE /dm-attachments/:attachment_id。そのチャンネルのメンバーなら削除可
// （attachments.goの「閲覧できる全員が削除可」という既存の意図的仕様を踏襲）。
func (h *DMHandler) DeleteAttachment(c *gin.Context) {
	u := middleware.CurrentUser(c)

	attachmentID, err := uuid.Parse(c.Param("attachment_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "attachment not found"})
		return
	}

	ctx := c.Request.Context()
	a, err := h.client.DMAttachment.Query().Where(dmattachment.IDEQ(attachmentID)).WithMessage().Only(ctx)
	if err != nil || a.Edges.Message == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "attachment not found"})
		return
	}

	isMember, err := h.client.DMChannelMember.Query().
		Where(dmchannelmember.ChannelIDEQ(a.Edges.Message.ChannelID), dmchannelmember.UserIDEQ(u.ID)).
		Exist(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check permission"})
		return
	}
	if !isMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	if h.r2 != nil {
		if err := h.r2.DeleteObject(ctx, a.StorageKey); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete file from storage"})
			return
		}
	}
	if err := h.client.DMAttachment.DeleteOneID(a.ID).Exec(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete attachment"})
		return
	}
	c.Status(http.StatusNoContent)
}
