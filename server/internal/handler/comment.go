package handler

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/comment"
	"github.com/osasadev-lab/aibo_pj/server/ent/commentmention"
	"github.com/osasadev-lab/aibo_pj/server/ent/predicate"
	"github.com/osasadev-lab/aibo_pj/server/ent/reaction"
	"github.com/osasadev-lab/aibo_pj/server/ent/user"
	"github.com/osasadev-lab/aibo_pj/server/internal/activity"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/pushdelivery"
)

// CommentHandler は /tasks/:task_id/comments, /tasks/:task_id/mentionable-members を扱う。
type CommentHandler struct {
	client      *ent.Client
	pushCfg     pushdelivery.Config
	frontendURL string
}

func NewCommentHandler(client *ent.Client, pushCfg pushdelivery.Config, frontendURL string) *CommentHandler {
	return &CommentHandler{client: client, pushCfg: pushCfg, frontendURL: frontendURL}
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
		// 編集済みかどうかはフロントでcreated_at!==updated_atで判定する
		// （専用フラグを新設せず既存のBaseMixinのupdated_atを流用、2026-08-27追加）。
		"updated_at": cm.UpdatedAt,
	}
	if cm.Edges.User != nil {
		row["user_name"] = cm.Edges.User.Name
		row["user_avatar_url"] = cm.Edges.User.AvatarURL
	}
	return row
}

const commentDefaultLimit = 30
const commentMaxLimit = 100
const commentAroundWindow = 15

// ListComments は GET /tasks/:task_id/comments。初回表示用（以降はSupabase Realtime購読）。
// ページネーション（2026-08-27追加、最新のものをデフォルトで表示する形に変更）：
//   - 通常時（`before`/`around`いずれも無し）：直近`limit`件（既定30）を返す。
//   - `before=<comment_id>`：そのコメントより古いものを`limit`件、チャットを上に
//     スクロールして遡る「もっと見る」用。
//   - `around=<comment_id>`：他画面（通知等）からそのコメントへ直接遷移してきた際に
//     使う。対象コメントの前後それぞれ最大commentAroundWindow件を含む窓を返す
//     （対象が最新の`limit`件の外にあっても必ず含めるため、専用の取得モードにしている）。
//
// レスポンスは常に`{items: [...(昇順=古い順)], has_more_older, has_more_newer}`。
// has_more_newerは`around`モード以外では常にfalse（通常/before取得は常に「取得済みの
// 範囲より新しい側」が既にフロント側にある前提のため）。
func (h *CommentHandler) ListComments(c *gin.Context) {
	t := middleware.CurrentTask(c)
	ctx := c.Request.Context()

	limit := parseLimit(c.Query("limit"), commentDefaultLimit, commentMaxLimit)

	if around := c.Query("around"); around != "" {
		aroundID, err := uuid.Parse(around)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid around"})
			return
		}
		target, err := h.client.Comment.Query().
			Where(comment.IDEQ(aroundID), comment.TaskIDEQ(t.ID)).
			WithUser().
			Only(ctx)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
			return
		}

		before, hasMoreOlder, err := h.fetchCommentWindow(ctx, t.ID, comment.CreatedAtLT(target.CreatedAt), comment.CreatedAtEQ(target.CreatedAt), true, target.ID, commentAroundWindow)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list comments"})
			return
		}
		after, hasMoreNewer, err := h.fetchCommentWindow(ctx, t.ID, comment.CreatedAtGT(target.CreatedAt), comment.CreatedAtEQ(target.CreatedAt), false, target.ID, commentAroundWindow)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list comments"})
			return
		}

		reactionsByComment, err := loadCommentReactionsByTask(ctx, h.client, t.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list comments"})
			return
		}
		out := make([]gin.H, 0, len(before)+1+len(after))
		for _, cm := range before {
			row := commentJSON(cm)
			row["reactions"] = orEmptyReactions(reactionsByComment[cm.ID])
			out = append(out, row)
		}
		targetRow := commentJSON(target)
		targetRow["reactions"] = orEmptyReactions(reactionsByComment[target.ID])
		out = append(out, targetRow)
		for _, cm := range after {
			row := commentJSON(cm)
			row["reactions"] = orEmptyReactions(reactionsByComment[cm.ID])
			out = append(out, row)
		}
		c.JSON(http.StatusOK, gin.H{"items": out, "has_more_older": hasMoreOlder, "has_more_newer": hasMoreNewer})
		return
	}

	// afterはaroundで開いた画面から「読み込み済みの一番新しいコメントより後」を
	// 追加取得する（realtime購読が始まる前に投稿された新着コメントを拾うための
	// 「新しい方へ読み込む」操作用）。beforeと対になる方向違いのカーソル。
	if after := c.Query("after"); after != "" {
		afterID, err := uuid.Parse(after)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after"})
			return
		}
		cursor, err := h.client.Comment.Query().
			Where(comment.IDEQ(afterID), comment.TaskIDEQ(t.ID)).
			Only(ctx)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after"})
			return
		}
		rows, hasMoreNewer, err := h.fetchCommentWindow(ctx, t.ID, comment.CreatedAtGT(cursor.CreatedAt), comment.CreatedAtEQ(cursor.CreatedAt), false, cursor.ID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list comments"})
			return
		}
		reactionsByComment, err := loadCommentReactionsByTask(ctx, h.client, t.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list comments"})
			return
		}
		out := make([]gin.H, 0, len(rows))
		for _, cm := range rows {
			row := commentJSON(cm)
			row["reactions"] = orEmptyReactions(reactionsByComment[cm.ID])
			out = append(out, row)
		}
		c.JSON(http.StatusOK, gin.H{"items": out, "has_more_older": false, "has_more_newer": hasMoreNewer})
		return
	}

	query := h.client.Comment.Query().
		Where(comment.TaskIDEQ(t.ID)).
		WithUser().
		Order(comment.ByCreatedAt(sql.OrderDesc()), comment.ByID(sql.OrderDesc()))

	if before := c.Query("before"); before != "" {
		beforeID, err := uuid.Parse(before)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before"})
			return
		}
		cursor, err := h.client.Comment.Query().
			Where(comment.IDEQ(beforeID), comment.TaskIDEQ(t.ID)).
			Only(ctx)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before"})
			return
		}
		query = query.Where(comment.Or(
			comment.CreatedAtLT(cursor.CreatedAt),
			comment.And(comment.CreatedAtEQ(cursor.CreatedAt), comment.IDLT(cursor.ID)),
		))
	}

	comments, err := query.Limit(limit + 1).All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list comments"})
		return
	}
	hasMoreOlder := len(comments) > limit
	if hasMoreOlder {
		comments = comments[:limit]
	}
	// DB取得は新しい順のため、表示用（古い順）に反転する。
	sort.SliceStable(comments, func(i, j int) bool { return comments[i].CreatedAt.Before(comments[j].CreatedAt) })

	reactionsByComment, err := loadCommentReactionsByTask(ctx, h.client, t.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list comments"})
		return
	}
	out := make([]gin.H, 0, len(comments))
	for _, cm := range comments {
		row := commentJSON(cm)
		row["reactions"] = orEmptyReactions(reactionsByComment[cm.ID])
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "has_more_older": hasMoreOlder, "has_more_newer": false})
}

// fetchCommentWindow はaroundモード用の片側（対象より古い/新しい）取得ヘルパー。
// olderSide=trueなら降順（対象に近い側から）で取得後に古い順へ反転、falseなら
// そのまま昇順で取得する。同一created_atの同時投稿を安定して順序付けるため、
// created_at比較に加えID比較も併用する。
func (h *CommentHandler) fetchCommentWindow(
	ctx context.Context,
	taskID uuid.UUID,
	primary, eqCreatedAt predicate.Comment,
	olderSide bool,
	targetID uuid.UUID,
	limit int,
) ([]*ent.Comment, bool, error) {
	var idCond predicate.Comment
	if olderSide {
		idCond = comment.IDLT(targetID)
	} else {
		idCond = comment.IDGT(targetID)
	}

	q := h.client.Comment.Query().
		Where(comment.TaskIDEQ(taskID), comment.Or(primary, comment.And(eqCreatedAt, idCond))).
		WithUser()
	if olderSide {
		q = q.Order(comment.ByCreatedAt(sql.OrderDesc()), comment.ByID(sql.OrderDesc()))
	} else {
		q = q.Order(comment.ByCreatedAt(sql.OrderAsc()), comment.ByID(sql.OrderAsc()))
	}

	rows, err := q.Limit(limit + 1).All(ctx)
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	if olderSide {
		// 降順で取ったものを古い順に反転する。
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].CreatedAt.Before(rows[j].CreatedAt) })
	}
	return rows, hasMore, nil
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
	var pendingPush []pushdelivery.Item
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
			payload := map[string]any{
				"task_id":           t.ID,
				"comment_id":        cm.ID,
				"project_id":        t.ProjectID,
				"mentioned_by":      u.ID,
				"mentioned_by_name": u.Name,
				"excerpt":           excerpt(req.Body, 100),
			}
			if _, err := tx.Notification.Create().
				SetUserID(uid).
				SetType("mentioned").
				SetPayload(payload).
				Save(ctx); err != nil {
				return err
			}
			pendingPush = append(pendingPush, pushdelivery.BuildItem(uid, h.frontendURL, t.WorkspaceID.String(), "mentioned", payload))
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

	pushdelivery.Async(h.client, h.pushCfg, pendingPush)

	c.JSON(http.StatusCreated, commentJSON(created))
}

type updateCommentRequest struct {
	Body string `json:"body" binding:"required"`
}

// UpdateComment は PATCH /tasks/:task_id/comments/:comment_id。
// 投稿者本人のみ編集可（ユーザー要望、2026-08-27追加）。編集履歴・メンションの
// 再計算は行わない（本文の置き換えのみ）。updated_atがBaseMixinの
// UpdateDefaultで自動更新されるため、フロントはcreated_at!==updated_atで
// 「編集済み」を判定できる。
func (h *CommentHandler) UpdateComment(c *gin.Context) {
	u := middleware.CurrentUser(c)
	t := middleware.CurrentTask(c)

	commentID, err := uuid.Parse(c.Param("comment_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		return
	}

	var req updateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body is required"})
		return
	}

	ctx := c.Request.Context()
	existing, err := h.client.Comment.Query().
		Where(comment.IDEQ(commentID), comment.TaskIDEQ(t.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		return
	}
	if existing.UserID != u.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	updated, err := h.client.Comment.UpdateOneID(commentID).SetBody(req.Body).Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update comment"})
		return
	}
	updated.Edges.User = u
	c.JSON(http.StatusOK, commentJSON(updated))
}

// DeleteComment は DELETE /tasks/:task_id/comments/:comment_id。投稿者本人のみ削除可。
func (h *CommentHandler) DeleteComment(c *gin.Context) {
	u := middleware.CurrentUser(c)
	t := middleware.CurrentTask(c)

	commentID, err := uuid.Parse(c.Param("comment_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		return
	}

	ctx := c.Request.Context()
	existing, err := h.client.Comment.Query().
		Where(comment.IDEQ(commentID), comment.TaskIDEQ(t.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		return
	}
	if existing.UserID != u.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		if _, err := tx.CommentMention.Delete().Where(commentmention.CommentIDEQ(commentID)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.Reaction.Delete().
			Where(reaction.TargetTypeEQ(reaction.TargetTypeComment), reaction.TargetIDEQ(commentID)).
			Exec(ctx); err != nil {
			return err
		}
		return tx.Comment.DeleteOneID(commentID).Exec(ctx)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete comment"})
		return
	}
	c.Status(http.StatusNoContent)
}

func excerpt(s string, maxLen int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxLen {
		return string(r)
	}
	return string(r[:maxLen]) + "..."
}
