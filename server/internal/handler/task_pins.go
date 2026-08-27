package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"entgo.io/ent/dialect/sql"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/task"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskpin"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

// Pin は POST /tasks/:task_id/pin。RequireTaskAccessで可視性確認済み。
// 既にピン留め済みの場合も冪等に200を返す。
func (h *TaskHandler) Pin(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	exists, err := h.client.TaskPin.Query().
		Where(taskpin.UserIDEQ(u.ID), taskpin.TaskIDEQ(t.ID)).
		Exist(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to pin task"})
		return
	}
	if !exists {
		if err := h.client.TaskPin.Create().SetUserID(u.ID).SetTaskID(t.ID).Exec(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to pin task"})
			return
		}
	}
	c.Status(http.StatusNoContent)
}

// Unpin は DELETE /tasks/:task_id/pin。本人のピンのみ削除する（無ければ何もしない）。
func (h *TaskHandler) Unpin(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	_, err := h.client.TaskPin.Delete().
		Where(taskpin.UserIDEQ(u.ID), taskpin.TaskIDEQ(t.ID)).
		Exec(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unpin task"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ListPinned は GET /workspaces/:workspace_id/pinned-tasks。ログインユーザーが
// ピン留めした、かつそのworkspaceに属するタスクをピン留め日時（task_pins.created_at）
// 降順で返す（docs/aibo/m8-implementation-plan.md スコープ追加B）。
func (h *TaskHandler) ListPinned(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	pins, err := h.client.TaskPin.Query().
		Where(taskpin.UserIDEQ(m.UserID), taskpin.HasTaskWith(task.WorkspaceIDEQ(m.WorkspaceID))).
		Order(taskpin.ByCreatedAt(sql.OrderDesc())).
		WithTask(func(q *ent.TaskQuery) { q.WithProject() }).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list pinned tasks"})
		return
	}

	out := make([]gin.H, 0, len(pins))
	for _, p := range pins {
		if p.Edges.Task == nil {
			continue
		}
		t := p.Edges.Task
		var projectName *string
		if t.Edges.Project != nil {
			projectName = &t.Edges.Project.Name
		}
		out = append(out, gin.H{
			"id":           t.ID,
			"title":        t.Title,
			"project_id":   t.ProjectID,
			"project_name": projectName,
			"status":       t.Status,
			"due_date":     formatDateTime(t.DueDate),
		})
	}
	c.JSON(http.StatusOK, out)
}
