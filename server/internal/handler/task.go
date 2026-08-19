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
	"github.com/osasadev-lab/aibo_pj/server/ent/attachment"
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
	"github.com/osasadev-lab/aibo_pj/server/internal/storage"
)

const dateLayout = "2006-01-02"

var errInvalidIDs = errors.New("one or more ids are invalid")

// TaskHandler は /tasks, /workspaces/:workspace_id/tasks 配下を扱う。
type TaskHandler struct {
	client *ent.Client
	r2     *storage.R2Client
}

func NewTaskHandler(client *ent.Client, r2 *storage.R2Client) *TaskHandler {
	return &TaskHandler{client: client, r2: r2}
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
	// tagsがeager-load済み（WithTags(func(q){ q.WithTag() })）の場合のみtagsを含める。
	if t.Edges.Tags != nil {
		tags := make([]gin.H, 0, len(t.Edges.Tags))
		for _, tt := range t.Edges.Tags {
			if tt.Edges.Tag == nil {
				continue
			}
			tags = append(tags, tagJSON(tt.Edges.Tag))
		}
		row["tags"] = tags
	}
	// dependenciesがeager-load済み（WithDependencies(func(q){ q.WithDependsOn() })）の
	// 場合のみhas_incomplete_dependenciesを含める（先行タスクにstatus!=doneが1件でも
	// あればtrue。カンバンカード・タスク詳細のバッジ表示用、spec.md 4.6）。
	if t.Edges.Dependencies != nil {
		incomplete := false
		ids := make([]uuid.UUID, 0, len(t.Edges.Dependencies))
		for _, d := range t.Edges.Dependencies {
			ids = append(ids, d.DependsOnTaskID)
			if d.Edges.DependsOn != nil && d.Edges.DependsOn.Status != task.StatusDone {
				incomplete = true
			}
		}
		row["has_incomplete_dependencies"] = incomplete
		// depends_on_task_idsはプロジェクトカンバンのホバー強調（個人設定、
		// docs/aibo/m4-implementation-plan.md 4章）がAPIを追加で叩かずクライアント側
		// だけで先行/後続関係を判定できるようにするための付随情報。「後続」側は
		// このtask_idを他タスクのdepends_on_task_idsから逆引きすればよいため、
		// パフォーマンス上の理由でDependentsは別途eager-loadしない（往復が1回減る）。
		row["depends_on_task_ids"] = ids
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

	tasks, err := query.
		WithAssignees().
		WithDependencies(func(q *ent.TaskDependencyQuery) { q.WithDependsOn() }).
		// Tagsはプロジェクトカンバンのタグバッジ・ホバー強調（tagモード）用。
		// MyTasksでは対象外（ユーザー確認済み）のためSearchのみ追加する。Dependentsは
		// あえてeager-loadしない（他タスクのdepends_on_task_idsから逆引きできるため、
		// 往復を1回減らすパフォーマンス上の判断）。
		WithTags(func(q *ent.TaskTagQuery) { q.WithTag() }).
		All(ctx)
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

	tasks, err := query.
		WithAssignees().
		WithDependencies(func(q *ent.TaskDependencyQuery) { q.WithDependsOn() }).
		All(ctx)
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
	var projID *uuid.UUID
	if proj != nil {
		projID = &proj.ID
	}
	tagIDs, err := h.validTaskTagIDs(ctx, m.WorkspaceID, projID, req.TagIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag_ids must be assignable to this task"})
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
// Get は GET /tasks/:task_id。assignee_ids/mentioned_user_ids/tagsを含めるための
// eager-loadはここでだけ行う（RequireTaskAccessは全タスク系エンドポイント共通のため
// 意図的に軽量化してある）。
func (h *TaskHandler) Get(c *gin.Context) {
	t := middleware.CurrentTask(c)
	full, err := h.client.Task.Query().
		Where(task.IDEQ(t.ID)).
		WithAssignees().
		WithMentions().
		WithTags(func(q *ent.TaskTagQuery) { q.WithTag() }).
		Only(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load task"})
		return
	}
	c.JSON(http.StatusOK, taskJSON(full))
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

	var attachmentKeys []string
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
		// R2オブジェクトはDBコミットが確定してから削除する（トランザクション内で
		// 外部API呼び出しをしない）。ここではキーだけ収集しておく。
		attachments, err := tx.Attachment.Query().Where(attachment.TaskIDIn(ids...)).All(ctx)
		if err != nil {
			return err
		}
		for _, a := range attachments {
			attachmentKeys = append(attachmentKeys, a.StorageKey)
		}
		if _, err := tx.Attachment.Delete().Where(attachment.TaskIDIn(ids...)).Exec(ctx); err != nil {
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
	deleteR2Objects(ctx, h.r2, attachmentKeys)
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

func dependencyTaskJSON(t *ent.Task) gin.H {
	return gin.H{"id": t.ID, "title": t.Title, "status": t.Status, "project_id": t.ProjectID}
}

// ListDependencies は GET /tasks/:task_id/dependencies。
// predecessors=このタスクが依存している先行タスク、successors=このタスクに
// 依存している後続タスク。
func (h *TaskHandler) ListDependencies(c *gin.Context) {
	t := middleware.CurrentTask(c)
	ctx := c.Request.Context()

	preds, err := h.client.TaskDependency.Query().
		Where(taskdependency.TaskIDEQ(t.ID)).
		WithDependsOn().
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list dependencies"})
		return
	}
	succs, err := h.client.TaskDependency.Query().
		Where(taskdependency.DependsOnTaskIDEQ(t.ID)).
		WithTask().
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list dependencies"})
		return
	}

	predecessors := make([]gin.H, 0, len(preds))
	for _, d := range preds {
		if d.Edges.DependsOn == nil {
			continue
		}
		predecessors = append(predecessors, gin.H{"id": d.ID, "task": dependencyTaskJSON(d.Edges.DependsOn)})
	}
	successors := make([]gin.H, 0, len(succs))
	for _, d := range succs {
		if d.Edges.Task == nil {
			continue
		}
		successors = append(successors, gin.H{"id": d.ID, "task": dependencyTaskJSON(d.Edges.Task)})
	}
	c.JSON(http.StatusOK, gin.H{"predecessors": predecessors, "successors": successors})
}

// wouldCreateCycle は、taskIDがdependsOnTaskIDに依存する辺を追加した場合に
// 循環依存が生じるかを判定する。dependsOnTaskIDを起点に既存のdepends_on辺を
// 辿って（＝depends_on_task_idがさらに依存している先行タスクを辿って）taskIDに
// 到達できれば、新しい辺を足すと閉路になる。
func (h *TaskHandler) wouldCreateCycle(ctx context.Context, taskID, dependsOnTaskID uuid.UUID) (bool, error) {
	visited := map[uuid.UUID]struct{}{}
	queue := []uuid.UUID{dependsOnTaskID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == taskID {
			return true, nil
		}
		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}

		preds, err := h.client.TaskDependency.Query().Where(taskdependency.TaskIDEQ(current)).All(ctx)
		if err != nil {
			return false, err
		}
		for _, d := range preds {
			queue = append(queue, d.DependsOnTaskID)
		}
	}
	return false, nil
}

type createDependencyRequest struct {
	DependsOnTaskID uuid.UUID `json:"depends_on_task_id" binding:"required"`
}

// CreateDependency は POST /tasks/:task_id/dependencies。先行タスクを追加する。
// 自己参照・ワークスペース跨ぎ・循環依存を拒否する。
func (h *TaskHandler) CreateDependency(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)

	var req createDependencyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "depends_on_task_id is required"})
		return
	}
	if req.DependsOnTaskID == t.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task cannot depend on itself"})
		return
	}

	ctx := c.Request.Context()

	target, err := h.client.Task.Get(ctx, req.DependsOnTaskID)
	if err != nil || target.WorkspaceID != t.WorkspaceID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "depends_on_task_id must be in the same workspace"})
		return
	}

	cyclic, err := h.wouldCreateCycle(ctx, t.ID, req.DependsOnTaskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check circular dependency"})
		return
	}
	if cyclic {
		c.JSON(http.StatusBadRequest, gin.H{"error": "circular_dependency"})
		return
	}

	var created *ent.TaskDependency
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		var txErr error
		created, txErr = tx.TaskDependency.Create().
			SetTaskID(t.ID).
			SetDependsOnTaskID(req.DependsOnTaskID).
			Save(ctx)
		if txErr != nil {
			return txErr
		}
		return activity.Record(ctx, tx, t.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.dependency_added",
			map[string]any{"depends_on_task_id": req.DependsOnTaskID})
	})
	if ent.IsConstraintError(err) {
		c.JSON(http.StatusConflict, gin.H{"error": "already_depends_on"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create dependency"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": created.ID, "task": dependencyTaskJSON(target)})
}

// DeleteDependency は DELETE /tasks/:task_id/dependencies/:dependency_id。
// このタスクの先行タスク一覧からの解除のみを扱う（＝task_idがこのタスクである行に限定）。
func (h *TaskHandler) DeleteDependency(c *gin.Context) {
	t := middleware.CurrentTask(c)
	u := middleware.CurrentUser(c)

	depID, err := uuid.Parse(c.Param("dependency_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "dependency not found"})
		return
	}

	ctx := c.Request.Context()

	existing, err := h.client.TaskDependency.Query().
		Where(taskdependency.IDEQ(depID), taskdependency.TaskIDEQ(t.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "dependency not found"})
		return
	}

	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		if err := tx.TaskDependency.DeleteOneID(existing.ID).Exec(ctx); err != nil {
			return err
		}
		return activity.Record(ctx, tx, t.WorkspaceID, &t.ID, t.ProjectID, u.ID, "task.dependency_removed",
			map[string]any{"depends_on_task_id": existing.DependsOnTaskID})
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete dependency"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ListAssignableTags は GET /tasks/:task_id/assignable-tags。
// このタスクに付与可能なタグ一覧（プロジェクト所属タスクならそのプロジェクト専用タグ＋
// ワークスペース共通タグ、単体タスクなら共通タグのみ）を返す。タスク詳細のタグピッカー用。
func (h *TaskHandler) ListAssignableTags(c *gin.Context) {
	t := middleware.CurrentTask(c)

	scope := tag.ProjectIDIsNil()
	if t.ProjectID != nil {
		scope = tag.Or(tag.ProjectIDEQ(*t.ProjectID), tag.ProjectIDIsNil())
	}

	tags, err := h.client.Tag.Query().
		Where(tag.WorkspaceIDEQ(t.WorkspaceID), scope).
		Order(tag.ByName()).
		All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list assignable tags"})
		return
	}

	out := make([]gin.H, 0, len(tags))
	for _, tg := range tags {
		out = append(out, tagJSON(tg))
	}
	c.JSON(http.StatusOK, out)
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
	ids, err := h.validTaskTagIDs(ctx, t.WorkspaceID, t.ProjectID, req.TagIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag_ids must be assignable to this task"})
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

// validTaskTagIDs はtag_idsが、このタスクに付与可能なタグ（projectIDが非nilなら
// そのプロジェクト専用タグ＋ワークスペース共通タグ、nilなら共通タグのみ）に
// すべて属することを検証する。
func (h *TaskHandler) validTaskTagIDs(ctx context.Context, workspaceID uuid.UUID, projectID *uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	deduped := dedupUUIDs(ids)
	scope := tag.ProjectIDIsNil()
	if projectID != nil {
		scope = tag.Or(tag.ProjectIDEQ(*projectID), tag.ProjectIDIsNil())
	}
	count, err := h.client.Tag.Query().
		Where(tag.WorkspaceIDEQ(workspaceID), tag.IDIn(deduped...), scope).
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
