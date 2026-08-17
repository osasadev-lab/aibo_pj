package handler

import (
	"net/http"

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
	"github.com/osasadev-lab/aibo_pj/server/ent/task"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskassignee"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskdependency"
	"github.com/osasadev-lab/aibo_pj/server/ent/tasktag"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	"github.com/osasadev-lab/aibo_pj/server/internal/activity"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
)

// ProjectHandler は /projects, /workspaces/:workspace_id/projects 配下を扱う。
type ProjectHandler struct {
	client *ent.Client
}

func NewProjectHandler(client *ent.Client) *ProjectHandler {
	return &ProjectHandler{client: client}
}

// defaultStatusColumns はプロジェクト作成時に自動投入する既定4列（db-schema.md）。
// いずれも is_default=true として作成され、UIから削除できない（ユーザー確認済み）。
var defaultStatusColumns = []struct {
	name         string
	mapsToStatus projectstatuscolumn.MapsToStatus
}{
	{"未対応", projectstatuscolumn.MapsToStatusNotStarted},
	{"着手中", projectstatuscolumn.MapsToStatusInProgress},
	{"対応済", projectstatuscolumn.MapsToStatusDone},
	{"保留", projectstatuscolumn.MapsToStatusOnHold},
}

const maxStatusColumns = 5

func projectJSON(p *ent.Project) gin.H {
	return gin.H{
		"id":          p.ID,
		"workspace_id": p.WorkspaceID,
		"name":        p.Name,
		"description": p.Description,
		"visibility":  p.Visibility,
		"created_by":  p.CreatedBy,
	}
}

// List は GET /workspaces/:workspace_id/projects。
// public全件 + privateは参画分のみを返す。
func (h *ProjectHandler) List(c *gin.Context) {
	m := middleware.CurrentMembership(c)

	projects, err := h.client.Project.Query().
		Where(
			project.WorkspaceIDEQ(m.WorkspaceID),
			project.Or(
				project.VisibilityEQ(project.VisibilityPublic),
				project.HasMembersWith(projectmember.UserIDEQ(m.UserID)),
			),
		).
		All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list projects"})
		return
	}

	out := make([]gin.H, 0, len(projects))
	for _, p := range projects {
		out = append(out, projectJSON(p))
	}
	c.JSON(http.StatusOK, out)
}

type createProjectRequest struct {
	Name        string      `json:"name" binding:"required"`
	Description *string     `json:"description"`
	Visibility  string      `json:"visibility" binding:"required,oneof=public private"`
	MemberIDs   []uuid.UUID `json:"member_ids" binding:"required"`
}

// Create は POST /workspaces/:workspace_id/projects。
// 参画メンバー・可視性指定必須。既定4ステータス列を自動生成する。
func (h *ProjectHandler) Create(c *gin.Context) {
	m := middleware.CurrentMembership(c)

	var req createProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, visibility, member_ids are required"})
		return
	}

	ctx := c.Request.Context()

	// member_idsが全てこのworkspaceのメンバーであることを検証する。
	memberSet := map[uuid.UUID]struct{}{m.UserID: {}}
	for _, id := range req.MemberIDs {
		memberSet[id] = struct{}{}
	}
	ids := make([]uuid.UUID, 0, len(memberSet))
	for id := range memberSet {
		ids = append(ids, id)
	}
	validCount, err := h.client.WorkspaceMember.Query().
		Where(
			workspacemember.WorkspaceIDEQ(m.WorkspaceID),
			workspacemember.UserIDIn(ids...),
		).
		Count(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate members"})
		return
	}
	if validCount != len(ids) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "member_ids must all be workspace members"})
		return
	}

	var created *ent.Project
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		builder := tx.Project.Create().
			SetWorkspaceID(m.WorkspaceID).
			SetName(req.Name).
			SetVisibility(project.Visibility(req.Visibility)).
			SetCreatedBy(m.UserID)
		if req.Description != nil {
			builder = builder.SetDescription(*req.Description)
		}
		p, err := builder.Save(ctx)
		if err != nil {
			return err
		}

		memberBuilders := make([]*ent.ProjectMemberCreate, 0, len(ids))
		for _, uid := range ids {
			memberBuilders = append(memberBuilders, tx.ProjectMember.Create().
				SetProjectID(p.ID).
				SetUserID(uid))
		}
		if _, err := tx.ProjectMember.CreateBulk(memberBuilders...).Save(ctx); err != nil {
			return err
		}

		columnBuilders := make([]*ent.ProjectStatusColumnCreate, 0, len(defaultStatusColumns))
		for i, col := range defaultStatusColumns {
			columnBuilders = append(columnBuilders, tx.ProjectStatusColumn.Create().
				SetProjectID(p.ID).
				SetName(col.name).
				SetPosition(i).
				SetMapsToStatus(col.mapsToStatus).
				SetIsDefault(true))
		}
		if _, err := tx.ProjectStatusColumn.CreateBulk(columnBuilders...).Save(ctx); err != nil {
			return err
		}

		if err := activity.Record(ctx, tx, m.WorkspaceID, nil, &p.ID, m.UserID, "project.created",
			map[string]any{"name": p.Name, "visibility": string(p.Visibility)}); err != nil {
			return err
		}

		created = p
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create project"})
		return
	}

	c.JSON(http.StatusCreated, projectJSON(created))
}

// Get は GET /projects/:project_id。RequireProjectAccessで可視性確認済み。
func (h *ProjectHandler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, projectJSON(middleware.CurrentProject(c)))
}

type updateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Visibility  *string `json:"visibility" binding:"omitempty,oneof=public private"`
}

// Update は PATCH /projects/:project_id。project accessがあれば誰でも可
// （api-spec.mdに権限制限の明記が無いための仮定）。
func (h *ProjectHandler) Update(c *gin.Context) {
	p := middleware.CurrentProject(c)
	u := middleware.CurrentUser(c)

	var req updateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()
	var updated *ent.Project
	err := withTx(ctx, h.client, func(tx *ent.Tx) error {
		builder := tx.Project.UpdateOneID(p.ID)
		changes := map[string]any{}
		if req.Name != nil {
			builder = builder.SetName(*req.Name)
			changes["name"] = *req.Name
		}
		if req.Description != nil {
			builder = builder.SetDescription(*req.Description)
			changes["description"] = *req.Description
		}
		if req.Visibility != nil {
			builder = builder.SetVisibility(project.Visibility(*req.Visibility))
			changes["visibility"] = *req.Visibility
		}

		var err error
		updated, err = builder.Save(ctx)
		if err != nil {
			return err
		}
		return activity.Record(ctx, tx, p.WorkspaceID, nil, &p.ID, u.ID, "project.updated", changes)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update project"})
		return
	}
	c.JSON(http.StatusOK, projectJSON(updated))
}

// Delete は DELETE /projects/:project_id。RequireProjectOwnerOrCreator限定。
// entのschemaにカスケード削除指定が無いため、関連テーブルを手動で正しい順に削除する。
// comments/activity_logsもtask_id・project_idを参照しているため、task.go Deleteと
// 同様に先に削除しておかないと外部キー制約違反になる。
func (h *ProjectHandler) Delete(c *gin.Context) {
	p := middleware.CurrentProject(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	err := withTx(ctx, h.client, func(tx *ent.Tx) error {
		taskIDs, err := tx.Task.Query().
			Where(task.ProjectIDEQ(p.ID)).
			IDs(ctx)
		if err != nil {
			return err
		}

		if len(taskIDs) > 0 {
			if _, err := tx.TaskDependency.Delete().
				Where(taskdependency.Or(
					taskdependency.TaskIDIn(taskIDs...),
					taskdependency.DependsOnTaskIDIn(taskIDs...),
				)).
				Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.TaskTag.Delete().Where(tasktag.TaskIDIn(taskIDs...)).Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.TaskAssignee.Delete().Where(taskassignee.TaskIDIn(taskIDs...)).Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.CommentMention.Delete().
				Where(commentmention.HasCommentWith(comment.TaskIDIn(taskIDs...))).
				Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.Comment.Delete().Where(comment.TaskIDIn(taskIDs...)).Exec(ctx); err != nil {
				return err
			}
		}
		// activity_logsはtask_id経由（このプロジェクトのタスク）とproject_id経由
		// （project.created/updated等）の両方で参照され得るため両方消す。
		if _, err := tx.ActivityLog.Delete().
			Where(activitylog.Or(
				activitylog.TaskIDIn(taskIDs...),
				activitylog.ProjectIDEQ(p.ID),
			)).
			Exec(ctx); err != nil {
			return err
		}
		if len(taskIDs) > 0 {
			if _, err := tx.Task.Delete().Where(task.IDIn(taskIDs...)).Exec(ctx); err != nil {
				return err
			}
		}

		if _, err := tx.Section.Delete().Where(section.ProjectIDEQ(p.ID)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.ProjectStatusColumn.Delete().Where(projectstatuscolumn.ProjectIDEQ(p.ID)).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.ProjectMember.Delete().Where(projectmember.ProjectIDEQ(p.ID)).Exec(ctx); err != nil {
			return err
		}
		// project.deleted自体は削除対象のproject_idを参照できない（削除後に外部キー
		// 違反になる）ため、project_idを付けずpayloadにだけ残す。
		if err := activity.Record(ctx, tx, p.WorkspaceID, nil, nil, u.ID, "project.deleted",
			map[string]any{"name": p.Name, "project_id": p.ID}); err != nil {
			return err
		}
		return tx.Project.DeleteOneID(p.ID).Exec(ctx)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete project"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ListMembers は GET /projects/:project_id/members。
func (h *ProjectHandler) ListMembers(c *gin.Context) {
	p := middleware.CurrentProject(c)

	members, err := h.client.ProjectMember.Query().
		Where(projectmember.ProjectIDEQ(p.ID)).
		WithUser().
		All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list members"})
		return
	}

	out := make([]gin.H, 0, len(members))
	for _, pm := range members {
		out = append(out, gin.H{
			"id":         pm.ID,
			"user_id":    pm.Edges.User.ID,
			"name":       pm.Edges.User.Name,
			"email":      pm.Edges.User.Email,
			"avatar_url": pm.Edges.User.AvatarURL,
		})
	}
	c.JSON(http.StatusOK, out)
}

type putMembersRequest struct {
	MemberIDs []uuid.UUID `json:"member_ids" binding:"required"`
}

// PutMembers は PUT /projects/:project_id/members。参画メンバーを入れ替える。
func (h *ProjectHandler) PutMembers(c *gin.Context) {
	p := middleware.CurrentProject(c)
	m := middleware.CurrentMembership(c)

	var req putMembersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "member_ids is required"})
		return
	}

	ctx := c.Request.Context()

	memberSet := map[uuid.UUID]struct{}{}
	for _, id := range req.MemberIDs {
		memberSet[id] = struct{}{}
	}
	ids := make([]uuid.UUID, 0, len(memberSet))
	for id := range memberSet {
		ids = append(ids, id)
	}
	validCount, err := h.client.WorkspaceMember.Query().
		Where(
			workspacemember.WorkspaceIDEQ(m.WorkspaceID),
			workspacemember.UserIDIn(ids...),
		).
		Count(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate members"})
		return
	}
	if validCount != len(ids) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "member_ids must all be workspace members"})
		return
	}

	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		if _, err := tx.ProjectMember.Delete().Where(projectmember.ProjectIDEQ(p.ID)).Exec(ctx); err != nil {
			return err
		}
		builders := make([]*ent.ProjectMemberCreate, 0, len(ids))
		for _, uid := range ids {
			builders = append(builders, tx.ProjectMember.Create().SetProjectID(p.ID).SetUserID(uid))
		}
		if len(builders) == 0 {
			return nil
		}
		_, err := tx.ProjectMember.CreateBulk(builders...).Save(ctx)
		return err
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update members"})
		return
	}
	c.Status(http.StatusNoContent)
}

func statusColumnJSON(col *ent.ProjectStatusColumn) gin.H {
	return gin.H{
		"id":             col.ID,
		"project_id":     col.ProjectID,
		"name":           col.Name,
		"position":       col.Position,
		"maps_to_status": col.MapsToStatus,
		"is_default":     col.IsDefault,
	}
}

// ListStatusColumns は GET /projects/:project_id/status-columns。
func (h *ProjectHandler) ListStatusColumns(c *gin.Context) {
	p := middleware.CurrentProject(c)

	cols, err := h.client.ProjectStatusColumn.Query().
		Where(projectstatuscolumn.ProjectIDEQ(p.ID)).
		Order(projectstatuscolumn.ByPosition()).
		All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list status columns"})
		return
	}

	out := make([]gin.H, 0, len(cols))
	for _, col := range cols {
		out = append(out, statusColumnJSON(col))
	}
	c.JSON(http.StatusOK, out)
}

type createStatusColumnRequest struct {
	Name         string `json:"name" binding:"required"`
	MapsToStatus string `json:"maps_to_status" binding:"required,oneof=not_started in_progress done on_hold"`
}

// CreateStatusColumn は POST /projects/:project_id/status-columns。
// プロジェクトあたり最大5列。
func (h *ProjectHandler) CreateStatusColumn(c *gin.Context) {
	p := middleware.CurrentProject(c)

	var req createStatusColumnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and maps_to_status are required"})
		return
	}

	ctx := c.Request.Context()

	count, err := h.client.ProjectStatusColumn.Query().
		Where(projectstatuscolumn.ProjectIDEQ(p.ID)).
		Count(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count status columns"})
		return
	}
	if count >= maxStatusColumns {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_columns_exceeded"})
		return
	}

	col, err := h.client.ProjectStatusColumn.Create().
		SetProjectID(p.ID).
		SetName(req.Name).
		SetPosition(count).
		SetMapsToStatus(projectstatuscolumn.MapsToStatus(req.MapsToStatus)).
		Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create status column"})
		return
	}
	c.JSON(http.StatusCreated, statusColumnJSON(col))
}

type updateStatusColumnRequest struct {
	Name         *string `json:"name"`
	Position     *int    `json:"position"`
	MapsToStatus *string `json:"maps_to_status" binding:"omitempty,oneof=not_started in_progress done on_hold"`
}

// UpdateStatusColumn は PATCH /projects/:project_id/status-columns/:column_id。
func (h *ProjectHandler) UpdateStatusColumn(c *gin.Context) {
	p := middleware.CurrentProject(c)

	columnID, err := uuid.Parse(c.Param("column_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "status column not found"})
		return
	}

	var req updateStatusColumnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()

	col, err := h.client.ProjectStatusColumn.Query().
		Where(projectstatuscolumn.IDEQ(columnID), projectstatuscolumn.ProjectIDEQ(p.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "status column not found"})
		return
	}

	builder := h.client.ProjectStatusColumn.UpdateOneID(col.ID)
	if req.Name != nil {
		builder = builder.SetName(*req.Name)
	}
	if req.Position != nil {
		builder = builder.SetPosition(*req.Position)
	}
	if req.MapsToStatus != nil {
		builder = builder.SetMapsToStatus(projectstatuscolumn.MapsToStatus(*req.MapsToStatus))
	}

	updated, err := builder.Save(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status column"})
		return
	}
	c.JSON(http.StatusOK, statusColumnJSON(updated))
}

// DeleteStatusColumn は DELETE /projects/:project_id/status-columns/:column_id。
// 列にタスクが残っている場合、クエリパラメータtarget_column_idで移動先を
// 指定しない限り拒否する（db-schema.md「タスクが残っている場合は移動を強制」）。
func (h *ProjectHandler) DeleteStatusColumn(c *gin.Context) {
	p := middleware.CurrentProject(c)

	columnID, err := uuid.Parse(c.Param("column_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "status column not found"})
		return
	}

	ctx := c.Request.Context()

	col, err := h.client.ProjectStatusColumn.Query().
		Where(projectstatuscolumn.IDEQ(columnID), projectstatuscolumn.ProjectIDEQ(p.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "status column not found"})
		return
	}

	if col.IsDefault {
		c.JSON(http.StatusBadRequest, gin.H{"error": "default_column_not_deletable"})
		return
	}

	taskCount, err := h.client.Task.Query().
		Where(task.StatusColumnIDEQ(col.ID)).
		Count(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count tasks"})
		return
	}

	if taskCount == 0 {
		if err := h.client.ProjectStatusColumn.DeleteOneID(col.ID).Exec(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete status column"})
			return
		}
		c.Status(http.StatusNoContent)
		return
	}

	targetIDStr := c.Query("target_column_id")
	if targetIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tasks_present", "task_count": taskCount})
		return
	}
	targetID, err := uuid.Parse(targetIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_column_id"})
		return
	}
	target, err := h.client.ProjectStatusColumn.Query().
		Where(projectstatuscolumn.IDEQ(targetID), projectstatuscolumn.ProjectIDEQ(p.ID)).
		Only(ctx)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_column_id must belong to the same project"})
		return
	}
	if target.ID == col.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_column_id must differ from the deleted column"})
		return
	}

	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		if _, err := tx.Task.Update().
			Where(task.StatusColumnIDEQ(col.ID)).
			SetStatusColumnID(target.ID).
			SetStatus(task.Status(target.MapsToStatus)).
			Save(ctx); err != nil {
			return err
		}
		return tx.ProjectStatusColumn.DeleteOneID(col.ID).Exec(ctx)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete status column"})
		return
	}
	c.Status(http.StatusNoContent)
}
