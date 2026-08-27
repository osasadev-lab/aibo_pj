package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/task"
	"github.com/osasadev-lab/aibo_pj/server/internal/activity"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

type createSubtaskRequest struct {
	Title       string      `json:"title" binding:"required"`
	Description *string     `json:"description"`
	Priority    *string     `json:"priority" binding:"omitempty,oneof=low medium high"`
	StartDate   *string     `json:"start_date"`
	DueDate     *string     `json:"due_date"`
	AssigneeIDs []uuid.UUID `json:"assignee_ids"`
	TagIDs      []uuid.UUID `json:"tag_ids"`
}

// CreateSubtask は POST /tasks/:task_id/subtasks。親タスク画面からの
// 「＋子タスクを追加」用。孫タスク作成はエラー。project_id/section_idは親から継承する。
func (h *TaskHandler) CreateSubtask(c *gin.Context) {
	parent := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)

	if parent.ParentTaskID != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot create a subtask of a subtask"})
		return
	}

	var req createSubtaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()

	startDate, err := parseDateTimeInput(req.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_date"})
		return
	}
	dueDate, err := parseDateTimeInput(req.DueDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid due_date"})
		return
	}
	if err := validateDateRange(startDate, dueDate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	assigneeIDs, err := h.validWorkspaceUserIDs(ctx, parent.WorkspaceID, req.AssigneeIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assignee_ids must all be workspace members"})
		return
	}
	tagIDs, err := h.validTaskTagIDs(ctx, parent.WorkspaceID, parent.ProjectID, req.TagIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag_ids must be assignable to this task"})
		return
	}

	var created *ent.Task
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		builder := tx.Task.Create().
			SetWorkspaceID(parent.WorkspaceID).
			SetParentTaskID(parent.ID).
			SetTitle(req.Title).
			SetCreatedBy(u.ID)
		if req.Description != nil {
			builder = builder.SetDescription(*req.Description)
		}
		if req.Priority != nil {
			builder = builder.SetPriority(task.Priority(*req.Priority))
		}
		if startDate.provided && !startDate.clear {
			builder = builder.SetStartDate(startDate.value)
		}
		if dueDate.provided && !dueDate.clear {
			builder = builder.SetDueDate(dueDate.value)
		}
		if parent.ProjectID != nil {
			builder = builder.SetProjectID(*parent.ProjectID)
			if parent.SectionID != nil {
				builder = builder.SetSectionID(*parent.SectionID)
			}
			statusColumnID, status, err := resolveStatusColumn(ctx, tx.Client(), *parent.ProjectID, nil, nil)
			if err != nil {
				return err
			}
			if statusColumnID != nil {
				builder = builder.SetStatusColumnID(*statusColumnID)
			}
			builder = builder.SetStatus(status)
		}

		t, err := builder.Save(ctx)
		if err != nil {
			return err
		}

		if len(assigneeIDs) > 0 {
			builders := make([]*ent.TaskAssigneeCreate, 0, len(assigneeIDs))
			for _, uid := range assigneeIDs {
				builders = append(builders, tx.TaskAssignee.Create().SetTaskID(t.ID).SetUserID(uid))
			}
			if _, err := tx.TaskAssignee.CreateBulk(builders...).Save(ctx); err != nil {
				return err
			}
		}
		if len(tagIDs) > 0 {
			builders := make([]*ent.TaskTagCreate, 0, len(tagIDs))
			for _, tid := range tagIDs {
				builders = append(builders, tx.TaskTag.Create().SetTaskID(t.ID).SetTagID(tid))
			}
			if _, err := tx.TaskTag.CreateBulk(builders...).Save(ctx); err != nil {
				return err
			}
		}

		if err := activity.Record(ctx, tx, parent.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.created",
			map[string]any{"title": t.Title, "parent_task_id": parent.ID}); err != nil {
			return err
		}

		created = t
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create subtask"})
		return
	}

	c.JSON(http.StatusCreated, taskJSON(created))
}

// ListSubtasks は GET /tasks/:task_id/subtasks。
func (h *TaskHandler) ListSubtasks(c *gin.Context) {
	parent := middleware.CurrentTask(c)

	children, err := h.client.Task.Query().
		Where(task.ParentTaskIDEQ(parent.ID)).
		All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list subtasks"})
		return
	}

	out := make([]gin.H, 0, len(children))
	for _, ch := range children {
		out = append(out, taskJSON(ch))
	}
	c.JSON(http.StatusOK, out)
}
