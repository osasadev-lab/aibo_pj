package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/project"
	"github.com/osasadev-lab/aibo_pj/server/ent/projectmember"
	"github.com/osasadev-lab/aibo_pj/server/ent/task"
	entuser "github.com/osasadev-lab/aibo_pj/server/ent/user"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

// ProgressHandler は /workspaces/:workspace_id/progress（M5追加）を扱う。
type ProgressHandler struct {
	client *ent.Client
}

func NewProgressHandler(client *ent.Client) *ProgressHandler {
	return &ProgressHandler{client: client}
}

type statusCounts struct {
	NotStarted int `json:"not_started"`
	InProgress int `json:"in_progress"`
	Done       int `json:"done"`
	OnHold     int `json:"on_hold"`
}

func (s *statusCounts) add(st task.Status) {
	switch st {
	case task.StatusNotStarted:
		s.NotStarted++
	case task.StatusInProgress:
		s.InProgress++
	case task.StatusDone:
		s.Done++
	case task.StatusOnHold:
		s.OnHold++
	}
}

// GetProgress は GET /workspaces/:workspace_id/progress?project_id=。
// プロジェクト別（ステータス毎のタスク数）・担当者別（保有数・完了数）の
// 集計を返す（spec.md 4.5、棒グラフ用）。project_id指定時はその対象のみに絞り込む。
func (h *ProgressHandler) GetProgress(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	query := h.client.Task.Query().Where(
		task.WorkspaceIDEQ(m.WorkspaceID),
		task.Or(
			task.ProjectIDIsNil(),
			task.HasProjectWith(project.VisibilityEQ(project.VisibilityPublic)),
			task.HasProjectWith(project.HasMembersWith(projectmember.UserIDEQ(m.UserID))),
		),
	)

	var filterProjectID *uuid.UUID
	if v := c.Query("project_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project_id"})
			return
		}
		filterProjectID = &id
		query = query.Where(task.ProjectIDEQ(id))
	}

	tasks, err := query.WithAssignees().All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load progress"})
		return
	}

	// プロジェクト別集計。
	byProjectCounts := map[uuid.UUID]*statusCounts{}
	projectOrder := make([]uuid.UUID, 0)
	// 担当者別集計。
	type assigneeAgg struct {
		total int
		done  int
	}
	byAssignee := map[uuid.UUID]*assigneeAgg{}
	assigneeOrder := make([]uuid.UUID, 0)

	for _, t := range tasks {
		if t.ProjectID != nil {
			counts, ok := byProjectCounts[*t.ProjectID]
			if !ok {
				counts = &statusCounts{}
				byProjectCounts[*t.ProjectID] = counts
				projectOrder = append(projectOrder, *t.ProjectID)
			}
			counts.add(t.Status)
		}
		for _, a := range t.Edges.Assignees {
			agg, ok := byAssignee[a.UserID]
			if !ok {
				agg = &assigneeAgg{}
				byAssignee[a.UserID] = agg
				assigneeOrder = append(assigneeOrder, a.UserID)
			}
			agg.total++
			if t.Status == task.StatusDone {
				agg.done++
			}
		}
	}

	projectNames := map[uuid.UUID]string{}
	if len(projectOrder) > 0 {
		projects, err := h.client.Project.Query().Where(project.IDIn(projectOrder...)).All(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load progress"})
			return
		}
		for _, p := range projects {
			projectNames[p.ID] = p.Name
		}
	}

	userNames := map[uuid.UUID]string{}
	if len(assigneeOrder) > 0 {
		users, err := h.client.User.Query().Where(entuser.IDIn(assigneeOrder...)).All(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load progress"})
			return
		}
		for _, u := range users {
			userNames[u.ID] = u.Name
		}
	}

	byProject := make([]gin.H, 0, len(projectOrder))
	for _, id := range projectOrder {
		if filterProjectID != nil && id != *filterProjectID {
			continue
		}
		byProject = append(byProject, gin.H{
			"project_id":   id,
			"project_name": projectNames[id],
			"counts":       byProjectCounts[id],
		})
	}

	byAssigneeOut := make([]gin.H, 0, len(assigneeOrder))
	for _, id := range assigneeOrder {
		agg := byAssignee[id]
		byAssigneeOut = append(byAssigneeOut, gin.H{
			"user_id": id,
			"name":    userNames[id],
			"total":   agg.total,
			"done":    agg.done,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"by_project":  byProject,
		"by_assignee": byAssigneeOut,
	})
}
