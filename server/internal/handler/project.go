package handler

import (
	"context"
	"errors"
	"net/http"

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
	"github.com/osasadev-lab/aibo_pj/server/ent/taskcalendarevent"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskdependency"
	"github.com/osasadev-lab/aibo_pj/server/ent/taskmention"
	"github.com/osasadev-lab/aibo_pj/server/ent/tasktag"
	"github.com/osasadev-lab/aibo_pj/server/ent/workspacemember"
	"github.com/osasadev-lab/aibo_pj/server/internal/activity"
	"github.com/osasadev-lab/aibo_pj/server/internal/calendarsync"
	"github.com/osasadev-lab/aibo_pj/server/internal/middleware"
	"github.com/osasadev-lab/aibo_pj/server/internal/storage"

	"golang.org/x/oauth2"
)

// ProjectHandler は /projects, /workspaces/:workspace_id/projects 配下を扱う。
type ProjectHandler struct {
	client *ent.Client
	r2     *storage.R2Client
	// calCfg/encKeyはM6用。プロジェクト削除時、配下タスクのGoogleカレンダー
	// 連携イベントもベストエフォートで削除する。
	calCfg *oauth2.Config
	encKey []byte
}

func NewProjectHandler(client *ent.Client, r2 *storage.R2Client, calCfg *oauth2.Config, encKey []byte) *ProjectHandler {
	return &ProjectHandler{client: client, r2: r2, calCfg: calCfg, encKey: encKey}
}

// defaultStatusColumns はプロジェクト作成時に自動投入する既定4列（db-schema.md）。
// いずれも is_default=true として作成され、UIから削除できない（ユーザー確認済み）。
var defaultStatusColumns = []struct {
	name         string
	mapsToStatus projectstatuscolumn.MapsToStatus
}{
	{"未対応", projectstatuscolumn.MapsToStatusNotStarted},
	{"対応中", projectstatuscolumn.MapsToStatusInProgress},
	{"対応済", projectstatuscolumn.MapsToStatusDone},
	{"保留", projectstatuscolumn.MapsToStatusOnHold},
}

const maxStatusColumns = 5

// notifyProjectMembership はプロジェクトへの参画・除外をuserIDに通知する
// （project_members行の作成・削除に伴うイベント。role変更のみの場合は呼ばない）。
func notifyProjectMembership(ctx context.Context, tx *ent.Tx, projectID uuid.UUID, projectName string, actorID uuid.UUID, actorName string, userID uuid.UUID, joined bool) error {
	notifType := "project_removed"
	if joined {
		notifType = "project_joined"
	}
	_, err := tx.Notification.Create().
		SetUserID(userID).
		SetType(notifType).
		SetPayload(map[string]any{
			"project_id":      projectID,
			"project_name":    projectName,
			"changed_by":      actorID,
			"changed_by_name": actorName,
		}).
		Save(ctx)
	return err
}

// notifyProjectLifecycle はプロジェクト自体の作成・削除をuserIDに通知する
// （notifyProjectMembershipは「参画状態の変化」用で、こちらは「プロジェクトという
// モノ自体の生成・消滅」用。publicはワークスペース全員が対象、privateは
// 参画メンバー全員が対象。actor自身には呼び出し側で通知しない）。
func notifyProjectLifecycle(ctx context.Context, tx *ent.Tx, projectID uuid.UUID, projectName string, actorID uuid.UUID, actorName string, userID uuid.UUID, created bool) error {
	notifType := "project_deleted"
	if created {
		notifType = "project_created"
	}
	_, err := tx.Notification.Create().
		SetUserID(userID).
		SetType(notifType).
		SetPayload(map[string]any{
			"project_id":      projectID,
			"project_name":    projectName,
			"changed_by":      actorID,
			"changed_by_name": actorName,
		}).
		Save(ctx)
	return err
}

func projectJSON(p *ent.Project) gin.H {
	return gin.H{
		"id":           p.ID,
		"workspace_id": p.WorkspaceID,
		"name":         p.Name,
		"description":  p.Description,
		"visibility":   p.Visibility,
		"created_by":   p.CreatedBy,
	}
}

// List は GET /workspaces/:workspace_id/projects。
// public全件 + privateは参画分のみを返す。各行にis_managerを含める
// （設定画面でOwner/責任者が管理対象プロジェクトを判定するため）。
func (h *ProjectHandler) List(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	projects, err := h.client.Project.Query().
		Where(
			project.WorkspaceIDEQ(m.WorkspaceID),
			project.Or(
				project.VisibilityEQ(project.VisibilityPublic),
				project.HasMembersWith(projectmember.UserIDEQ(m.UserID)),
			),
		).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list projects"})
		return
	}

	isOwner := m.Role == workspacemember.RoleOwner
	managedProjectIDs := map[uuid.UUID]struct{}{}
	if !isOwner && len(projects) > 0 {
		ids := make([]uuid.UUID, 0, len(projects))
		for _, p := range projects {
			ids = append(ids, p.ID)
		}
		rows, err := h.client.ProjectMember.Query().
			Where(
				projectmember.UserIDEQ(m.UserID),
				projectmember.RoleEQ(projectmember.RoleManager),
				projectmember.ProjectIDIn(ids...),
			).
			All(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve managed projects"})
			return
		}
		for _, r := range rows {
			managedProjectIDs[r.ProjectID] = struct{}{}
		}
	}

	out := make([]gin.H, 0, len(projects))
	for _, p := range projects {
		row := projectJSON(p)
		_, managed := managedProjectIDs[p.ID]
		row["is_manager"] = isOwner || managed
		out = append(out, row)
	}
	c.JSON(http.StatusOK, out)
}

type createProjectRequest struct {
	Name        string      `json:"name" binding:"required"`
	Description *string     `json:"description"`
	Visibility  string      `json:"visibility" binding:"required,oneof=public private"`
	MemberIDs   []uuid.UUID `json:"member_ids"`
}

// Create は POST /workspaces/:workspace_id/projects。可視性指定必須。
// publicはワークスペース全員が閲覧できるため参画メンバーという概念が無く、
// 作成者のみをmanagerとして登録する（member_idsは無視する）。privateのみ、
// 指定されたmember_idsをstaffとして参画させる。既定4ステータス列を自動生成する。
func (h *ProjectHandler) Create(c *gin.Context) {
	m := middleware.CurrentMembership(c)
	u := middleware.CurrentUser(c)

	var req createProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, visibility, member_ids are required"})
		return
	}

	ctx := c.Request.Context()

	ids := []uuid.UUID{m.UserID}
	if req.Visibility == "private" {
		// member_idsが全てこのworkspaceのメンバーであることを検証する。
		memberSet := map[uuid.UUID]struct{}{m.UserID: {}}
		for _, id := range req.MemberIDs {
			memberSet[id] = struct{}{}
		}
		ids = make([]uuid.UUID, 0, len(memberSet))
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
	}

	var created *ent.Project
	err := withTx(ctx, h.client, func(tx *ent.Tx) error {
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
			b := tx.ProjectMember.Create().SetProjectID(p.ID).SetUserID(uid)
			// 作成者は自動的に責任者（manager）にする。他はデフォルトのstaffのまま。
			if uid == m.UserID {
				b = b.SetRole(projectmember.RoleManager)
			}
			memberBuilders = append(memberBuilders, b)
		}
		if _, err := tx.ProjectMember.CreateBulk(memberBuilders...).Save(ctx); err != nil {
			return err
		}

		for _, uid := range ids {
			if uid == m.UserID {
				continue
			}
			if err := notifyProjectMembership(ctx, tx, p.ID, p.Name, m.UserID, u.Name, uid, true); err != nil {
				return err
			}
		}

		// publicは参画メンバーという概念が無く前段のループが空振りするため、
		// 代わりにワークスペース全員（作成者以外）へプロジェクト作成を通知する
		// （publicは誰でも閲覧できるため、新設自体を全員に知らせる）。
		if req.Visibility == "public" {
			wms, err := tx.WorkspaceMember.Query().
				Where(workspacemember.WorkspaceIDEQ(m.WorkspaceID)).
				All(ctx)
			if err != nil {
				return err
			}
			for _, wm := range wms {
				if wm.UserID == m.UserID {
					continue
				}
				if err := notifyProjectLifecycle(ctx, tx, p.ID, p.Name, m.UserID, u.Name, wm.UserID, true); err != nil {
					return err
				}
			}
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

// Update は PATCH /projects/:project_id。RequireProjectManager限定
// （プロジェクト責任者かworkspace Ownerのみ名称・説明・可視性を変更できる）。
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

// Delete は DELETE /projects/:project_id。RequireProjectManager限定。
// entのschemaにカスケード削除指定が無いため、関連テーブルを手動で正しい順に削除する。
// comments/activity_logsもtask_id・project_idを参照しているため、task.go Deleteと
// 同様に先に削除しておかないと外部キー制約違反になる。
func (h *ProjectHandler) Delete(c *gin.Context) {
	p := middleware.CurrentProject(c)
	u := middleware.CurrentUser(c)
	ctx := c.Request.Context()

	var attachmentKeys []string
	var calendarEventRefs []calendarsync.EventRef
	err := withTx(ctx, h.client, func(tx *ent.Tx) error {
		taskIDs, err := tx.Task.Query().
			Where(task.ProjectIDEQ(p.ID)).
			IDs(ctx)
		if err != nil {
			return err
		}

		if len(taskIDs) > 0 {
			// Googleカレンダー連携イベント（M6）。Google側の実削除はコミット後に
			// ベストエフォートで行うため、ここでは参照だけ収集する（task.goのDeleteと
			// 同じ理由でDB行自体は先に消す必要がある）。
			calendarEvents, err := tx.TaskCalendarEvent.Query().Where(taskcalendarevent.TaskIDIn(taskIDs...)).WithUser().All(ctx)
			if err != nil {
				return err
			}
			for _, e := range calendarEvents {
				calendarEventRefs = append(calendarEventRefs, calendarsync.EventRef{User: e.Edges.User, GoogleEventID: e.GoogleEventID})
			}
			if _, err := tx.TaskCalendarEvent.Delete().Where(taskcalendarevent.TaskIDIn(taskIDs...)).Exec(ctx); err != nil {
				return err
			}

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
			if _, err := tx.TaskMention.Delete().Where(taskmention.TaskIDIn(taskIDs...)).Exec(ctx); err != nil {
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
			// R2オブジェクトはDBコミット確定後に削除する（キーだけここで収集）。
			attachments, err := tx.Attachment.Query().Where(attachment.TaskIDIn(taskIDs...)).All(ctx)
			if err != nil {
				return err
			}
			for _, a := range attachments {
				attachmentKeys = append(attachmentKeys, a.StorageKey)
			}
			if _, err := tx.Attachment.Delete().Where(attachment.TaskIDIn(taskIDs...)).Exec(ctx); err != nil {
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
		// このプロジェクト専用タグ（project_id非nil）も削除する。
		// task_tagsはこのプロジェクトのタスクの分は既に上で削除済みだが、
		// タグ行自体はここで消す必要がある。
		projectTagIDs, err := tx.Tag.Query().Where(tag.ProjectIDEQ(p.ID)).IDs(ctx)
		if err != nil {
			return err
		}
		if len(projectTagIDs) > 0 {
			if _, err := tx.TaskTag.Delete().Where(tasktag.TagIDIn(projectTagIDs...)).Exec(ctx); err != nil {
				return err
			}
			if _, err := tx.Tag.Delete().Where(tag.IDIn(projectTagIDs...)).Exec(ctx); err != nil {
				return err
			}
		}
		// project_members行を消す前に通知対象を確定しておく（private=参画メンバー
		// 全員、public=ワークスペース全員。ともにactor自身は除く）。
		var recipientIDs []uuid.UUID
		if p.Visibility == project.VisibilityPublic {
			wms, err := tx.WorkspaceMember.Query().
				Where(workspacemember.WorkspaceIDEQ(p.WorkspaceID)).
				All(ctx)
			if err != nil {
				return err
			}
			for _, wm := range wms {
				if wm.UserID != u.ID {
					recipientIDs = append(recipientIDs, wm.UserID)
				}
			}
		} else {
			pms, err := tx.ProjectMember.Query().Where(projectmember.ProjectIDEQ(p.ID)).All(ctx)
			if err != nil {
				return err
			}
			for _, pm := range pms {
				if pm.UserID != u.ID {
					recipientIDs = append(recipientIDs, pm.UserID)
				}
			}
		}

		if _, err := tx.ProjectMember.Delete().Where(projectmember.ProjectIDEQ(p.ID)).Exec(ctx); err != nil {
			return err
		}
		for _, uid := range recipientIDs {
			if err := notifyProjectLifecycle(ctx, tx, p.ID, p.Name, u.ID, u.Name, uid, false); err != nil {
				return err
			}
		}
		// project.deleted自体は削除対象のproject_idを参照できない（削除後に外部キー
		// 違反になる）ため、project_idを付けずpayloadにだけ残す。ハイライト機能
		// （GET /workspaces/:id/activity、M5）は削除後のDB状態からこの行の可視性を
		// 判定できないため、可視性判定に必要な情報（visibility・private時のメンバー
		// 一覧）をpayloadにも複製しておく。recipientIDsは上でnotifyProjectLifecycle用に
		// 算出済み（actor自身は含まない）のものを再利用する。
		deletedPayload := map[string]any{"name": p.Name, "project_id": p.ID, "visibility": string(p.Visibility)}
		if p.Visibility == project.VisibilityPrivate {
			deletedPayload["member_user_ids"] = recipientIDs
		}
		if err := activity.Record(ctx, tx, p.WorkspaceID, nil, nil, u.ID, "project.deleted",
			deletedPayload); err != nil {
			return err
		}
		return tx.Project.DeleteOneID(p.ID).Exec(ctx)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete project"})
		return
	}
	deleteR2Objects(ctx, h.r2, attachmentKeys)
	// バックグラウンド化の理由はtask.goと同じ（体感速度対策、2026-08-21）。
	calendarsync.Async(func(ctx context.Context) {
		calendarsync.DeleteGoogleEventsOnly(ctx, h.calCfg, h.encKey, calendarEventRefs)
	})
	c.Status(http.StatusNoContent)
}

// ListMembers は GET /projects/:project_id/members。
// publicプロジェクトはworkspace全員が閲覧できてしまうため、参画者一覧自体は
// 責任者（manager）かworkspace Ownerにのみ公開する。privateはRequireProjectAccessの
// 時点で参画者以外弾かれているため無条件で許可する（8番の可視性ルール）。
func (h *ProjectHandler) ListMembers(c *gin.Context) {
	p := middleware.CurrentProject(c)
	u := middleware.CurrentUser(c)
	m := middleware.CurrentMembership(c)
	ctx := c.Request.Context()

	if p.Visibility == project.VisibilityPublic && m.Role != workspacemember.RoleOwner {
		isManager, err := h.client.ProjectMember.Query().
			Where(
				projectmember.ProjectIDEQ(p.ID),
				projectmember.UserIDEQ(u.ID),
				projectmember.RoleEQ(projectmember.RoleManager),
			).
			Exist(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check permission"})
			return
		}
		if !isManager {
			c.JSON(http.StatusForbidden, gin.H{"error": "project manager required"})
			return
		}
	}

	members, err := h.client.ProjectMember.Query().
		Where(projectmember.ProjectIDEQ(p.ID)).
		WithUser().
		All(ctx)
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
			"role":       pm.Role,
		})
	}
	c.JSON(http.StatusOK, out)
}

type putMembersRequest struct {
	MemberIDs []uuid.UUID `json:"member_ids" binding:"required"`
}

var errNoManager = errors.New("resulting member set has no manager")

// PutMembers は PUT /projects/:project_id/members。参画メンバーを入れ替える。
// publicはワークスペース全員が閲覧できるため参画という概念自体が無く、
// 責任者の付与はPutManagersで行う（本エンドポイントはprivate専用）。
// 既存メンバーのroleは維持し（新規追加分はデフォルトのstaff）、置き換え後の集合に
// managerが1人もいなくなる場合は拒否する（プロジェクトが無責任者状態になるのを防ぐ、
// ワークスペースOwnerの「最後の1人保護」と同じ考え方）。
func (h *ProjectHandler) PutMembers(c *gin.Context) {
	p := middleware.CurrentProject(c)
	m := middleware.CurrentMembership(c)
	u := middleware.CurrentUser(c)

	if p.Visibility == project.VisibilityPublic {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not_applicable_for_public_project"})
		return
	}

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
		existing, err := tx.ProjectMember.Query().Where(projectmember.ProjectIDEQ(p.ID)).All(ctx)
		if err != nil {
			return err
		}
		roleByUser := make(map[uuid.UUID]projectmember.Role, len(existing))
		wasMember := make(map[uuid.UUID]struct{}, len(existing))
		for _, pm := range existing {
			roleByUser[pm.UserID] = pm.Role
			wasMember[pm.UserID] = struct{}{}
		}
		isMember := make(map[uuid.UUID]struct{}, len(ids))
		for _, uid := range ids {
			isMember[uid] = struct{}{}
		}

		managerCount := 0
		for _, uid := range ids {
			if roleByUser[uid] == projectmember.RoleManager {
				managerCount++
			}
		}
		if managerCount == 0 {
			return errNoManager
		}

		if _, err := tx.ProjectMember.Delete().Where(projectmember.ProjectIDEQ(p.ID)).Exec(ctx); err != nil {
			return err
		}
		builders := make([]*ent.ProjectMemberCreate, 0, len(ids))
		for _, uid := range ids {
			b := tx.ProjectMember.Create().SetProjectID(p.ID).SetUserID(uid)
			if roleByUser[uid] == projectmember.RoleManager {
				b = b.SetRole(projectmember.RoleManager)
			}
			builders = append(builders, b)
		}
		if len(builders) > 0 {
			if _, err := tx.ProjectMember.CreateBulk(builders...).Save(ctx); err != nil {
				return err
			}
		}

		// 参画・除外の通知（新規追加・除外されたユーザーのみ。既存参画者のroleのみの
		// 変更はnotifyProjectMembershipを呼ばない＝参画状態自体は変わっていないため）。
		for _, uid := range ids {
			if _, was := wasMember[uid]; was {
				continue
			}
			if err := notifyProjectMembership(ctx, tx, p.ID, p.Name, m.UserID, u.Name, uid, true); err != nil {
				return err
			}
		}
		for uid := range wasMember {
			if _, still := isMember[uid]; still {
				continue
			}
			if err := notifyProjectMembership(ctx, tx, p.ID, p.Name, m.UserID, u.Name, uid, false); err != nil {
				return err
			}
		}
		return nil
	})

	switch {
	case err == errNoManager:
		c.JSON(http.StatusBadRequest, gin.H{"error": "at_least_one_manager_required"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update members"})
	default:
		c.Status(http.StatusNoContent)
	}
}

type changeProjectMemberRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=manager staff"`
}

var errLastManager = errors.New("last manager cannot be demoted")

// ChangeMemberRole は PATCH /projects/:project_id/members/:member_id。
// 責任者(manager)⇄スタッフ(staff)を切り替える。最後の1人のmanagerは降格できない。
// publicはPutManagers専用（staffという参画概念自体が無いため）。
func (h *ProjectHandler) ChangeMemberRole(c *gin.Context) {
	p := middleware.CurrentProject(c)

	if p.Visibility == project.VisibilityPublic {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not_applicable_for_public_project"})
		return
	}

	memberID, err := uuid.Parse(c.Param("member_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}

	var req changeProjectMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid role is required"})
		return
	}
	newRole := projectmember.Role(req.Role)

	ctx := c.Request.Context()
	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		target, err := tx.ProjectMember.Query().
			Where(projectmember.IDEQ(memberID), projectmember.ProjectIDEQ(p.ID)).
			Only(ctx)
		if err != nil {
			return err
		}

		if target.Role == projectmember.RoleManager && newRole != projectmember.RoleManager {
			if err := ensureNotLastManager(ctx, tx, p.ID, target.ID); err != nil {
				return err
			}
		}

		_, err = tx.ProjectMember.UpdateOneID(target.ID).SetRole(newRole).Save(ctx)
		return err
	})

	switch {
	case err == errLastManager:
		c.JSON(http.StatusBadRequest, gin.H{"error": "last_manager"})
	case ent.IsNotFound(err):
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to change role"})
	default:
		c.Status(http.StatusNoContent)
	}
}

type putManagersRequest struct {
	UserIDs []uuid.UUID `json:"user_ids" binding:"required"`
}

// PutManagers は PUT /projects/:project_id/managers。責任者(manager)の付与・剥奪。
// publicプロジェクトはワークスペース全員が閲覧できるため「参画」という概念自体が無く、
// 責任者を付与されたユーザーのみproject_membersにrole=managerとして記録する
// （剥奪時はその行ごと削除する）。privateは既存の参画メンバーのうち責任者権限だけを
// 入れ替える（剥奪してもstaffとして参画は維持する。参画自体の追加・削除はPutMembers）。
func (h *ProjectHandler) PutManagers(c *gin.Context) {
	p := middleware.CurrentProject(c)
	m := middleware.CurrentMembership(c)
	u := middleware.CurrentUser(c)

	var req putManagersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids is required"})
		return
	}

	ctx := c.Request.Context()
	ids := dedupUUIDs(req.UserIDs)
	if len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at_least_one_manager_required"})
		return
	}

	validCount, err := h.client.WorkspaceMember.Query().
		Where(workspacemember.WorkspaceIDEQ(m.WorkspaceID), workspacemember.UserIDIn(ids...)).
		Count(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate members"})
		return
	}
	if validCount != len(ids) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids must all be workspace members"})
		return
	}

	err = withTx(ctx, h.client, func(tx *ent.Tx) error {
		existing, err := tx.ProjectMember.Query().Where(projectmember.ProjectIDEQ(p.ID)).All(ctx)
		if err != nil {
			return err
		}
		byUser := make(map[uuid.UUID]*ent.ProjectMember, len(existing))
		for _, pm := range existing {
			byUser[pm.UserID] = pm
		}
		newSet := make(map[uuid.UUID]struct{}, len(ids))
		for _, uid := range ids {
			newSet[uid] = struct{}{}
		}

		// 新たにmanagerにするユーザー：既存行があればrole更新、無ければ新規作成。
		// 新規作成の場合はproject_membersに初めて参画したことになるため通知する
		// （publicで新たに責任者を付与された、等）。
		for _, uid := range ids {
			if pm, ok := byUser[uid]; ok {
				if pm.Role != projectmember.RoleManager {
					if _, err := tx.ProjectMember.UpdateOneID(pm.ID).SetRole(projectmember.RoleManager).Save(ctx); err != nil {
						return err
					}
				}
				continue
			}
			if _, err := tx.ProjectMember.Create().
				SetProjectID(p.ID).
				SetUserID(uid).
				SetRole(projectmember.RoleManager).
				Save(ctx); err != nil {
				return err
			}
			if err := notifyProjectMembership(ctx, tx, p.ID, p.Name, m.UserID, u.Name, uid, true); err != nil {
				return err
			}
		}

		// 外れたmanagerの後始末：publicは参画という概念自体が無いので行ごと削除
		// （＝参画から外れたことになるため通知する）、privateは参画は維持したまま
		// staffに降格するだけ（参画状態自体は変わらないため通知しない）。
		for _, pm := range existing {
			if pm.Role != projectmember.RoleManager {
				continue
			}
			if _, still := newSet[pm.UserID]; still {
				continue
			}
			if p.Visibility == project.VisibilityPublic {
				if err := tx.ProjectMember.DeleteOne(pm).Exec(ctx); err != nil {
					return err
				}
				if err := notifyProjectMembership(ctx, tx, p.ID, p.Name, m.UserID, u.Name, pm.UserID, false); err != nil {
					return err
				}
			} else if _, err := tx.ProjectMember.UpdateOneID(pm.ID).SetRole(projectmember.RoleStaff).Save(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update managers"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ensureNotLastManager はmember.goのensureNotLastOwnerと同型のヘルパー。
// targetMemberIDを除いた残りmanager数が0ならerrLastManagerを返す。
func ensureNotLastManager(ctx context.Context, tx *ent.Tx, projectID, targetMemberID uuid.UUID) error {
	remaining, err := tx.ProjectMember.Query().
		Where(
			projectmember.ProjectIDEQ(projectID),
			projectmember.RoleEQ(projectmember.RoleManager),
			projectmember.IDNEQ(targetMemberID),
		).
		Count(ctx)
	if err != nil {
		return err
	}
	if remaining == 0 {
		return errLastManager
	}
	return nil
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
