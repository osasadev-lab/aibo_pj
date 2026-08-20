package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/calendarwatchedmember"
	"github.com/osasadev-lab/aibo_pj/server/ent/project"
	"github.com/osasadev-lab/aibo_pj/server/ent/projectmember"
	"github.com/osasadev-lab/aibo_pj/server/ent/task"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskassignee"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

const monthLayout = "2006-01"

// CalendarHandler は /workspaces/:workspace_id/calendar・calendar-watched-users
// （M5追加）を扱う。
type CalendarHandler struct {
	client *ent.Client
}

func NewCalendarHandler(client *ent.Client) *CalendarHandler {
	return &CalendarHandler{client: client}
}

// GetCalendar は GET /workspaces/:workspace_id/calendar?month=YYYY-MM（省略時は当月）。
// 閲覧対象は自分＋自分が追加したcalendar_watched_membersの担当タスク（期限のある
// もののみ、spec.md 5章の「対象：期限が設定されているタスクのみ」と同じ考え方）。
func (h *CalendarHandler) GetCalendar(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	monthStart := time.Now()
	if v := c.Query("month"); v != "" {
		t, err := time.Parse(monthLayout, v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid month"})
			return
		}
		monthStart = t
	}
	rangeStart := time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, time.UTC)
	rangeEnd := rangeStart.AddDate(0, 1, 0)

	watchedIDs, err := h.client.CalendarWatchedMember.Query().
		Where(
			calendarwatchedmember.WorkspaceIDEQ(m.WorkspaceID),
			calendarwatchedmember.UserIDEQ(m.UserID),
		).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load calendar"})
		return
	}
	targetUserIDs := make([]uuid.UUID, 0, len(watchedIDs)+1)
	targetUserIDs = append(targetUserIDs, m.UserID)
	for _, w := range watchedIDs {
		targetUserIDs = append(targetUserIDs, w.WatchedUserID)
	}

	tasks, err := h.client.Task.Query().
		Where(
			task.WorkspaceIDEQ(m.WorkspaceID),
			task.DueDateNotNil(),
			task.DueDateGTE(rangeStart),
			task.DueDateLT(rangeEnd),
			task.HasAssigneesWith(taskassignee.UserIDIn(targetUserIDs...)),
			task.Or(
				task.ProjectIDIsNil(),
				task.HasProjectWith(project.VisibilityEQ(project.VisibilityPublic)),
				task.HasProjectWith(project.HasMembersWith(projectmember.UserIDEQ(m.UserID))),
			),
		).
		WithAssignees().
		// タグ絞り込みUI（M5追加要望）のため、タグ情報もあわせて返す。
		WithTags(func(q *ent.TaskTagQuery) { q.WithTag() }).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load calendar"})
		return
	}

	out := make([]gin.H, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, taskJSON(t))
	}
	c.JSON(http.StatusOK, out)
}

// GetWatchedMembers は GET /workspaces/:workspace_id/calendar-watched-users。
// 本人がカレンダーに追加表示しているメンバー一覧を返す。
func (h *CalendarHandler) GetWatchedMembers(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	rows, err := h.client.CalendarWatchedMember.Query().
		Where(
			calendarwatchedmember.WorkspaceIDEQ(m.WorkspaceID),
			calendarwatchedmember.UserIDEQ(m.UserID),
		).
		WithWatchedUser().
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load watched members"})
		return
	}

	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		if r.Edges.WatchedUser == nil {
			continue
		}
		out = append(out, gin.H{
			"user_id": r.Edges.WatchedUser.ID,
			"name":    r.Edges.WatchedUser.Name,
			"email":   r.Edges.WatchedUser.Email,
		})
	}
	c.JSON(http.StatusOK, out)
}

type putWatchedMembersRequest struct {
	UserIDs []uuid.UUID `json:"user_ids"`
}

// PutWatchedMembers は PUT /workspaces/:workspace_id/calendar-watched-users。
// 指定したメンバー集合に全置換する（PutTags/PutMembersと同じ全置換パターン）。
// 自分自身のIDは無視する（自分のタスクは常に表示対象のため）。
func (h *CalendarHandler) PutWatchedMembers(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	var req putWatchedMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ids := dedupUUIDs(req.UserIDs)
	filtered := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id != m.UserID {
			filtered = append(filtered, id)
		}
	}

	if len(filtered) > 0 {
		valid, err := h.validWorkspaceUserIDs(ctx, m.WorkspaceID, filtered)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate members"})
			return
		}
		if len(valid) != len(filtered) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "one or more user_ids are not workspace members"})
			return
		}
	}

	err := withTx(ctx, h.client, func(tx *ent.Tx) error {
		if _, err := tx.CalendarWatchedMember.Delete().
			Where(
				calendarwatchedmember.WorkspaceIDEQ(m.WorkspaceID),
				calendarwatchedmember.UserIDEQ(m.UserID),
			).
			Exec(ctx); err != nil {
			return err
		}
		for _, watchedID := range filtered {
			if _, err := tx.CalendarWatchedMember.Create().
				SetWorkspaceID(m.WorkspaceID).
				SetUserID(m.UserID).
				SetWatchedUserID(watchedID).
				Save(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update watched members"})
		return
	}
	c.Status(http.StatusNoContent)
}

// validWorkspaceUserIDs は指定したIDが全てそのworkspaceのメンバーであることを
// 検証し、実際にメンバーであったIDのみを返す（TaskHandler.validWorkspaceUserIDsと
// 同型のチェック）。
func (h *CalendarHandler) validWorkspaceUserIDs(ctx context.Context, workspaceID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	rows, err := h.client.WorkspaceMember.Query().
		Where(workspacemember.WorkspaceIDEQ(workspaceID), workspacemember.UserIDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.UserID)
	}
	return out, nil
}
