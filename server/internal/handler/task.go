package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/osasadev-lab/aibo_pj/server/ent"
	"github.com/osasadev-lab/aibo_pj/server/ent/activitylog"
	"github.com/osasadev-lab/aibo_pj/server/ent/comment"
	"github.com/osasadev-lab/aibo_pj/server/ent/commentmention"
	"github.com/osasadev-lab/aibo_pj/server/ent/project"
	"github.com/osasadev-lab/aibo_pj/server/ent/projectmember"
	"github.com/osasadev-lab/aibo_pj/server/ent/projectstatuscolumn"
	"github.com/osasadev-lab/aibo_pj/server/ent/section"
	"github.com/osasadev-lab/aibo_pj/server/ent/tag"
	"github.com/osasadev-lab/aibo_pj/server/ent/task"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskassignee"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskdependency"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskmention"
	"github.com/osasadev-lab/aibo_pj/server/ent/tasktag"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	"github.com/osasadev-lab/aibo_pj/server/internal/activity"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

const dateLayout = "2006-01-02"

var errInvalidIDs = errors.New("one or more ids are invalid")

// TaskHandler は /tasks, /workspaces/:workspace_id/tasks 配下を扱う。
type TaskHandler struct {
	client *ent.Client
}

func NewTaskHandler(client *ent.Client) *TaskHandler {
	return &TaskHandler{client: client}
}

func taskJSON(t *ent.Task) gin.H {
	row := gin.H{
		"id":               t.ID,
		"workspace_id":     t.WorkspaceID,
		"project_id":       t.ProjectID,
		"section_id":       t.SectionID,
		"status":           t.Status,
		"status_column_id": t.StatusColumnID,
		"parent_task_id":   t.ParentTaskID,
		"title":            t.Title,
		"description":      t.Description,
		"priority":         t.Priority,
		"start_date":       formatDate(t.StartDate),
		"due_date":         formatDate(t.DueDate),
		"created_by":       t.CreatedBy,
	}
	// assigneesがeager-load済み（WithAssignees()）の場合のみassignee_idsを含める。
	if t.Edges.Assignees != nil {
		ids := make([]uuid.UUID, 0, len(t.Edges.Assignees))
		for _, a := range t.Edges.Assignees {
			ids = append(ids, a.UserID)
		}
		row["assignee_ids"] = ids
	}
	// mentionsがeager-load済み（WithMentions()）の場合のみmentioned_user_idsを含める。
	if t.Edges.Mentions != nil {
		ids := make([]uuid.UUID, 0, len(t.Edges.Mentions))
		for _, tm := range t.Edges.Mentions {
			ids = append(ids, tm.MentionedUserID)
		}
		row["mentioned_user_ids"] = ids
	}
	return row
}

func formatDate(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(dateLayout)
	return &s
}

// Search は GET /workspaces/:workspace_id/tasks。
// 可視性フィルタ（単体タスクはworkspace全員、publicプロジェクトは全員、
// privateプロジェクトは参画メンバーのみ）を必ず適用する。
func (h *TaskHandler) Search(c *gin.Context) {
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

	if v := c.Query("project_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project_id"})
			return
		}
		query = query.Where(task.ProjectIDEQ(id))
	}
	if v := c.Query("assignee_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid assignee_id"})
			return
		}
		query = query.Where(task.HasAssigneesWith(taskassignee.UserIDEQ(id)))
	}
	if v := c.Query("status"); v != "" {
		query = query.Where(task.StatusEQ(task.Status(v)))
	}
	if v := c.Query("status_column_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status_column_id"})
			return
		}
		query = query.Where(task.StatusColumnIDEQ(id))
	}
	if v := c.Query("due_before"); v != "" {
		d, err := time.Parse(dateLayout, v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid due_before"})
			return
		}
		query = query.Where(task.DueDateLT(d))
	}

	tasks, err := query.WithAssignees().All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search tasks"})
		return
	}

	out := make([]gin.H, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, taskJSON(t))
	}
	c.JSON(http.StatusOK, out)
}

// MyTasks は GET /workspaces/:workspace_id/my-tasks。
// 呼び出しユーザーがassigneeになっているタスクをプロジェクト横断で共通statusで
// 集約する。可視性フィルタも防御的に適用する（担当者なら通常閲覧可能なはずだが、
// Searchと同じ絞り込みを維持する）。
// ?dateで基準日を指定すると、その日以前が期限のタスクだけに絞り込む
// （未指定時は今まで通り全件表示、due_todayは基準日=今日で判定）。
func (h *TaskHandler) MyTasks(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	query := h.client.Task.Query().Where(
		task.WorkspaceIDEQ(m.WorkspaceID),
		task.HasAssigneesWith(taskassignee.UserIDEQ(m.UserID)),
		task.Or(
			task.ProjectIDIsNil(),
			task.HasProjectWith(project.VisibilityEQ(project.VisibilityPublic)),
			task.HasProjectWith(project.HasMembersWith(projectmember.UserIDEQ(m.UserID))),
		),
	)
	if v := c.Query("status"); v != "" {
		query = query.Where(task.StatusEQ(task.Status(v)))
	}

	referenceDate := time.Now()
	if v := c.Query("date"); v != "" {
		d, err := time.Parse(dateLayout, v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date"})
			return
		}
		referenceDate = d
		query = query.Where(task.DueDateLTE(d))
	}

	tasks, err := query.WithAssignees().All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list my-tasks"})
		return
	}

	refDateStr := referenceDate.Format(dateLayout)
	out := make([]gin.H, 0, len(tasks))
	for _, t := range tasks {
		row := taskJSON(t)
		row["due_today"] = t.DueDate != nil && t.DueDate.Format(dateLayout) == refDateStr
		out = append(out, row)
	}
	c.JSON(http.StatusOK, out)
}

type createTaskRequest struct {
	Title          string      `json:"title" binding:"required"`
	Description    *string     `json:"description"`
	ProjectID      *uuid.UUID  `json:"project_id"`
	SectionID      *uuid.UUID  `json:"section_id"`
	Status         *string     `json:"status" binding:"omitempty,oneof=not_started in_progress done on_hold"`
	StatusColumnID *uuid.UUID  `json:"status_column_id"`
	Priority       *string     `json:"priority" binding:"omitempty,oneof=low medium high"`
	StartDate      *string     `json:"start_date"`
	DueDate        *string     `json:"due_date"`
	AssigneeIDs    []uuid.UUID `json:"assignee_ids"`
	TagIDs         []uuid.UUID `json:"tag_ids"`
}

// Create は POST /workspaces/:workspace_id/tasks。project_id未指定なら単体タスク。
func (h *TaskHandler) Create(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	u := middleware.CurrentUser(c)

	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()

	var proj *ent.Project
	if req.ProjectID != nil {
		p, err := h.client.Project.Get(ctx, *req.ProjectID)
		if err != nil || p.WorkspaceID != m.WorkspaceID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "project not found in this workspace"})
			return
		}
		if err := middleware.CheckProjectVisibility(ctx, h.client, p, u.ID); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "not a project member"})
			return
		}
		proj = p

		if req.SectionID != nil {
			ok, err := h.client.Section.Query().
				Where(section.IDEQ(*req.SectionID), section.ProjectIDEQ(proj.ID)).
				Exist(ctx)
			if err != nil || !ok {
				c.JSON(http.StatusBadRequest, gin.H{"error": "section not found in this project"})
				return
			}
		}
	} else if req.SectionID != nil || req.StatusColumnID != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "section_id/status_column_id require project_id"})
		return
	}

	startDate, err := parseOptionalDate(req.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_date"})
		return
	}
	dueDate, err := parseOptionalDate(req.DueDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid due_date"})
		return
	}

	assigneeIDs, err := h.validWorkspaceUserIDs(ctx, m.WorkspaceID, req.AssigneeIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assignee_ids must all be workspace members"})
		return
	}
	tagIDs, err := h.validWorkspaceTagIDs(ctx, m.WorkspaceID, req.TagIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag_ids must all belong to this workspace"})
		return
	}

	var created *ent.Task
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		builder := tx.Task.Create().
			SetWorkspaceID(m.WorkspaceID).
			SetTitle(req.Title).
			SetCreatedBy(u.ID)
		if req.Description != nil {
			builder = builder.SetDescription(*req.Description)
		}
		if req.Priority != nil {
			builder = builder.SetPriority(task.Priority(*req.Priority))
		}
		if startDate != nil {
			builder = builder.SetStartDate(*startDate)
		}
		if dueDate != nil {
			builder = builder.SetDueDate(*dueDate)
		}
		if req.SectionID != nil {
			builder = builder.SetSectionID(*req.SectionID)
		}

		if proj != nil {
			builder = builder.SetProjectID(proj.ID)
			statusColumnID, status, err := resolveStatusColumn(ctx, tx.Client(), proj.ID, req.StatusColumnID, req.Status)
			if err != nil {
				return err
			}
			if statusColumnID != nil {
				builder = builder.SetStatusColumnID(*statusColumnID)
			}
			builder = builder.SetStatus(status)
		} else if req.Status != nil {
			builder = builder.SetStatus(task.Status(*req.Status))
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

		if err := activity.Record(ctx, tx, m.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.created",
			map[string]any{"title": t.Title}); err != nil {
			return err
		}

		created = t
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	c.JSON(http.StatusCreated, taskJSON(created))
}

// Get は GET /tasks/:task_id。RequireTaskAccessで可視性確認済み。
func (h *TaskHandler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, taskJSON(middleware.CurrentTask(c)))
}

type updateTaskRequest struct {
	Title          *string    `json:"title"`
	Description    *string    `json:"description"`
	Status         *string    `json:"status" binding:"omitempty,oneof=not_started in_progress done on_hold"`
	StatusColumnID *uuid.UUID `json:"status_column_id"`
	Priority       *string    `json:"priority" binding:"omitempty,oneof=low medium high"`
	StartDate      *string    `json:"start_date"`
	DueDate        *string    `json:"due_date"`
	// ポインタにして「フィールド省略＝メンション不変」と「空配列＝全メンション解除」を
	// 区別する（ステータス変更等の部分PATCHが誤ってメンションを消さないようにするため）。
	MentionedUserIDs *[]uuid.UUID `json:"mentioned_user_ids"`
}

// Update は PATCH /tasks/:task_id。
// カンバンD&Dはstatus_column_idを、マイタスクD&Dはstatusを送る想定で、
// サーバー側でもう一方を自動同期する（db-schema.md）。両方指定時はstatus_column_id優先。
func (h *TaskHandler) Update(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)

	var req updateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	startDate, err := parseOptionalDate(req.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_date"})
		return
	}
	dueDate, err := parseOptionalDate(req.DueDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid due_date"})
		return
	}

	ctx := c.Request.Context()

	var mentionIDs []uuid.UUID
	if req.MentionedUserIDs != nil {
		mentionIDs = dedupUUIDs(*req.MentionedUserIDs)
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
	}

	statusChanging := req.Status != nil || req.StatusColumnID != nil
	previousStatus := t.Status

	var updated *ent.Task
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		builder := tx.Task.UpdateOneID(t.ID)
		changes := map[string]any{}

		if req.Title != nil {
			builder = builder.SetTitle(*req.Title)
			changes["title"] = *req.Title
		}
		if req.Description != nil {
			builder = builder.SetDescription(*req.Description)
			changes["description"] = *req.Description
		}
		if req.Priority != nil {
			builder = builder.SetPriority(task.Priority(*req.Priority))
			changes["priority"] = *req.Priority
		}
		if startDate != nil {
			builder = builder.SetStartDate(*startDate)
			changes["start_date"] = *req.StartDate
		}
		if dueDate != nil {
			builder = builder.SetDueDate(*dueDate)
			changes["due_date"] = *req.DueDate
		}

		switch {
		case req.StatusColumnID != nil:
			col, err := tx.ProjectStatusColumn.Get(ctx, *req.StatusColumnID)
			if err != nil || t.ProjectID == nil || col.ProjectID != *t.ProjectID {
				return errInvalidIDs
			}
			builder = builder.SetStatusColumnID(col.ID).SetStatus(task.Status(col.MapsToStatus))
		case req.Status != nil:
			if t.ProjectID != nil {
				col, err := tx.ProjectStatusColumn.Query().
					Where(
						projectstatuscolumn.ProjectIDEQ(*t.ProjectID),
						projectstatuscolumn.MapsToStatusEQ(projectstatuscolumn.MapsToStatus(*req.Status)),
					).
					Order(projectstatuscolumn.ByPosition()).
					First(ctx)
				if err == nil {
					builder = builder.SetStatusColumnID(col.ID)
				} else {
					builder = builder.ClearStatusColumnID()
				}
			}
			builder = builder.SetStatus(task.Status(*req.Status))
		}

		var err error
		updated, err = builder.Save(ctx)
		if err != nil {
			return err
		}

		if req.MentionedUserIDs != nil {
			existing, err := tx.TaskMention.Query().Where(taskmention.TaskIDEQ(t.ID)).All(ctx)
			if err != nil {
				return err
			}
			oldSet := make(map[uuid.UUID]struct{}, len(existing))
			for _, tm := range existing {
				oldSet[tm.MentionedUserID] = struct{}{}
			}

			if _, err := tx.TaskMention.Delete().Where(taskmention.TaskIDEQ(t.ID)).Exec(ctx); err != nil {
				return err
			}
			for _, id := range mentionIDs {
				if _, err := tx.TaskMention.Create().SetTaskID(t.ID).SetMentionedUserID(id).Save(ctx); err != nil {
					return err
				}
			}

			description := t.Description
			if req.Description != nil {
				description = req.Description
			}
			var descText string
			if description != nil {
				descText = *description
			}
			for _, id := range mentionIDs {
				if _, wasMentioned := oldSet[id]; wasMentioned {
					continue
				}
				if _, err := tx.Notification.Create().
					SetUserID(id).
					SetType("mentioned").
					SetPayload(map[string]any{
						"task_id":           t.ID,
						"project_id":        t.ProjectID,
						"mentioned_by":      u.ID,
						"mentioned_by_name": u.Name,
						"excerpt":           excerpt(descText, 100),
					}).
					Save(ctx); err != nil {
					return err
				}
			}
		}

		if statusChanging {
			if err := activity.Record(ctx, tx, t.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.status_changed",
				map[string]any{"from": string(previousStatus), "to": string(updated.Status)}); err != nil {
				return err
			}
		}
		if len(changes) > 0 {
			if err := activity.Record(ctx, tx, t.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.updated", changes); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errInvalidIDs) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": "failed to update task"})
		return
	}
	c.JSON(http.StatusOK, taskJSON(updated))
}

// Delete は DELETE /tasks/:task_id。子タスクも道連れに削除する。
// entのschemaにカスケード削除指定が無いため、関連テーブルを手動で削除する。
// comments/activity_logsもtask_idを参照しているため、他のedgeテーブルと同様に
// 先に削除しておかないとタスク本体の削除時に外部キー制約違反になる。
func (h *TaskHandler) Delete(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	err := withTx(ctx, h.client, func(tx *ent.Tx) error {
		childIDs, err := tx.Task.Query().Where(task.ParentTaskIDEQ(t.ID)).IDs(ctx)
		if err != nil {
			return err
		}
		ids := append(childIDs, t.ID)

		if _, err := tx.TaskDependency.Delete().
			Where(taskdependency.Or(
				taskdependency.TaskIDIn(ids...),
				taskdependency.DependsOnTaskIDIn(ids...),
			)).
			Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.TaskTag.Delete().Where(tasktag.TaskIDIn(ids...)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.TaskAssignee.Delete().Where(taskassignee.TaskIDIn(ids...)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.TaskMention.Delete().Where(taskmention.TaskIDIn(ids...)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.CommentMention.Delete().
			Where(commentmention.HasCommentWith(comment.TaskIDIn(ids...))).
			Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.Comment.Delete().Where(comment.TaskIDIn(ids...)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.ActivityLog.Delete().Where(activitylog.TaskIDIn(ids...)).Exec(ctx); err != nil {
			return err
		}
		// task.deleted自体は削除対象のtask_idを参照できない（削除後に外部キー違反になる）ため、
		// task_idを付けずproject_idのみで記録する。
		if err := activity.Record(ctx, tx, t.WorkspaceID, nil, t.ProjectID, u.ID, "task.deleted",
			map[string]any{"title": t.Title, "task_id": t.ID, "child_ids": childIDs}); err != nil {
			return err
		}
		_, err = tx.Task.Delete().Where(task.IDIn(ids...)).Exec(ctx)
		return err
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete task"})
		return
	}
	c.Status(http.StatusNoContent)
}

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

	startDate, err := parseOptionalDate(req.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_date"})
		return
	}
	dueDate, err := parseOptionalDate(req.DueDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid due_date"})
		return
	}

	assigneeIDs, err := h.validWorkspaceUserIDs(ctx, parent.WorkspaceID, req.AssigneeIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assignee_ids must all be workspace members"})
		return
	}
	tagIDs, err := h.validWorkspaceTagIDs(ctx, parent.WorkspaceID, req.TagIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag_ids must all belong to this workspace"})
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
		if startDate != nil {
			builder = builder.SetStartDate(*startDate)
		}
		if dueDate != nil {
			builder = builder.SetDueDate(*dueDate)
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

type putAssigneesRequest struct {
	UserIDs []uuid.UUID `json:"user_ids" binding:"required"`
}

// PutAssignees は PUT /tasks/:task_id/assignees。担当者を入れ替える。
func (h *TaskHandler) PutAssignees(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)

	var req putAssigneesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids is required"})
		return
	}

	ctx := c.Request.Context()
	ids, err := h.validWorkspaceUserIDs(ctx, t.WorkspaceID, req.UserIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids must all be workspace members"})
		return
	}

	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		existing, err := tx.TaskAssignee.Query().Where(taskassignee.TaskIDEQ(t.ID)).All(ctx)
		if err != nil {
			return err
		}
		oldSet := make(map[uuid.UUID]struct{}, len(existing))
		for _, a := range existing {
			oldSet[a.UserID] = struct{}{}
		}
		newSet := make(map[uuid.UUID]struct{}, len(ids))
		for _, id := range ids {
			newSet[id] = struct{}{}
		}

		if _, err := tx.TaskAssignee.Delete().Where(taskassignee.TaskIDEQ(t.ID)).Exec(ctx); err != nil {
			return err
		}
		if len(ids) > 0 {
			builders := make([]*ent.TaskAssigneeCreate, 0, len(ids))
			for _, uid := range ids {
				builders = append(builders, tx.TaskAssignee.Create().SetTaskID(t.ID).SetUserID(uid))
			}
			if _, err := tx.TaskAssignee.CreateBulk(builders...).Save(ctx); err != nil {
				return err
			}
		}

		for _, id := range ids {
			if _, was := oldSet[id]; was {
				continue
			}
			if _, err := tx.Notification.Create().
				SetUserID(id).
				SetType("assigned").
				SetPayload(map[string]any{
					"task_id":         t.ID,
					"project_id":      t.ProjectID,
					"changed_by":      u.ID,
					"changed_by_name": u.Name,
					"task_title":      t.Title,
				}).
				Save(ctx); err != nil {
				return err
			}
		}
		for id := range oldSet {
			if _, still := newSet[id]; still {
				continue
			}
			if _, err := tx.Notification.Create().
				SetUserID(id).
				SetType("unassigned").
				SetPayload(map[string]any{
					"task_id":         t.ID,
					"project_id":      t.ProjectID,
					"changed_by":      u.ID,
					"changed_by_name": u.Name,
					"task_title":      t.Title,
				}).
				Save(ctx); err != nil {
				return err
			}
		}

		return activity.Record(ctx, tx, t.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.assigned",
			map[string]any{"assignee_ids": ids})
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update assignees"})
		return
	}
	c.Status(http.StatusNoContent)
}

type putTagsRequest struct {
	TagIDs []uuid.UUID `json:"tag_ids" binding:"required"`
}

// PutTags は PUT /tasks/:task_id/tags。タグを入れ替える。
func (h *TaskHandler) PutTags(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)

	var req putTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag_ids is required"})
		return
	}

	ctx := c.Request.Context()
	ids, err := h.validWorkspaceTagIDs(ctx, t.WorkspaceID, req.TagIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag_ids must all belong to this workspace"})
		return
	}

	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		if _, err := tx.TaskTag.Delete().Where(tasktag.TaskIDEQ(t.ID)).Exec(ctx); err != nil {
			return err
		}
		if len(ids) > 0 {
			builders := make([]*ent.TaskTagCreate, 0, len(ids))
			for _, tid := range ids {
				builders = append(builders, tx.TaskTag.Create().SetTaskID(t.ID).SetTagID(tid))
			}
			if _, err := tx.TaskTag.CreateBulk(builders...).Save(ctx); err != nil {
				return err
			}
		}
		return activity.Record(ctx, tx, t.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.tagged",
			map[string]any{"tag_ids": ids})
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tags"})
		return
	}
	c.Status(http.StatusNoContent)
}

// --- helpers ---

func parseOptionalDate(s *string) (*time.Time, error) {
	if s == nil {
		return nil, nil
	}
	d, err := time.Parse(dateLayout, *s)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (h *TaskHandler) validWorkspaceUserIDs(ctx context.Context, workspaceID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	deduped := dedupUUIDs(ids)
	count, err := h.client.WorkspaceMember.Query().
		Where(workspacemember.WorkspaceIDEQ(workspaceID), workspacemember.UserIDIn(deduped...)).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	if count != len(deduped) {
		return nil, errInvalidIDs
	}
	return deduped, nil
}

func (h *TaskHandler) validWorkspaceTagIDs(ctx context.Context, workspaceID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	deduped := dedupUUIDs(ids)
	count, err := h.client.Tag.Query().
		Where(tag.WorkspaceIDEQ(workspaceID), tag.IDIn(deduped...)).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	if count != len(deduped) {
		return nil, errInvalidIDs
	}
	return deduped, nil
}

func dedupUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]struct{}{}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// resolveStatusColumn は指定されたstatus_column_id/statusから
// (status_column_id, status)を決定する。両方nilなら、そのプロジェクトの
// not_started列（position最小）をデフォルトにする。
func resolveStatusColumn(ctx context.Context, client *ent.Client, projectID uuid.UUID, statusColumnID *uuid.UUID, status *string) (*uuid.UUID, task.Status, error) {
	if statusColumnID != nil {
		col, err := client.ProjectStatusColumn.Get(ctx, *statusColumnID)
		if err != nil || col.ProjectID != projectID {
			return nil, "", errInvalidIDs
		}
		id := col.ID
		return &id, task.Status(col.MapsToStatus), nil
	}

	var mapsTo projectstatuscolumn.MapsToStatus
	if status != nil {
		mapsTo = projectstatuscolumn.MapsToStatus(*status)
	} else {
		mapsTo = projectstatuscolumn.MapsToStatusNotStarted
	}

	col, err := client.ProjectStatusColumn.Query().
		Where(
			projectstatuscolumn.ProjectIDEQ(projectID),
			projectstatuscolumn.MapsToStatusEQ(mapsTo),
		).
		Order(projectstatuscolumn.ByPosition()).
		First(ctx)
	if err != nil {
		return nil, task.Status(mapsTo), nil
	}
	id := col.ID
	return &id, task.Status(mapsTo), nil
}
