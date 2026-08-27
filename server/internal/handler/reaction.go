package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/comment"
	"github.com/osasadev-lab/aibo_pj/server/ent/dmmessage"
	"github.com/osasadev-lab/aibo_pj/server/ent/reaction"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

// ReactionHandler は /tasks/:task_id/reactions,
// /tasks/:task_id/comments/:comment_id/reactions,
// /dm/channels/:channel_id/messages/:message_id/reactions を扱う
// （タスク説明・コメント・DMメッセージへの絵文字リアクション、2026-08-28追加）。
type ReactionHandler struct {
	client *ent.Client
}

func NewReactionHandler(client *ent.Client) *ReactionHandler {
	return &ReactionHandler{client: client}
}

// 固定の絵文字セット（ユーザー確認済み：フル絵文字ピッカーではなく定番セット方式）。
const reactionEmojiSet = "👍 ❤️ 😂 😮 😢 🎉"

type toggleReactionRequest struct {
	Emoji string `json:"emoji" binding:"required,oneof=👍 ❤️ 😂 😮 😢 🎉"`
}

// orEmptyReactions はmapアクセスでキーが無い場合のnilスライスをJSON上`null`ではなく
// `[]`にするためのヘルパー（フロント側は常に配列を期待するため）。
func orEmptyReactions(rs []gin.H) []gin.H {
	if rs == nil {
		return []gin.H{}
	}
	return rs
}

func reactionJSON(r *ent.Reaction) gin.H {
	row := gin.H{
		"id":        r.ID,
		"target_id": r.TargetID,
		"user_id":   r.UserID,
		"emoji":     r.Emoji,
	}
	if r.Edges.User != nil {
		row["user_name"] = r.Edges.User.Name
	}
	return row
}

// loadTaskReactions はタスク説明欄自体へのリアクション一覧（GET /tasks/:task_id用）。
func loadTaskReactions(ctx context.Context, client *ent.Client, taskID uuid.UUID) ([]gin.H, error) {
	rows, err := client.Reaction.Query().
		Where(reaction.TargetTypeEQ(reaction.TargetTypeTask), reaction.TargetIDEQ(taskID)).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, reactionJSON(r))
	}
	return out, nil
}

// loadCommentReactionsByTask はタスク配下の全コメントのリアクションをcomment_id別に
// まとめて取得する（一覧のN+1回避、denormalizeしたtask_id列で一括取得）。
func loadCommentReactionsByTask(ctx context.Context, client *ent.Client, taskID uuid.UUID) (map[uuid.UUID][]gin.H, error) {
	rows, err := client.Reaction.Query().
		Where(reaction.TargetTypeEQ(reaction.TargetTypeComment), reaction.TaskIDEQ(taskID)).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID][]gin.H{}
	for _, r := range rows {
		out[r.TargetID] = append(out[r.TargetID], reactionJSON(r))
	}
	return out, nil
}

// loadDMReactionsByChannel はDMチャンネル配下の全メッセージのリアクションをmessage_id別に
// まとめて取得する。
func loadDMReactionsByChannel(ctx context.Context, client *ent.Client, channelID uuid.UUID) (map[uuid.UUID][]gin.H, error) {
	rows, err := client.Reaction.Query().
		Where(reaction.TargetTypeEQ(reaction.TargetTypeDmMessage), reaction.DmChannelIDEQ(channelID)).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID][]gin.H{}
	for _, r := range rows {
		out[r.TargetID] = append(out[r.TargetID], reactionJSON(r))
	}
	return out, nil
}

// toggle は対象へのリアクションを1人1つに保つ共通処理（ユーザー要望、2026-08-28）。
//   - 未リアクション → 新規作成（201）
//   - 同じ絵文字で既にリアクション済み → 解除（204）
//   - 別の絵文字で既にリアクション済み → 削除してから新しい絵文字で作り直す「変更」
//     （201）。delete→createの2イベントとしてRealtimeに流れるため、フロント側は
//     既存のINSERT/DELETEハンドラだけで変更を反映できる（UPDATE購読を増やさずに済む）。
func (h *ReactionHandler) toggle(
	c *gin.Context,
	targetType reaction.TargetType,
	targetID uuid.UUID,
	taskID *uuid.UUID,
	dmChannelID *uuid.UUID,
) {
	u := middleware.CurrentUser(c)
	var req toggleReactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "emoji must be one of: " + reactionEmojiSet})
		return
	}

	ctx := c.Request.Context()
	existing, err := h.client.Reaction.Query().
		Where(
			reaction.TargetTypeEQ(targetType),
			reaction.TargetIDEQ(targetID),
			reaction.UserIDEQ(u.ID),
		).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check reaction"})
		return
	}

	if existing != nil && existing.Emoji == req.Emoji {
		if err := h.client.Reaction.DeleteOneID(existing.ID).Exec(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove reaction"})
			return
		}
		c.Status(http.StatusNoContent)
		return
	}

	var created *ent.Reaction
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		if existing != nil {
			if err := tx.Reaction.DeleteOneID(existing.ID).Exec(ctx); err != nil {
				return err
			}
		}
		builder := tx.Reaction.Create().
			SetTargetType(targetType).
			SetTargetID(targetID).
			SetUserID(u.ID).
			SetEmoji(req.Emoji)
		if taskID != nil {
			builder = builder.SetTaskID(*taskID)
		}
		if dmChannelID != nil {
			builder = builder.SetDmChannelID(*dmChannelID)
		}
		var txErr error
		created, txErr = builder.Save(ctx)
		return txErr
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save reaction"})
		return
	}
	created.Edges.User = u
	c.JSON(http.StatusCreated, reactionJSON(created))
}

// ToggleTaskReaction は POST /tasks/:task_id/reactions（説明欄へのリアクション、タスク単位）。
func (h *ReactionHandler) ToggleTaskReaction(c *gin.Context) {
	t := middleware.CurrentTask(c)
	h.toggle(c, reaction.TargetTypeTask, t.ID, &t.ID, nil)
}

// ToggleCommentReaction は POST /tasks/:task_id/comments/:comment_id/reactions。
func (h *ReactionHandler) ToggleCommentReaction(c *gin.Context) {
	t := middleware.CurrentTask(c)
	commentID, err := uuid.Parse(c.Param("comment_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		return
	}
	exists, err := h.client.Comment.Query().
		Where(comment.IDEQ(commentID), comment.TaskIDEQ(t.ID)).
		Exist(c.Request.Context())
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		return
	}
	h.toggle(c, reaction.TargetTypeComment, commentID, &t.ID, nil)
}

// ToggleDMReaction は POST /dm/channels/:channel_id/messages/:message_id/reactions。
func (h *ReactionHandler) ToggleDMReaction(c *gin.Context) {
	ch := middleware.CurrentDMChannel(c)
	messageID, err := uuid.Parse(c.Param("message_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}
	exists, err := h.client.DMMessage.Query().
		Where(dmmessage.IDEQ(messageID), dmmessage.ChannelIDEQ(ch.ID)).
		Exist(c.Request.Context())
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}
	h.toggle(c, reaction.TargetTypeDmMessage, messageID, nil, &ch.ID)
}
